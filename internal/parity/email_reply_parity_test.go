package parity

import (
	"net/url"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

// Compare the complete delivery request, including the normalized reply address
// and SES sender-name precedence, against the pinned Python implementation.
func TestEmailReplyRequestParity(t *testing.T) {
	testutil.RequirePythonApprise(t)
	cases := []struct {
		name  string
		reply string
		from  string
	}{
		{"absent", "", ""},
		{"bare", "reply@example.com", ""},
		{"double-quoted-bare", `"reply@example.com"`, ""},
		{"single-quoted-bare", "'reply@example.com'", ""},
		{"double-quote-empty-local", `""@example.com`, ""},
		{"single-quote-empty-local", "''@example.com", ""},
		{"double-quoted-local", `"reply"@example.com`, ""},
		{"single-quoted-local", "'reply'@example.com", ""},
		{"bracketed-double-quoted-local", `Ops <"reply"@example.com>`, ""},
		{"bracketed-single-quoted-local", "Ops <'reply'@example.com>", ""},
		{"dot-apostrophe", "reply.o'connor@example.com", ""},
		{"plus-label", "ops+reply@example.com", ""},
		{"bracketed-dot-apostrophe", "Ops <reply.o'connor@example.com>", ""},
		{"double-quote-leading-only", `"reply@example.com`, ""},
		{"double-quote-trailing-only", `reply@example.com"`, ""},
		{"named-plus-label", "Ops Team:ops+reply@example.com", ""},
		{"nbsp-name", "Ops\u00a0reply@example.com", ""},
		{"emspace-name", "Ops\u2003reply@example.com", ""},
		{"vertical-tab-name", "Ops\vreply@example.com", ""},
		{"control-separator-name", "Ops\u001creply@example.com", ""},
		{"turkish-uppercase-user", "İreply@example.com", ""},
		{"turkish-dotless-user", "ıreply@example.com", ""},
		{"turkish-uppercase-domain", "reply@İexample.com", ""},
		{"turkish-dotless-domain", "reply@ıexample.com", ""},
		{"colon", "Ops:reply@example.com", ""},
		{"spaced-colon", "Ops Team:reply@example.com", ""},
		{"bracketed", "Ops Team <reply@example.com>", ""},
		{"brackets-only", "<reply@example.com>", ""},
		{"quoted-name", `"Ops Team" <reply@example.com>`, ""},
		{"punctuation-name", `"Ops, Team" <reply@example.com>`, ""},
		{"unicode-name", "München <reply@example.com>", ""},
		{"explicit-from-name", "Ops Team:reply@example.com", "Sender Name"},
		{"equal-from", "Ops Team:sender@example.com", ""},
		{"malformed-colon", "Name:Name:reply@example.com", ""},
		{"invalid", "not-an-email", ""},
	}
	for _, provider := range []struct {
		name string
		url  string
	}{
		{"brevo", "brevo://apikey123:sender@example.com/recipient@example.com"},
		{"ses", "ses://sender@example.com/access123/secret123/us-east-1/recipient@example.com"},
	} {
		for _, tt := range cases {
			t.Run(provider.name+"/"+tt.name, func(t *testing.T) {
				query := url.Values{}
				if tt.reply != "" {
					query.Set("reply", tt.reply)
				}
				if tt.from != "" {
					query.Set("name", tt.from)
				}
				rawURL := provider.url
				if len(query) > 0 {
					rawURL += "?" + strings.ReplaceAll(query.Encode(), "+", "%20")
				}
				pythonSpecs, pythonSuccess := testutil.CapturePythonRequestsWithTypeResult(t, rawURL, "body", "title", "", notify.NotifyInfo)
				parsed, err := notify.ParseURL(rawURL)
				if err != nil {
					t.Fatal(err)
				}
				goSpecs, err := testutil.CaptureGoRequestsResult(t, func() error {
					target, err := notify.NewTarget(parsed)
					if err != nil {
						return err
					}
					return notify.DispatchSendWithInput(target, parsed, "body", "title", "", notify.NotifyInfo, nil)
				})
				if pythonSuccess == nil {
					t.Fatal("Python capture omitted the delivery outcome")
				}
				assertNotifySuccessMatches(t, pythonSuccess, err)
				assertRequestSpecSequenceMatches(t, pythonSpecs, goSpecs)
			})
		}
	}
}
