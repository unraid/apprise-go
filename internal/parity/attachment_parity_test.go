package parity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

// TestAttachmentParity diffs a Go attachment request against the one upstream
// produces for the same URL and file.
//
// Encoding an attachment is exactly the kind of thing that looks right and is
// wrong — a field named content rather than fileblob, a filename where a name
// belongs — and the receiver drops it without complaint. Comparing against
// upstream is the only check that catches that.
func TestAttachmentParity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(path, []byte("PDFDATA"), 0o600); err != nil {
		t.Fatalf("write attachment: %v", err)
	}

	cases := []struct {
		name string
		url  string
	}{
		{"json", "json://localhost/"},
		{"xml", "xml://localhost/"},
		{"discord", "discord://1234567890/abcdefghijklmnopqrstuvwxyz"},
		{"sendgrid", "sendgrid://key:sender@example.com/user@example.com"},
		{"brevo", "brevo://key:sender@example.com/user@example.com"},
		{"sparkpost", "sparkpost://sender@example.com/key/user@example.com"},
		{"smtp2go", "smtp2go://key:sender@example.com/user@example.com"},
		{"postmark", "postmark://token:sender@example.com/user@example.com"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pythonSpecs := testutil.CapturePythonRequestsWithAttachments(
				t, tc.url, "body", "title", notify.NotifyInfo, []string{path})

			parsed, err := notify.ParseURL(tc.url)
			if err != nil {
				t.Fatalf("parse url: %v", err)
			}
			target, err := notify.NewTarget(parsed)
			if err != nil {
				t.Fatalf("build target: %v", err)
			}
			sender, ok := target.(notify.AttachmentSender)
			if !ok {
				t.Fatal("provider does not accept attachments")
			}

			attachment, err := notify.LoadAttachment(path)
			if err != nil {
				t.Fatalf("load attachment: %v", err)
			}

			goSpecs := testutil.CaptureGoRequests(t, func() error {
				return sender.SendWithAttachments("body", "title", notify.NotifyInfo,
					[]notify.Attachment{attachment})
			})

			assertRequestSpecSequenceMatches(t, pythonSpecs, goSpecs)
		})
	}
}
