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
		MIMEType: "text/plain",
		Data:     []byte("the attached bytes"),
	}

	specs := testutil.CaptureGoRequests(t, func() error {
		return target.SendWithAttachments("body", "title", notify.NotifyInfo,
			[]notify.Attachment{attachment})
	})
	// Discord posts the message first and the file in a second request; a
	// capture of upstream confirms both.
	if len(specs) != 2 {
		t.Fatalf("want the message and the attachment as two requests, got %d", len(specs))
	}

	if got := specs[0].Headers["Content-Type"]; got != "application/json; charset=utf-8" {
		t.Fatalf("the message request should stay JSON, got %q", got)
	}
	if !strings.Contains(specs[0].Body, "body") {
		t.Fatalf("the message request lost its content: %s", specs[0].Body)
	}

	spec := specs[1]
	if !strings.HasPrefix(spec.Headers["Content-Type"], "multipart/form-data; boundary=") {
		t.Fatalf("attachment did not switch the request to multipart: %q",
			spec.Headers["Content-Type"])
	}
	// The attachment post drops the message text rather than repeating it.
	if strings.Contains(spec.Body, `"content"`) {
		t.Fatalf("attachment request repeated the message content: %s", spec.Body)
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
		MIMEType: "application/pdf",
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
		{
			// Postmark is the only one that capitalises its keys.
			"postmark", "postmark://token:sender@example.com/user@example.com",
			[]string{`"Attachments"`, `"Content":"` + encoded + `"`, `"Name":"report.pdf"`,
				`"ContentType":"application/pdf"`},
		},
		{
			// SendPulse keys by filename instead of listing objects.
			"sendpulse", "sendpulse://user@example.com/clientid/clientsecret/target@example.com",
			[]string{`"attachments_binary"`, `"report.pdf":"` + encoded + `"`},
		},
		{
			// SMSEagle carries no filename at all.
			"smseagle", "smseagle://token@smseagle.example.com/15551234567",
			[]string{`"attachments"`, `"content":"` + encoded + `"`,
				`"content_type":"application/pdf"`},
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

			// Some providers authenticate first, so the email is not always
			// the opening request.
			bodies := make([]string, 0, len(specs))
			for _, spec := range specs {
				bodies = append(bodies, strings.ReplaceAll(spec.Body, " ", ""))
			}

			for _, want := range tc.want {
				found := false
				for _, candidate := range bodies {
					if strings.Contains(candidate, want) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("no request carries %s\nbodies: %s", want, strings.Join(bodies, "\n"))
				}
			}
		})
	}
}

// TestOffice365ReplyToCarriesEveryAddress covers what the parity fixture
// cannot: upstream keeps these in a set, so their order is undefined and a
// fixture comparing two of them is a coin flip. What matters is that none is
// lost.
func TestOffice365ReplyToCarriesEveryAddress(t *testing.T) {
	parsed, err := notify.ParseURL(
		"azure://sender@example.com/tenant123/client123/secret123/target@example.com" +
			"?reply_to=a@example.com,b@example.com")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	target, err := notify.NewOffice365Target(parsed)
	if err != nil {
		t.Fatalf("build target: %v", err)
	}

	specs := testutil.CaptureGoRequests(t, func() error {
		return target.Send("hello", "t", notify.NotifyInfo)
	})
	if len(specs) == 0 {
		t.Fatal("no request was sent")
	}

	body := specs[len(specs)-1].Body
	for _, address := range []string{"a@example.com", "b@example.com"} {
		if !strings.Contains(body, address) {
			t.Fatalf("reply-to lost %s: %s", address, body)
		}
	}
}
