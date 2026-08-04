package notify_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

func TestAttachmentRequestParity(t *testing.T) {
	testutil.RequirePythonApprise(t)

	localOne := writeAttachment(t, "report.txt", "attachment body\n")
	localTwo := writeAttachment(t, "debug.bin", "\x00\x01debug body\n")
	remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	}))
	defer remoteServer.Close()
	remoteOne := remoteServer.URL + "/report.txt"

	tests := []struct {
		name             string
		rawURL           string
		attachments      []string
		compareFinalOnly bool
	}{
		{
			name:        "json local attachments",
			rawURL:      "json://example.com/notify",
			attachments: []string{localOne, localTwo},
		},
		{
			name:             "json remote attachment",
			rawURL:           "json://example.com/notify",
			attachments:      []string{remoteOne},
			compareFinalOnly: true,
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
			pythonSpecs := testutil.CapturePythonRequestsWithAttachments(
				t, tt.rawURL, body, title, notify.NotifyInfo, tt.attachments)

			goSpecs := testutil.CaptureGoRequests(t, func() error {
				attachments, err := notify.ParseAttachments(tt.attachments)
				if err != nil {
					return err
				}
				return notify.SendTargetURLWithAttachments(tt.rawURL, body, title, "text", notify.NotifyInfo, attachments)
			})

			if tt.compareFinalOnly {
				pythonSpecs = finalRequestSpec(t, pythonSpecs)
				goSpecs = finalRequestSpec(t, goSpecs)
			}
			testutil.AssertRequestSpecSequenceMatches(t, pythonSpecs, goSpecs)
		})
	}
}

func finalRequestSpec(t *testing.T, specs []notify.RequestSpec) []notify.RequestSpec {
	t.Helper()
	if len(specs) == 0 {
		t.Fatalf("no request specs captured")
	}
	return []notify.RequestSpec{specs[len(specs)-1]}
}

func writeAttachment(t *testing.T, name, body string) string {
	t.Helper()
	return testutil.WriteAttachmentFixture(t, name, body)
}
