package parity

// frameworkArg records what this port actually does with an argument every
// provider inherits from upstream's NotifyBase.
//
// These are the knobs a plugin declares but does not implement, because the
// base class acts on them: overflow, the timeouts, retry, and so on. They are
// the easiest thing in the whole port to miss, and were missed. Every schema
// entry carries them, so TestSchemaDetailsParity compares the declarations and
// passes — which says the labels agree and nothing about whether the behavior
// behind them exists. Reading a plugin never reveals it either, because the
// plugin genuinely does not mention them.
type FrameworkArg struct {
	// implemented is whether the port acts on the argument rather than only
	// declaring it in every schema entry.
	Implemented bool

	// fixtureCovered is whether a request fixture exercises it. Only an
	// argument whose effect shows up in a request can be covered this way;
	// a sleep or a socket timeout cannot.
	FixtureCovered bool

	// note explains anything not both implemented and covered. Required in
	// that case, so a gap cannot be recorded without saying what it is.
	Note string
}

// frameworkArgs is the claim this file checks. Adding an entry is a statement
// about the port that the test then holds you to.
var FrameworkArgs = map[string]FrameworkArg{
	"format": {
		Implemented:    true,
		FixtureCovered: true,
	},
	"verify": {
		Implemented: true,
		Note: "TLS verification is a property of the connection, not of the " +
			"request, so a captured request looks identical either way. " +
			"It is exercised by the xmpp and irc fixtures, which need " +
			"verify=no to accept the capture server's own certificate.",
	},

	// --- declared, not implemented -------------------------------------
	"overflow": {
		Implemented:    true,
		FixtureCovered: true,
		Note: "Implemented for the framework default: ?overflow=truncate " +
			"shortens the body to the service's body_maxlen and " +
			"?overflow=split sends one notification per chunk, splitting on " +
			"newlines, then spaces, then punctuation. Verified against " +
			"upstream by the 46elks and discord fixtures, which take " +
			"different branches of the sizing logic — discord repeats the " +
			"title with a [1/2] counter where 46elks folds it into the body. " +
			"Not covered: services that override the framework's splitting, " +
			"telegram being one, and the fifteen whose body_maxlen upstream " +
			"computes per instance rather than declaring, which " +
			"overflow_limits.py reports as null and ApplyOverflow leaves " +
			"alone. Old note: ?overflow=split makes upstream send one request per chunk and " +
			"?overflow=truncate shortens the body to the provider's " +
			"body_maxlen. The port does neither, so a split sends one " +
			"request where upstream sends several. The default mode is " +
			"upstream, which means leave the content alone, and that the " +
			"port does match. This is fixture-able: a case with " +
			"?overflow=split and a long enough body fails today with a " +
			"request count mismatch.",
	},
	"retry": {
		Note: "?retry= asks upstream to re-send after a failure response. " +
			"Not fixture-able as things stand: the capture mocks answer 200, " +
			"so no retry path is ever entered on either side. Catching it " +
			"needs a mock that can fail on demand.",
	},
	"wait": {
		Note: "?wait= is how long upstream sleeps between retries. Invisible " +
			"to a request diff, and only reachable once retry exists.",
	},
	"cto": {
		Note: "Socket connect timeout. A captured request cannot show it; " +
			"observing it needs a server that refuses to complete a " +
			"handshake.",
	},
	"rto": {
		Note: "Socket read timeout. Same as cto — it needs a server that " +
			"stalls, not a request diff.",
	},
	"store": {
		Note: "Persistent storage is configured process-wide by the CLI " +
			"rather than per URL, so ?store=no does not turn it off for one " +
			"target the way upstream does.",
	},
	"emojis": {
		Note: "?emojis=yes has upstream translate :smile: style codes in the " +
			"body before sending. The port passes the body through, so the " +
			"request differs whenever a body contains one. Fixture-able.",
	},
	"tz": {
		Note: "?tz= sets the timezone upstream renders timestamps in. Only " +
			"visible for providers that put a local time in the payload.",
	},
	"redirect": {
		Note: "?redirect=no stops upstream following HTTP redirects. The " +
			"capture mocks never redirect, so neither side is exercised.",
	},
	"optional": {
		Note: "?optional=yes marks a service whose failure should not fail " +
			"the notification overall. It changes the reported result rather " +
			"than the request.",
	},
}
