package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func WriteAttachmentFixture(t *testing.T, name, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write attachment fixture: %v", err)
	}
	return path
}
