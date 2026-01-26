package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
)

func TestStorageListActive(t *testing.T) {
	tempDir := t.TempDir()
	url := "json://user:pass@example.com"

	parsed, err := notify.ParseURL(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	uid := notify.URLID(parsed, defaultStorageUIDLength, nil)
	if uid == "" {
		t.Fatalf("empty url id")
	}

	dataDir := filepath.Join(tempDir, uid, "var")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, storageCacheFile), []byte("x"), 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	t.Setenv(defaultEnvAppriseURLs, url)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"--storage-path", tempDir, "storage"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("storage list failed: code=%d stdout=%s stderr=%s", code, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}

	output := stdout.String()
	if !strings.Contains(output, uid) {
		t.Fatalf("expected uid in output: %s", output)
	}
	if !strings.Contains(output, "active") {
		t.Fatalf("expected active state in output: %s", output)
	}
}
