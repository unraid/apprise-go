package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAttachmentIsNotSilentlyDropped is the regression guard for a real bug:
// --attach was parsed and then discarded, so a notification carrying a file
// reported success having sent nothing at all.
func TestAttachmentIsNotSilentlyDropped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(path, []byte("attachment body"), 0o600); err != nil {
		t.Fatalf("write attachment: %v", err)
	}

	var stdout, stderr bytes.Buffer
	// This service cannot carry an attachment yet, so the run must fail rather
	// than quietly deliver a notification without it. Swap the URL for another
	// unsupported service if this one gains support.
	code := Run([]string{
		"--body", "hello", "--attach", path, "gotify://localhost/token",
	}, &stdout, &stderr)

	if code == 0 {
		t.Fatalf("attaching a file to a service that cannot send one reported success; stderr=%s",
			strings.TrimSpace(stderr.String()))
	}
	if !strings.Contains(stderr.String(), "attachment") {
		t.Fatalf("failure does not mention attachments, so the cause is unclear: %s",
			strings.TrimSpace(stderr.String()))
	}
}

// TestMissingAttachmentFailsBeforeSending checks that an unreadable file stops
// the run rather than reaching some targets and not others.
func TestMissingAttachmentFailsBeforeSending(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"--body", "hello", "--attach", filepath.Join(t.TempDir(), "absent.png"), "gotify://localhost/token",
	}, &stdout, &stderr)

	if code == 0 {
		t.Fatal("a missing attachment did not fail the run")
	}
	if !strings.Contains(stderr.String(), "absent.png") {
		t.Fatalf("failure does not name the missing file: %s", strings.TrimSpace(stderr.String()))
	}
}
