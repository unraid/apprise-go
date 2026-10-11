package parity

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

func TestMatrixEncryptedRequestParity(t *testing.T) {
	for _, url := range []string{
		"matrixs://user:pass@example.com:1/!room",
		"matrixs://token@example.com:1/!room",
		"matrixs://user:pass@example.com:1/!room?version=2",
		"matrix://user:pass@example.com:1/!room",
		"matrix://user:pass@example.com:1/!room?version=2&image=yes",
		"matrixs://user:pass@example.com:1/!room?hsreq=no",
	} {
		t.Run(url, func(t *testing.T) {
			pythonSpecs, success := testutil.CapturePythonRequestsWithTypeResult(t, url, "secret body", "secret title", "", notify.NotifyInfo)
			parsed, err := notify.ParseURL(url)
			if err != nil {
				t.Fatal(err)
			}
			goSpecs, err := testutil.CaptureGoRequestsResult(t, func() error {
				// The Python capture has a fresh per-instance memory store.
				parsed.Query["store"] = "no"
				target, err := notify.NewMatrixTarget(parsed)
				if err != nil {
					return err
				}
				return target.Send("secret body", "secret title", notify.NotifyInfo)
			})
			if err != nil || success == nil || !*success {
				t.Fatalf("send failed: python=%v go=%v", success, err)
			}
			assertRequestSpecSequenceMatches(t, pythonSpecs, goSpecs)
			if url == "matrixs://user:pass@example.com:1/!room" {
				data, err := json.Marshal(goSpecs)
				if err != nil {
					t.Fatal(err)
				}
				script := filepath.Join(testutil.RepoRoot(t), "internal/testutil/scripts/contract_fixtures_test.py")
				stdout, stderr, err := testutil.RunPythonScript(t, script, string(data))
				if err != nil {
					t.Fatalf("fixture regression checks failed: %v (%s%s)", err, stdout, stderr)
				}
			}
		})
	}
}
