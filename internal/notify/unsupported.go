package notify

import "strings"

// UnsupportedSchemas are upstream schemas this port deliberately does not
// implement. Listing one is a decision rather than a shortcut: it says the
// gap is known and accepted, so anything *not* listed staying unimplemented
// remains a test failure.
//
// blink1 drives a USB HID device attached to the machine running Apprise.
// Supporting it means cgo and a HID library, which costs the pure-Go static
// build, and it is the least likely of anything here to be reached through a
// Go port running in a container.
//
// irc and ircs need no dependency — upstream implements the protocol itself —
// but they need a stateful client (registration, nick collision, PING/PONG,
// JOIN confirmation, NickServ) and a fake IRC server to verify against. That
// work is scoped in .codex/upstream-1.12.0-next-steps.md; it simply has not
// been done.
var UnsupportedSchemas = map[string]struct{}{
	"blink1": {},
}

// IsKnownGapSchema reports whether a schema is a known, accepted gap. The
// name avoids IsUnsupportedSchema, which already answers a different
// question: whether an error was an unsupported-schema error.
func IsKnownGapSchema(schema string) bool {
	_, ok := UnsupportedSchemas[strings.ToLower(strings.TrimSpace(schema))]

	return ok
}
