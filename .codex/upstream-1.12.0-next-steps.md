# Upstream 1.12.0 port: what is left

Branch: `feat/upstream-1.12.0` (PR #84). Everything described as done is pushed.

Where things stand: 27 services ported this pass, missing schemas 36 → 0.
`TestProviderRequestParity` and `TestProviderGoldenRequests` have been green
after every commit, and that is the bar for anything added below.

## The recipe

Each of these took roughly fifteen minutes once the tooling existed. Deviating
from the order below is how the two registry gaps happened.

1. Read `apprise/plugins/<name>.py` upstream — the URL parsing, `send`, and
   `url()` round-trip.
2. Write `internal/notify/<name>.go`: `New<Name>Target`, `BuildRequest`,
   `Send`.
3. Generate the schema entry:
   `python3 internal/tools/schema_gen/main.py <schema> <order>`.
4. Register in **all three** places — schema entry (from step 3), the category
   registry in `registry_*.go`, and `targetBuilders` in `target.go`.
5. `go test ./internal/notify/ -run 'TestSchemaEntries|TestSupportedSchemas'`
   catches a missed registration immediately. Provider tests will *not* — they
   construct the target directly, which is exactly why a provider twice
   shipped with green tests and an unsupported URL.
6. Add `internal/parity/providers/<name>/{manifest,cases}.json`, regenerate
   goldens, run the parity suites.

Fixtures have caught a wrong assumption on nearly every provider — a missing
`charset=utf-8`, a 201 instead of 200, a default mode that was not the one in
the docs. Write the case first and let it tell you.

## What is left

The whole test suite passes. Missing schemas are zero, metadata drift is zero,
and provider request parity is green.

### Deliberate gaps

Three schemas are declared unsupported in `internal/notify/unsupported.go`,
which every suite defers to. Removing an entry there is how you turn one back
into a test failure.

- `blink1` — a USB HID device on the machine running Apprise. Supporting it
  means cgo and a HID library, costing the pure-Go static build, and it is the
  least likely of anything here to be reached through a Go port in a container.
- `irc` / `ircs` are implemented now. `testutil/irc_capture.go` completes
  registration, answers pings, confirms joins and can refuse a nick, and
  `irc_parity_test.go` runs both implementations against it and compares the
  command streams.

  One thing that surfaced is worth knowing about rather than filing away.
  Upstream declares `title_maxlen = 0`, so the framework folds the title into
  the body with a CRLF, and the plugin puts the result straight into the
  PRIVMSG. That newline ends the IRC line: everything after it is read by the
  server as a fresh command. This port reproduces it, because matching
  upstream is the contract, but it is a command-injection vector — a title or
  body carrying a newline can issue arbitrary IRC commands as the sending
  user. Diverging is a product decision rather than a porting one, so it is
  flagged here rather than quietly fixed.

  Not covered: ZNC bouncer mode (`?mode=znc`, which sends `user:password` in
  PASS and then a PING/PONG liveness check), NickServ identify beyond the
  line being sent, and `body_maxlen = 380` truncation.

`attachment_support` is *not* excluded. It mirrors what upstream declares
about the service, which the port does for every entry, and it is compared in
full. An exclusion was briefly added here on the mistaken belief that the flag
described whether this port sends attachments; it does not, and ten entries
were simply carrying wrong data.

### XMPP is now compared against upstream

`testutil/xmpp_capture.go` is a server that negotiates enough of a stream to
receive stanzas — STARTTLS with a certificate minted per run, SASL PLAIN,
resource binding, and the MUC join sequence — and `xmpp_parity_test.go` runs
both implementations against it. It speaks STARTTLS rather than plaintext
because neither client will authenticate over a socket in the clear: mellium's
SASL feature requires a secure session, and slixmpp defaults
`unencrypted_plain` and `unencrypted_scram` to off.

Three differences surfaced the moment there was something to compare against,
all of which had been invisible:

- `xmpp://` defaulted to starttls. Upstream applies the declared default only
  to `xmpps://`; a plain `xmpp://` is plaintext.
- The mode was matched exactly where upstream matches a prefix, so `?mode=start`
  was an error here and starttls there.
- A folded title was joined to the body with CRLF. Upstream writes the same
  CRLF, but XML normalizes a literal one to a newline before the recipient
  sees it, while Go's encoder escapes the carriage return as `&#xD;` — a
  character reference, which survives normalization. The recipient got an
  extra carriage return.

`?roster=` and `?scramplus=` now do something. Roster sends the contact-list
request upstream sends and the fixture compares the count both ways, so neither
side skipping it can pass as agreement. scramplus was the more interesting of
the two: it defaults to *on* upstream, so the default path offered channel
binding and this port did not — a server advertising SCRAM-SHA-256-PLUS got a
weaker mechanism here than upstream would have given it. The claim that
mellium could not do this was wrong; `mellium.im/sasl` has had ScramSha256Plus
and ScramSha1Plus all along.

Which mechanism is negotiated is still not compared against upstream. The
capture server advertises PLAIN only, and discriminating scramplus on the wire
needs a server that implements SCRAM with channel binding.

`?keepalive=` remains unimplemented. Upstream holds one session open across
sends where this port dials per notification; it is a lifecycle change rather
than a stanza change, and the capture server would need to count connections
rather than compare bytes.

The plaintext mode is unreachable in both implementations — neither will
authenticate over a socket in the clear — so there is nothing to close there.

### Matrix e2ee has one harness gap

The encrypted path works and is tested Go-side — request ordering, no
plaintext leak, forged device signatures rejected — but it has no Python
fixture. Upstream's e2ee flow does not terminate under the frozen clock the
golden harness pins, so an encrypting capture hangs. See
`internal/parity/providers/matrix/README.md`. A per-provider opt-out from the
frozen clock would let that case come back.

### Attachments are done

Every provider that advertises attachment support now transmits one, verified
against upstream. `TestAttachmentSupportIsImplemented` was written failing,
naming the providers still short, and is now a guard against the gap
reopening.

It guards two ways. A provider with request fixtures has to implement
`AttachmentSender`, and a schema advertising the flag that no fixture reaches
has to be named as covered elsewhere. mailto sat in that second gap: it
declared attachment support, sent nothing, and never appeared in the count
because the walk only reached HTTP providers. It is covered by
`mailto_attachment_parity_test.go` now, which compares the MIME tree the
message is built from against upstream's.

Adding a file to a provider is the same recipe as adding a provider, plus:

1. Implement `SendWithAttachments` — it is a separate interface from `Sender`
   on purpose, so a provider that cannot carry a file fails the type assertion
   rather than silently dropping it.
2. Add a case with `attachments` to `cases.json` and capture the golden.
3. Check the golden really carries the bytes. `TestAttachmentGoldensCarryTheirFiles`
   does this automatically and is the check that matters most: twice a mocked
   response was missing a field upstream needed, upstream quietly skipped the
   upload, and a Go implementation that also skipped it looked like agreement.

Three cases needed something beyond a golden, each documented in its own test:

- `ses` — signs its own body, which contains a per-message MIME boundary, so
  the signature cannot match by construction. The case marks `authorization`
  volatile and `ses_signature_test.go` checks the signature follows the body.
- `office365` — the large-attachment path only opens past 3MB, so pinning it
  would mean committing an oversized fixture and a golden holding its bytes.
  `office365_large_attachment_test.go` compares against upstream live instead.
- `matrix` — the encrypted path cannot be captured (see below), so
  `TestMatrixE2EEAttachmentIsEncryptedBeforeUpload` stands in, checking the
  media server never receives the plaintext.

### Worth doing next

- The persistent store now exists, so `wechat` and `ringc` could cache their
  tokens instead of refetching on every send.

## Guardrails that now exist

Do not remove these; each one exists because something got through.

- `registry_consistency_test.go` — the three-way registration check.
- `internal/notify/unsupported.go` declares the schemas the port does not
  implement, and every suite defers to it, so the reasoning lives in one place
  rather than being re-argued per test.
- `capture_request.py` cache key includes the harness script hash. Without it
  the cache served stale results and inflated failure counts.
- A case may declare `sends_nothing`, and the golden loader checks it both
  ways. Before this, an empty golden was always treated as a broken capture,
  which made "upstream deliberately sends no request" untestable — and that is
  exactly where the Opsgenie action-mapping bug was hiding.
- A case may declare `volatile_headers` of its own, merged with the
  manifest's. Only `ses/attachment` uses this, and only because it signs a
  body containing a random boundary.
- The capture pins the MIME boundary the same way it pins the clock, by
  replacing `Generator._make_boundary`. Note it is a classmethod: rebinding
  the module-level function of the same name looks right and does nothing,
  which is how the first attempt silently failed.
- `content-range` is kept when comparing headers. It was being dropped, which
  left the whole protocol of a chunked upload uncompared.
- Multipart parts are compared **in order**, by position. They used to be
  indexed into a map by field name, which ignored order and silently discarded
  repeats — a service sending several files under one field name had every
  part but the last thrown away before comparison. Build form fields with
  `formFields` (`internal/notify/form_fields.go`), not `url.Values`: a map has
  no order to emit, and upstream sends fields in the order its payload
  dictionary declares them.
- `multipart_order_test.go` tests the comparison itself. A checker that cannot
  reject anything passes every fixture, which is exactly how the old one hid.
- A manifest may declare `volatile_headers` for values that cannot be
  reproduced across runs, such as SOGS signing a random nonce and the current
  time. They are asserted present, never equal. Anything listed there owes a
  pinned vector test proving how it is built — `sogs_vectors_test.go` is the
  model — because nothing else checks its contents.
- `internal/tools/parity_report` — writes a "Work Outstanding" section listing
  failing providers, and the sync workflow puts it in the PR body. The drift
  to 1.12.0 happened in the first place because the sync workflow was disabled
  and had no code in it.
