package notify_test

import (
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

func TestAttachmentRequestParity(t *testing.T) {
	testutil.RequirePythonApprise(t)

	localOne := writeAttachment(t, "report.txt", "attachment body\n")
	localTwo := writeAttachment(t, "debug.log", "debug body\n")
	remote := "https://files.example/report.txt?name=remote.txt"

	tests := []struct {
		name        string
		rawURL      string
		attachments []string
	}{
		{
			name:        "json local attachments",
			rawURL:      "json://example.com/notify",
			attachments: []string{localOne, localTwo},
		},
		{
			name:        "json remote attachment",
			rawURL:      "json://example.com/notify",
			attachments: []string{remote},
		},
		{
			name:        "xml local attachment",
			rawURL:      "xml://example.com/notify",
			attachments: []string{localOne},
		},
		{
			name:        "apprise api json local attachment",
			rawURL:      "apprise://example.com/token?method=json",
			attachments: []string{localOne},
		},
		{
			name:        "apprise api form local attachment",
			rawURL:      "apprise://example.com/token",
			attachments: []string{localOne},
		},
		{
			name:        "form local attachment",
			rawURL:      "form://example.com/notify",
			attachments: []string{localOne},
		},
		{
			name:        "ntfy local attachment",
			rawURL:      "ntfy://example.com/topic",
			attachments: []string{localOne},
		},
		{
			name:        "ntfy multiple local attachments",
			rawURL:      "ntfy://example.com/topic",
			attachments: []string{localOne, localTwo},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := "hello"
			title := "Greeting"
			pythonSpecs := testutil.CapturePythonRequestsWithAttachments(t, tt.rawURL, body, title, tt.attachments)

			goSpecs := testutil.CaptureGoRequests(t, func() error {
				attachments, err := notify.ParseAttachments(tt.attachments)
				if err != nil {
					return err
				}
				return notify.SendTargetURLWithAttachments(tt.rawURL, body, title, "text", notify.NotifyInfo, attachments)
			})

			testutil.AssertRequestSpecSequenceMatches(t, pythonSpecs, goSpecs)
		})
	}
}

func writeAttachment(t *testing.T, name, body string) string {
	t.Helper()
	return testutil.WriteAttachmentFixture(t, name, body)
}
