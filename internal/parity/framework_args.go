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
			"The eight services whose limits upstream computes per instance " +
			"rather than declaring — twilio's depends on ?method=, webex's " +
			"on whether it is a webhook — are resolved from the URL and " +
			"covered by fixtures of their own. Not covered: the four " +
			"plugins that take over splitting entirely (telegram, " +
			"evolution, google_chat, slack), which convert or repair markup " +
			"around the split; TestOverflowOverridesAreKnown fails when " +
			"upstream adds another. Old note: ?overflow=split makes upstream send one request per chunk and " +
			"?overflow=truncate shortens the body to the provider's " +
			"body_maxlen. The port does neither, so a split sends one " +
			"request where upstream sends several. The default mode is " +
			"upstream, which means leave the content alone, and that the " +
			"port does match. This is fixture-able: a case with " +
			"?overflow=split and a long enough body fails today with a " +
			"request count mismatch.",
	},
	"retry": {
		Implemented: true,
		Note: "Re-sends after a failure, up to retry extra attempts, in the " +
			"dispatch layer rather than in a provider — which is where " +
			"upstream keeps it too. It needed instrumentation before it " +
			"could be checked at all: every mock in the harness answers 200, " +
			"so neither side ever re-sent anything. Both capture mocks now " +
			"take a failure budget (--fail-first, FailNextRequests), and " +
			"retry_parity_test.go compares the attempt count against " +
			"upstream. Not marked fixture-covered because it needs that " +
			"budget rather than an ordinary provider case.",
	},
	"wait": {
		Implemented: true,
		Note: "How long to pause before a retry. Implemented alongside retry; " +
			"the pause itself is a sleep, so a request diff cannot see it and " +
			"only its presence in the retry loop is covered.",
	},
	"cto": {
		Implemented: true,
		Note: "Connect timeout, defaulting to upstream's four seconds. " +
			"Requests here previously went out on http.DefaultClient, which " +
			"has no timeout at all, so a service that accepted a connection " +
			"and went quiet hung the caller forever. Covered by " +
			"timeout_parity_test.go against a server that never replies; " +
			"not fixture-covered because a captured request cannot show how " +
			"long the caller was willing to wait.",
	},
	"rto": {
		Implemented: true,
		Note: "Read timeout — how long to wait for the server to start " +
			"replying. Same default and the same coverage as cto. Removing " +
			"it fails the test in four seconds with the request still " +
			"outstanding, which is what the port did on every send before " +
			"this.",
	},
	"store": {
		Implemented: true,
		Note: "?store=no keeps one target in memory even when a storage path " +
			"is configured, which is how upstream lets a URL opt out without " +
			"turning persistence off for everything else. Not fixture-" +
			"covered: whether a value survives is not visible in a request.",
	},
	"emojis": {
		Implemented:    true,
		FixtureCovered: true,
		Note: "Swaps :code: shortcuts for the character they name, in the " +
			"body and the title, before anything is measured. Upstream keys " +
			"its map by regex and compiles one alternation over 1812 " +
			"entries; the table here is those patterns expanded into 1855 " +
			"literal codes by emoji_map.py, which runs every code it " +
			"generates back through upstream before emitting it.",
	},
	"tz": {
		Implemented: true,
		Note: "Renders timestamps in the given zone. mailto's Date header is " +
			"the only place upstream consumes it, and the fixture compares " +
			"the UTC offset both sides produce for Asia/Tokyo and " +
			"America/Denver — comparing the instant would pass without the " +
			"zone being applied at all.",
	},
	"redirect": {
		Implemented: true,
		Note: "?redirect=no stops the client following a 3xx, which is then " +
			"reported as a failure rather than quietly succeeding at the " +
			"new location. Covered by timeout_parity_test.go against a " +
			"server that redirects; the capture mocks never do, so a request " +
			"fixture cannot reach it.",
	},
	"optional": {
		Implemented: true,
		Note: "?optional=yes absorbs a failure once every attempt has been " +
			"made, so a service the caller does not depend on cannot fail " +
			"the notification. It does not skip retries — upstream runs them " +
			"all and only reinterprets the final result, which the fixture " +
			"checks. Not fixture-covered: it changes the reported result " +
			"rather than the request, so it needs the failure budget.",
	},
}
