package parity

import (
	"encoding/json"
	"fmt"
	"net"
	"net/mail"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

// TestOptionalAbsorbsFailure covers ?optional=yes, which changes the reported
// result rather than the request — so no request diff can see it, and it needs
// the same failure budget the retry work introduced.
func TestOptionalAbsorbsFailure(t *testing.T) {
	for _, tc := range []struct {
		name     string
		query    string
		failures int
		wantErr  bool
	}{
		{name: "a failure is reported by default", failures: 1, wantErr: true},
		{name: "optional absorbs it", query: "?optional=yes", failures: 1},
		{
			// Retries still run; optional only reinterprets the result once
			// they are exhausted.
			name:     "optional does not skip retries",
			query:    "?optional=yes&retry=1",
			failures: 5,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := notify.ParseURL("json://localhost/" + tc.query)
			if err != nil {
				t.Fatalf("parse url: %v", err)
			}
			target, err := notify.NewTarget(parsed)
			if err != nil {
				t.Fatalf("build target: %v", err)
			}

			restore := testutil.FailNextRequests(tc.failures)
			defer restore()

			var sendErr error
			specs := testutil.CaptureGoRequests(t, func() error {
				sendErr = notify.DispatchSendWithOverflow(
					target, parsed, "body", "title", notify.NotifyInfo, nil)

				return nil
			})

			if tc.wantErr && sendErr == nil {
				t.Fatal("expected the failure to be reported")
			}
			if !tc.wantErr && sendErr != nil {
				t.Fatalf("expected the failure to be absorbed, got: %v", sendErr)
			}

			// Absorbing a failure must not mean skipping the send.
			if len(specs) == 0 {
				t.Fatal("no request was attempted")
			}
		})
	}
}

// TestStoreCanBeDisabledPerURL covers ?store=no, which upstream uses to keep
// one target out of persistent storage without turning it off for the rest.
func TestStoreCanBeDisabledPerURL(t *testing.T) {
	root := t.TempDir()
	notify.ConfigureStorage(root, 8, nil)
	t.Cleanup(func() { notify.ConfigureStorage("", 8, nil) })

	persisted, err := notify.ParseURL("matrixs://user:pass@example.com/%23room:example.com")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	opted, err := notify.ParseURL("matrixs://user:pass@example.com/%23room:example.com?store=no")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	// The configured target keeps what it writes; the opted-out one does not,
	// because its store lives only for as long as it is asked for.
	if err := notify.StoreFor(persisted).Set("key", "value", time.Hour); err != nil {
		t.Fatalf("store set: %v", err)
	}
	if _, ok := notify.StoreFor(persisted).Get("key"); !ok {
		t.Fatal("a configured target lost what it stored")
	}

	if err := notify.StoreFor(opted).Set("key", "value", time.Hour); err != nil {
		t.Fatalf("store set: %v", err)
	}
	if _, ok := notify.StoreFor(opted).Get("key"); ok {
		t.Fatal("store=no still persisted the value")
	}
}

// TestTimezoneMatchesUpstream covers ?tz=, which is only observable where a
// provider puts a local time in what it sends. mailto's Date header is the one
// place upstream uses it.
func TestTimezoneMatchesUpstream(t *testing.T) {
	for _, zone := range []string{"Asia/Tokyo", "America/Denver"} {
		t.Run(zone, func(t *testing.T) {
			capture := testutil.StartSMTPCapture(t)
			defer func() {
				_ = capture.Close()
			}()

			host, port, err := net.SplitHostPort(capture.Addr())
			if err != nil {
				t.Fatalf("split smtp address: %v", err)
			}

			url := fmt.Sprintf(
				"mailto://%s:%s?from=sender@example.com&to=recipient@example.com"+
					"&mode=insecure&tz=%s", host, port, zone)

			t.Setenv("PYTHONPATH", testutil.AppriseSourceRoot(t))
			script := filepath.Join(testutil.RepoRoot(t),
				"internal", "testutil", "scripts", "capture_smtp.py")
			stdout, stderr, err := testutil.RunPythonScript(t, script,
				"--url", url, "--body", "body", "--title", "title")
			if err != nil {
				t.Fatalf("python smtp send failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
			}

			var result smtpSendResult
			if err := json.Unmarshal([]byte(stdout), &result); err != nil || !result.Success {
				t.Fatalf("python smtp send reported failure: %s", strings.TrimSpace(stdout))
			}

			pythonMsgs := capture.Messages()
			if len(pythonMsgs) == 0 {
				t.Fatal("no message captured from python")
			}
			pythonOffset := dateOffset(t, pythonMsgs[len(pythonMsgs)-1].Data)
			capture.Reset()

			parsed, err := notify.ParseURL(url)
			if err != nil {
				t.Fatalf("parse mailto url: %v", err)
			}
			target, err := notify.NewMailtoTarget(parsed)
			if err != nil {
				t.Fatalf("build mailto target: %v", err)
			}
			if err := target.Send("body", "title", notify.NotifyInfo); err != nil {
				t.Fatalf("go mailto send failed: %v", err)
			}

			goMsgs := capture.Messages()
			if len(goMsgs) == 0 {
				t.Fatal("no message captured from go")
			}
			goOffset := dateOffset(t, goMsgs[len(goMsgs)-1].Data)

			if pythonOffset != goOffset {
				t.Fatalf("Date offset mismatch for %s: python=%s go=%s",
					zone, pythonOffset, goOffset)
			}

			// A test that compared two UTC offsets would pass without the
			// timezone being applied at all.
			if goOffset == "+0000" {
				t.Fatalf("%s rendered as UTC; ?tz= is not reaching the Date header", zone)
			}
		})
	}
}

// dateOffset reads the UTC offset out of a message's Date header, which is
// what the timezone changes; the instant itself is the same either way.
func dateOffset(t *testing.T, raw string) string {
	t.Helper()

	parsed, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parse message: %v", err)
	}

	date := strings.TrimSpace(parsed.Header.Get("Date"))
	if date == "" {
		t.Fatal("message carries no Date header")
	}

	when, err := mail.ParseDate(date)
	if err != nil {
		t.Fatalf("parse date %q: %v", date, err)
	}

	return when.Format("-0700")
}
