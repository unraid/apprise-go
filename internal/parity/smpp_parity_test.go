package parity

import (
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

func TestSMPPParity(t *testing.T) {
	capture := testutil.StartSMPPCapture(t)
	defer func() {
		_ = capture.Close()
	}()

	host, port, err := net.SplitHostPort(capture.Addr())
	if err != nil {
		t.Fatalf("smpp host split failed: %v", err)
	}

	cases := []string{"smpp", "smpps"}
	body := "apprise parity body"

	for _, schema := range cases {
		schema := schema
		t.Run(schema, func(t *testing.T) {
			url := fmt.Sprintf("%s://user:pass@%s:%s/15551231234/15551230000", schema, host, port)

			capture.Reset()
			pythonSuccess, pythonErr := runPythonSMPP(t, url, body, "", port)
			if !pythonSuccess {
				t.Fatalf("python smpp send reported failure: %s", pythonErr)
			}
			pythonMessages := waitForSMPPMessages(t, capture, 1)

			capture.Reset()
			parsed, err := notify.ParseURL(url)
			if err != nil {
				t.Fatalf("parse smpp url: %v", err)
			}
			target, err := notify.NewSMPPTarget(parsed)
			if err != nil {
				t.Fatalf("build smpp target: %v", err)
			}
			if err := target.Send(body, "", notify.NotifyInfo); err != nil {
				t.Fatalf("go smpp send failed: %v", err)
			}
			goMessages := waitForSMPPMessages(t, capture, 1)

			if len(pythonMessages) != len(goMessages) {
				t.Fatalf("smpp message count mismatch: python=%d go=%d", len(pythonMessages), len(goMessages))
			}
			for i := range pythonMessages {
				py := pythonMessages[i]
				goMsg := goMessages[i]
				if py.Source != goMsg.Source {
					t.Fatalf("smpp source mismatch: python=%s go=%s", py.Source, goMsg.Source)
				}
				if py.Destination != goMsg.Destination {
					t.Fatalf("smpp destination mismatch: python=%s go=%s", py.Destination, goMsg.Destination)
				}
				if py.Body != goMsg.Body {
					t.Fatalf("smpp body mismatch: python=%q go=%q", py.Body, goMsg.Body)
				}
			}
		})
	}
}

func runPythonSMPP(t *testing.T, url, body, title, port string) (bool, string) {
	t.Helper()

	appriseRoot := testutil.AppriseSourceRoot(t)
	t.Setenv("PYTHONPATH", appriseRoot)

	script := filepath.Join(testutil.RepoRoot(t), "internal", "testutil", "scripts", "capture_smpp.py")
	stdout, stderr, err := testutil.RunPythonScript(t, script,
		"--url", url,
		"--body", body,
		"--title", title,
		"--port", port,
	)
	if err != nil {
		t.Fatalf("python smpp send failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	var result notifyResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse python smpp result failed: %v (stdout: %s)", err, strings.TrimSpace(stdout))
	}
	return result.Success, strings.TrimSpace(result.Error)
}

func waitForSMPPMessages(t *testing.T, capture *testutil.SMPPCapture, count int) []testutil.SMPPMessage {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		messages := capture.Messages()
		if len(messages) >= count {
			return messages
		}
		if time.Now().After(deadline) {
			t.Fatalf("smpp messages not captured (expected %d)", count)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
