package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unraid/apprise-go/internal/notify"
)

func TestStorageListActive(t *testing.T) {
	tempDir := t.TempDir()
	url := "json://user:pass@example.com"

	uid := urlID(t, url)
	cachePath := writeCacheFile(t, tempDir, uid)
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
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache file missing: %v", err)
	}
}

func TestStorageListStale(t *testing.T) {
	tempDir := t.TempDir()
	uid := "abc12345"

	if err := os.MkdirAll(filepath.Join(tempDir, uid), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"--storage-path", tempDir, "storage"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("storage list failed: code=%d stdout=%s stderr=%s", code, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}

	output := stdout.String()
	if !strings.Contains(output, uid) {
		t.Fatalf("expected stale uid in output: %s", output)
	}
	if !strings.Contains(output, "stale") {
		t.Fatalf("expected stale state in output: %s", output)
	}
}

func TestStorageListTagFilter(t *testing.T) {
	tempDir := t.TempDir()
	url := "json://user:pass@example.com"
	uid := urlID(t, url)
	writeCacheFile(t, tempDir, uid)

	configPath := filepath.Join(tempDir, "apprise.conf")
	if err := os.WriteFile(configPath, []byte("ops="+url+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"--storage-path", tempDir, "--config", configPath, "--tag", "ops", "storage"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("storage list failed: code=%d stdout=%s stderr=%s", code, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}

	output := stdout.String()
	if !strings.Contains(output, uid) {
		t.Fatalf("expected uid in output: %s", output)
	}
	if !strings.Contains(output, "tags") || !strings.Contains(output, "ops") {
		t.Fatalf("expected tags in output: %s", output)
	}
}

func TestStoragePruneRemovesExpired(t *testing.T) {
	tempDir := t.TempDir()
	url := "json://user:pass@example.com"
	uid := urlID(t, url)
	cachePath := writeCacheFile(t, tempDir, uid)

	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(cachePath, past, past); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"--storage-path", tempDir, "--storage-prune-days", "1", "storage", "prune"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("storage prune failed: code=%d stdout=%s stderr=%s", code, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}

	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("expected cache file removed, err=%v", err)
	}
}

func TestStoragePruneDryRunKeepsFiles(t *testing.T) {
	tempDir := t.TempDir()
	url := "json://user:pass@example.com"
	uid := urlID(t, url)
	cachePath := writeCacheFile(t, tempDir, uid)

	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(cachePath, past, past); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"--storage-path", tempDir, "--storage-prune-days", "1", "--dry-run", "storage", "prune"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("storage prune failed: code=%d stdout=%s stderr=%s", code, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}

	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("expected cache file to remain, err=%v", err)
	}
}

func TestStorageClearRemovesAll(t *testing.T) {
	tempDir := t.TempDir()
	url := "json://user:pass@example.com"
	uid := urlID(t, url)
	cachePath := writeCacheFile(t, tempDir, uid)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"--storage-path", tempDir, "storage", "clear"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("storage clear failed: code=%d stdout=%s stderr=%s", code, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}

	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("expected cache file removed, err=%v", err)
	}
}

func urlID(t *testing.T, url string) string {
	t.Helper()

	parsed, err := notify.ParseURL(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	uid := notify.URLID(parsed, defaultStorageUIDLength, nil)
	if uid == "" {
		t.Fatalf("empty url id")
	}
	return uid
}

func writeCacheFile(t *testing.T, root, uid string) string {
	t.Helper()

	dataDir := filepath.Join(root, uid, "var")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cachePath := filepath.Join(dataDir, storageCacheFile)
	if err := os.WriteFile(cachePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	return cachePath
}
