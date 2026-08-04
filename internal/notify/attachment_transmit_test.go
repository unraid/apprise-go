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

// TestEmailProvidersEncodeAttachments pins the field names each service
// expects. They differ — content/filename/type, content/name, name/type/data,
// filename/fileblob/mimetype — and a wrong key is dropped by the receiver
// without complaint, so this cannot be checked by eye.
func TestEmailProvidersEncodeAttachments(t *testing.T) {
	attachment := notify.Attachment{
		Name:     "report.pdf",
		MimeType: "application/pdf",
		Data:     []byte("PDFDATA"),
	}
	encoded := "UERGREFUQQ==" // base64 of PDFDATA

	cases := []struct {
		name string
		url  string
		want []string
	}{
		{
			"sendgrid", "sendgrid://key:sender@example.com/user@example.com",
			[]string{`"attachments"`, `"content":"` + encoded + `"`, `"filename":"report.pdf"`,
				`"type":"application/octet-stream"`, `"disposition":"attachment"`},
		},
		{
			"resend", "resend://key:sender@example.com/user@example.com",
			[]string{`"attachments"`, `"content":"` + encoded + `"`, `"filename":"report.pdf"`},
		},
		{
			"brevo", "brevo://key:sender@example.com/user@example.com",
			// Singular key, and a name rather than a filename.
			[]string{`"attachment"`, `"content":"` + encoded + `"`, `"name":"report.pdf"`},
		},
		{
			"sparkpost", "sparkpost://sender@example.com/key/user@example.com",
			[]string{`"attachments"`, `"data":"` + encoded + `"`, `"name":"report.pdf"`,
				`"type":"application/pdf"`},
		},
		{
			"smtp2go", "smtp2go://key:sender@example.com/user@example.com",
			[]string{`"attachments"`, `"fileblob":"` + encoded + `"`, `"filename":"report.pdf"`,
				`"mimetype":"application/pdf"`},
		},
		{
			"mailersend", "mailersend://key@example.com/user@example.com",
			[]string{`"attachments"`, `"content":"` + encoded + `"`, `"filename":"report.pdf"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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

			specs := testutil.CaptureGoRequests(t, func() error {
				return sender.SendWithAttachments("body", "title", notify.NotifyInfo,
					[]notify.Attachment{attachment})
			})
			if len(specs) == 0 {
				t.Fatal("no request was sent")
			}

			compact := strings.ReplaceAll(specs[0].Body, " ", "")
			for _, want := range tc.want {
				if !strings.Contains(compact, want) {
					t.Fatalf("request body is missing %s\nbody: %s", want, specs[0].Body)
				}
			}
		})
	}
}
