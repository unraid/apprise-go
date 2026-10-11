package notify

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"testing"
)

func TestEmailReplyDeliveryRequests(t *testing.T) {
	t.Setenv("APPRISE_FIXED_TIME", "2024-01-01T00:00:00Z")
	for _, provider := range []struct{ name, rawURL string }{
		{"brevo", "brevo://apikey123:sender@example.com/recipient@example.com"},
		{"ses", "ses://sender@example.com/access123/secret123/us-east-1/recipient@example.com"},
	} {
		for _, tt := range []struct{ name, reply, senderName, expectedName string }{
			{"bare", "reply@example.com", "", sesDefaultFromName},
			{"double-quoted-bare", `"reply@example.com"`, "", sesDefaultFromName},
			{"single-quoted-bare", "'reply@example.com'", "", sesDefaultFromName},
			{"colon", "Ops:reply@example.com", "", "Ops"},
			{"spaced-colon", "Ops Team:reply@example.com", "", "Ops Team"},
			{"bracketed", "Ops Team <reply@example.com>", "", "Ops Team"},
			{"quoted-name", `"Ops Team" <reply@example.com>`, "", "Ops Team"},
			{"punctuation-name", `"Ops, Team" <reply@example.com>`, "", "Ops, Team"},
			{"unicode-name", "München <reply@example.com>", "", "München"},
			{"explicit-name", "Ops Team:reply@example.com", "Sender Name", "Sender Name"},
			{"equal-from", "Ops Team:sender@example.com", "", "Ops Team"},
		} {
			t.Run(provider.name+"/"+tt.name, func(t *testing.T) {
				rawURL := provider.rawURL + "?reply=" + strings.ReplaceAll(url.QueryEscape(tt.reply), "+", "%20")
				if tt.senderName != "" {
					rawURL += "&name=" + strings.ReplaceAll(url.QueryEscape(tt.senderName), "+", "%20")
				}
				parsed, err := ParseURL(rawURL)
				if err != nil {
					t.Fatal(err)
				}
				target, err := NewTarget(parsed)
				if err != nil {
					t.Fatal(err)
				}
				builder := target.(interface {
					BuildRequest(string, string, NotifyType) (RequestSpec, error)
				})
				expectBody := func(body string) {
					t.Helper()
					address := "reply@example.com"
					if tt.name == "equal-from" {
						address = "sender@example.com"
					}
					if provider.name == "brevo" {
						var payload struct{ ReplyTo struct{ Email string } }
						if err := json.Unmarshal([]byte(body), &payload); err != nil {
							t.Fatal(err)
						}
						if payload.ReplyTo.Email != address {
							t.Fatalf("reply destination = %q, want %q", payload.ReplyTo.Email, address)
						}
						return
					}
					form, err := url.ParseQuery(body)
					if err != nil {
						t.Fatal(err)
					}
					raw, err := base64.StdEncoding.DecodeString(form.Get("RawMessage.Data"))
					if err != nil {
						t.Fatal(err)
					}
					message, err := mail.ReadMessage(strings.NewReader(string(raw)))
					if err != nil {
						t.Fatal(err)
					}
					if tt.name == "equal-from" {
						if message.Header.Get("Reply-To") != "" {
							t.Fatal("sender's own reply address should be omitted")
						}
						return
					}
					reply, err := mail.ParseAddress(message.Header.Get("Reply-To"))
					if err != nil {
						t.Fatal(err)
					}
					if reply.Address != address || reply.Name != tt.expectedName {
						t.Fatalf("reply = %#v, want name %q/address %q", reply, tt.expectedName, address)
					}
				}
				spec, err := builder.BuildRequest("body", "title", NotifyInfo)
				if err != nil {
					t.Fatal(err)
				}
				expectBody(spec.Body)
				previous := http.DefaultClient
				previousTransport := http.DefaultTransport
				calls := 0
				transport := reviewTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					body, err := io.ReadAll(r.Body)
					if err != nil {
						return nil, err
					}
					expectBody(string(body))
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{}, Request: r}, nil
				})
				http.DefaultClient = &http.Client{Transport: transport}
				http.DefaultTransport = transport
				t.Cleanup(func() {
					http.DefaultClient = previous
					http.DefaultTransport = previousTransport
				})
				if err := target.Send("body", "title", NotifyInfo); err != nil {
					t.Fatal(err)
				}
				if calls != 1 {
					t.Fatalf("delivery requests = %d, want 1", calls)
				}
			})
		}
	}
}

func TestEmailReplyRejectsEmptyQuotedLocalPart(t *testing.T) {
	for _, provider := range []struct{ name, rawURL string }{
		{"brevo", "brevo://apikey123:sender@example.com/recipient@example.com"},
		{"ses", "ses://sender@example.com/access123/secret123/us-east-1/recipient@example.com"},
	} {
		for _, tt := range []struct{ name, reply string }{
			{"double-quoted", `""@example.com`},
			{"single-quoted", "''@example.com"},
		} {
			t.Run(provider.name+"/"+tt.name, func(t *testing.T) {
				parsed, err := ParseURL(provider.rawURL + "?reply=" + url.QueryEscape(tt.reply))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := NewTarget(parsed); err == nil {
					t.Fatal("empty quoted local part must be rejected before delivery")
				}
			})
		}
	}
}

func TestEmailReplyRejectsQuotedLocalPart(t *testing.T) {
	for _, provider := range []struct{ name, rawURL string }{
		{"brevo", "brevo://apikey123:sender@example.com/recipient@example.com"},
		{"ses", "ses://sender@example.com/access123/secret123/us-east-1/recipient@example.com"},
	} {
		for _, tt := range []struct{ name, reply string }{
			{"double-quoted", `"reply"@example.com`},
			{"single-quoted", "'reply'@example.com"},
			{"bracketed-double-quoted", `Ops <"reply"@example.com>`},
			{"bracketed-single-quoted", "Ops <'reply'@example.com>"},
		} {
			t.Run(provider.name+"/"+tt.name, func(t *testing.T) {
				parsed, err := ParseURL(provider.rawURL + "?reply=" + url.QueryEscape(tt.reply))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := NewTarget(parsed); err == nil {
					t.Fatal("quoted local part must be rejected before delivery")
				}
			})
		}
	}
}

func TestSESReplyRejectsNonASCIIAddress(t *testing.T) {
	for _, reply := range []string{
		"İreply@example.com",
		"ıreply@example.com",
		"reply@İexample.com",
		"reply@ıexample.com",
	} {
		t.Run(reply, func(t *testing.T) {
			parsed, err := ParseURL("ses://sender@example.com/access123/secret123/us-east-1/recipient@example.com?reply=" + url.QueryEscape(reply))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := NewTarget(parsed); err == nil {
				t.Fatal("non-ASCII reply addr-spec must be rejected before MIME delivery")
			}
		})
	}
}
