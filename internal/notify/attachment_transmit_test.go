package notify_test

import (
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

// TestDiscordTransmitsAttachment checks the bytes actually leave. Declaring
// support and building a multipart body are not the same as the file being in
// the request, and the difference is invisible unless the body is inspected.
func TestDiscordTransmitsAttachment(t *testing.T) {
	parsed, err := notify.ParseURL("discord://1234567890/abcdefghijklmnopqrstuvwxyz")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	target, err := notify.NewDiscordTarget(parsed)
	if err != nil {
		t.Fatalf("build target: %v", err)
	}

	attachment := notify.Attachment{
		Name:     "report.txt",
		MimeType: "text/plain",
		Data:     []byte("the attached bytes"),
	}

	specs := testutil.CaptureGoRequests(t, func() error {
		return target.SendWithAttachments("body", "title", notify.NotifyInfo,
			[]notify.Attachment{attachment})
	})
	if len(specs) != 1 {
		t.Fatalf("want one request, got %d", len(specs))
	}
	spec := specs[0]

	if !strings.HasPrefix(spec.Headers["Content-Type"], "multipart/form-data; boundary=") {
		t.Fatalf("attachment did not switch the request to multipart: %q",
			spec.Headers["Content-Type"])
	}
	for _, want := range []string{
		`name="payload_json"`,
		`name="files[0]"`,
		`filename="report.txt"`,
		"text/plain",
		"the attached bytes",
	} {
		if !strings.Contains(spec.Body, want) {
			t.Fatalf("request body is missing %q", want)
		}
	}
}

// TestDiscordWithoutAttachmentStaysJSON guards the other direction: adding
// attachment support must not change an ordinary notification.
func TestDiscordWithoutAttachmentStaysJSON(t *testing.T) {
	parsed, err := notify.ParseURL("discord://1234567890/abcdefghijklmnopqrstuvwxyz")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	target, err := notify.NewDiscordTarget(parsed)
	if err != nil {
		t.Fatalf("build target: %v", err)
	}

	specs := testutil.CaptureGoRequests(t, func() error {
		return target.Send("body", "title", notify.NotifyInfo)
	})
	if len(specs) != 1 {
		t.Fatalf("want one request, got %d", len(specs))
	}
	if got := specs[0].Headers["Content-Type"]; got != "application/json; charset=utf-8" {
		t.Fatalf("a plain notification changed content type: %q", got)
	}
}
