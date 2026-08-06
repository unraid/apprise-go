package parity

var nonHTTPSchemas = map[string]struct{}{
	"aprs":   {},
	"dbus":   {},
	"gio":    {},
	"glib":   {},
	"gnome":  {},
	"growl":  {},
	"kde":    {},
	"macosx": {},
	// irc is a stateful line protocol over a raw socket, so the HTTP capture
	// harness cannot see it. It is compared against upstream by
	// irc_parity_test.go instead, which runs both implementations against
	// the server in testutil/irc_capture.go.
	"irc":     {},
	"ircs":    {},
	"mqtt":    {},
	"mqtts":   {},
	"mailto":  {},
	"mailtos": {},
	"qt":      {},
	"rsyslog": {},
	"smpp":    {},
	"smpps":   {},
	"syslog":  {},
	"windows": {},
	// XMPP is a stateful XML stream over a raw socket, so the HTTP capture
	// harness cannot see it. It is compared against upstream by
	// xmpp_parity_test.go instead, which runs both implementations against
	// the server in testutil/xmpp_capture.go.
	"xmpp":  {},
	"xmpps": {},
}

func isNonHTTPSchema(schema string) bool {
	_, ok := nonHTTPSchemas[schema]
	return ok
}
