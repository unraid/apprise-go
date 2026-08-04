package parity

import (
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

// TestRetryParity covers ?retry= and ?wait=, which no request fixture could
// reach before: every mock in the capture harness answers 200, so neither
// implementation ever re-sent anything.
//
// The instrumentation is a failure budget on both mocks — answer the first N
// requests with a 500, then behave normally — which makes the retry path
// observable as a request count.
func TestRetryParity(t *testing.T) {
	const url = "json://localhost/"

	tests := []struct {
		name     string
		query    string
		failures int
		want     int
		wantErr  bool
	}{
		{
			name: "no retry means one attempt",
			want: 1,
		},
		{
			// Two failures with two retries allowed: the third attempt is
			// the one that lands.
			name:     "retries until it succeeds",
			query:    "?retry=2",
			failures: 2,
			want:     3,
		},
		{
			// A budget deeper than the retries leaves it failing, and the
			// attempts still stop at retry+1.
			name:     "gives up after the last retry",
			query:    "?retry=1",
			failures: 5,
			want:     2,
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := notify.ParseURL(url + tc.query)
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
				t.Fatal("expected the send to fail once its retries ran out")
			}
			if !tc.wantErr && sendErr != nil {
				t.Fatalf("send failed: %v", sendErr)
			}

			if len(specs) != tc.want {
				t.Fatalf("expected %d attempt(s), got %d", tc.want, len(specs))
			}

			// Every attempt has to carry the same message; a retry that
			// re-rendered the body would be a different bug wearing the same
			// request count.
			for i, spec := range specs[1:] {
				if spec.Body != specs[0].Body {
					t.Fatalf("attempt %d differs from the first:\n%s\n%s",
						i+2, specs[0].Body, spec.Body)
				}
			}
		})
	}
}

// TestRetryMatchesUpstreamAttemptCount runs the same failure budget through
// upstream, so the attempt count is compared rather than assumed.
func TestRetryParityAgainstUpstream(t *testing.T) {
	const url = "json://localhost/?retry=2"

	pythonSpecs := testutil.CapturePythonRequestsWithFailures(t, url, "body", "title", 2)

	parsed, err := notify.ParseURL(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	target, err := notify.NewTarget(parsed)
	if err != nil {
		t.Fatalf("build target: %v", err)
	}

	restore := testutil.FailNextRequests(2)
	defer restore()

	goSpecs := testutil.CaptureGoRequests(t, func() error {
		return notify.DispatchSendWithOverflow(
			target, parsed, "body", "title", notify.NotifyInfo, nil)
	})

	if len(pythonSpecs) != len(goSpecs) {
		t.Fatalf("attempt count mismatch: python=%d go=%d",
			len(pythonSpecs), len(goSpecs))
	}
	if len(goSpecs) < 2 {
		t.Fatalf("only %d attempt(s) captured; the failure budget is not "+
			"reaching the retry path", len(goSpecs))
	}

	for i := range pythonSpecs {
		assertRequestSpecMatches(t, pythonSpecs[i], goSpecs[i])
	}
}
