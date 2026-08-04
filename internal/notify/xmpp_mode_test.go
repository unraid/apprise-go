package notify_test

import (
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
)

// TestXMPPSecureModeMatchesUpstream pins how the secure mode is resolved.
//
// XMPP is the one provider with no request fixture — it is a stateful XML
// stream over a raw socket, which the HTTP capture harness cannot see — so
// nothing was comparing this against upstream and two differences had gone
// unnoticed.
//
// The port defaulted to starttls for both schemas. Upstream only applies the
// declared default to xmpps://; a plain xmpp:// is plaintext, so the port was
// negotiating encryption where upstream sends in the clear. It also matched
// the mode exactly where upstream matches a prefix, so ?mode=start was an
// error here and starttls there.
//
// The expectations below were read off upstream directly:
//
//	xmpp://…            -> none
//	xmpps://…           -> starttls
//	xmpp://…?mode=start -> starttls
//	xmpp://…?mode=t     -> tls
func TestXMPPSecureModeMatchesUpstream(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"xmpp://user:pass@localhost/t@localhost", "none"},
		{"xmpps://user:pass@localhost/t@localhost", "starttls"},
		{"xmpp://user:pass@localhost/t@localhost?mode=start", "starttls"},
		{"xmpp://user:pass@localhost/t@localhost?mode=s", "starttls"},
		{"xmpp://user:pass@localhost/t@localhost?mode=t", "tls"},
		{"xmpp://user:pass@localhost/t@localhost?mode=tls", "tls"},
		{"xmpp://user:pass@localhost/t@localhost?mode=n", "none"},
		{"xmpps://user:pass@localhost/t@localhost?mode=none", "none"},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			parsed, err := notify.ParseURL(tc.url)
			if err != nil {
				t.Fatalf("parse url: %v", err)
			}
			target, err := notify.NewXMPPTarget(parsed)
			if err != nil {
				t.Fatalf("build target: %v", err)
			}

			if got := target.SecureMode(); got != tc.want {
				t.Fatalf("secure mode: got %q, want %q", got, tc.want)
			}
		})
	}

	// A prefix that matches nothing is still an error, the same as upstream.
	parsed, err := notify.ParseURL("xmpp://user:pass@localhost/t@localhost?mode=zzz")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if _, err := notify.NewXMPPTarget(parsed); err == nil {
		t.Fatal("an unrecognized secure mode was accepted")
	}
}
