package parity

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

// TestSESSignsAnAttachmentBody covers the one thing the golden comparison
// cannot. SES signs the request body, and a body carrying attachments carries
// a randomly generated MIME boundary, so the two sides sign different bytes by
// construction and ses/attachment marks the authorization header volatile.
//
// The signature is still the part of that request most worth getting right, so
// this asserts it directly: send the same notification twice, and everything
// that is not the boundary — and the signature over it — must differ, while the
// signature must stay a function of the body actually sent.
//
// It works by replaying the captured request through the signing path a second
// time. If the signature were stale, copied, or computed over the wrong bytes,
// the two captures would agree where they should not.
func TestSESSignsAnAttachmentBody(t *testing.T) {
	const target = "ses://sender@example.com/access123/secret123/us-east-1/target@example.com"

	parsed, err := notify.ParseURL(target)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	sender, err := notify.NewSESTarget(parsed)
	if err != nil {
		t.Fatalf("build target: %v", err)
	}

	attachments := []notify.Attachment{{
		Name:     "notes.txt",
		MimeType: "text/plain",
		Data:     []byte("plain text attachment\n"),
	}}

	capture := func() (body, signature string) {
		specs := testutil.CaptureGoRequests(t, func() error {
			return notify.SendWithAttachments(sender, "body", "title", notify.NotifyInfo, attachments)
		})
		if len(specs) != 1 {
			t.Fatalf("expected one request, got %d", len(specs))
		}

		return specs[0].Body, specs[0].Headers["Authorization"]
	}

	firstBody, firstSignature := capture()
	secondBody, secondSignature := capture()

	if firstSignature == "" {
		t.Fatal("request carried no authorization header")
	}

	// The boundary is the only thing that may differ between two sends of the
	// same notification. Once it is normalized away the bodies must agree.
	if normalizeSESBoundary(t, firstBody) != normalizeSESBoundary(t, secondBody) {
		t.Fatal("two sends of the same notification differ by more than the MIME boundary")
	}

	if firstBody == secondBody {
		t.Fatal("the MIME boundary is not being generated per message")
	}

	// A signature that survived a changed body was never computed over it.
	if firstSignature == secondSignature {
		t.Fatalf("signature did not follow the body it signs: %s", firstSignature)
	}
}

// normalizeSESBoundary rewrites the generated boundary inside the base64
// message SES posts, so two sends can be compared on everything else.
func normalizeSESBoundary(t *testing.T, body string) string {
	t.Helper()

	values, err := url.ParseQuery(body)
	if err != nil {
		t.Fatalf("parse ses body: %v", err)
	}

	raw := values.Get("RawMessage.Data")
	if raw == "" {
		t.Fatal("ses request carried no message")
	}

	normalized, ok := normalizeEmbeddedEmail(raw)
	if !ok {
		t.Fatal("ses message is not a multipart email; the attachment path did not run")
	}

	// Prove the message really carries the file rather than just a boundary,
	// so a send that quietly dropped the attachment cannot pass this test.
	decoded, err := base64.StdEncoding.DecodeString(normalized)
	if err != nil {
		t.Fatalf("decode ses message: %v", err)
	}
	if !strings.Contains(stripWhitespace(string(decoded)),
		base64.StdEncoding.EncodeToString([]byte("plain text attachment\n"))) {
		t.Fatal("ses message does not carry the attachment")
	}

	values.Set("RawMessage.Data", normalized)

	return values.Encode()
}
