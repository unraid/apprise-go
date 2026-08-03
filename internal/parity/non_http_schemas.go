package parity

var nonHTTPSchemas = map[string]struct{}{
	"aprs":    {},
	"dbus":    {},
	"gio":     {},
	"glib":    {},
	"gnome":   {},
	"growl":   {},
	"kde":     {},
	"macosx":  {},
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
	// harness cannot see it. Verifying it needs a fake XMPP server the way
	// smpp_parity_test.go stands up a listener — see the next-steps doc.
	"xmpp":  {},
	"xmpps": {},
}

func isNonHTTPSchema(schema string) bool {
	_, ok := nonHTTPSchemas[schema]
	return ok
}
