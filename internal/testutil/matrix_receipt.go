package testutil

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
)

// NormalizeMatrixReceipts verifies all signatures and decrypts ciphertext with
// a separate receiver implementation before comparing the recovered events.
func NormalizeMatrixReceipts(t *testing.T, requests []notify.RequestSpec) []notify.RequestSpec {
	t.Helper()
	for _, request := range requests {
		if !strings.Contains(request.URL, "/keys/upload") {
			continue
		}
		data, err := json.Marshal(requests)
		if err != nil {
			t.Fatal(err)
		}
		script := filepath.Join(RepoRoot(t), "internal/testutil/scripts/matrix_receipt.py")
		stdout, stderr, err := RunPythonScript(t, script, string(data))
		if err != nil {
			t.Fatalf("Matrix receipt verification failed: %v (%s)", err, stderr)
		}
		var verified []notify.RequestSpec
		if err := json.Unmarshal([]byte(stdout), &verified); err != nil {
			t.Fatalf("decode Matrix receipts: %v", err)
		}
		return verified
	}
	return requests
}
