package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func AppriseSourceRoot(t *testing.T) string {
	t.Helper()

	if override := os.Getenv("APPRISE_SOURCE_ROOT"); override != "" {
		if fileExists(filepath.Join(override, "pyproject.toml")) {
			return override
		}
		t.Fatalf("APPRISE_SOURCE_ROOT does not point to an Apprise source repo: %s", override)
	}

	root := RepoRoot(t)
	candidate := filepath.Clean(filepath.Join(root, "..", "apprise"))

	if fileExists(filepath.Join(candidate, "pyproject.toml")) {
		return candidate
	}
	if fileExists(filepath.Join(candidate, "apprise", "cli.py")) {
		return candidate
	}

	t.Skipf("apprise source repo not found near %s", candidate)
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
