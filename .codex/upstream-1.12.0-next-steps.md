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
- `irc` / `ircs` — no dependency needed, since upstream implements the protocol
  itself. What it needs is a stateful client (registration, nick collision,
  PING/PONG, JOIN confirmation, NickServ) and a fake IRC server for parity.
  `internal/parity/smpp_parity_test.go` is the pattern: stand up a Go listener,
  run both implementations against it, compare the frames.

`attachment_support` is *not* excluded. It mirrors what upstream declares
about the service, which the port does for every entry, and it is compared in
full. An exclusion was briefly added here on the mistaken belief that the flag
described whether this port sends attachments; it does not, and ten entries
were simply carrying wrong data.

### XMPP is unverified against upstream

`xmpp` and `xmpps` are implemented on `mellium.im/xmpp` and are listed in
`non_http_schemas.go`, so no fixture compares them to upstream. That is honest
about the harness — an XML stream over a raw socket is invisible to an HTTP
capture — but it does mean the wire format has not been checked the way every
HTTP provider has. A fake XMPP server, again modelled on the SMPP test, is
what would close that.

Not covered either: `?roster=`, `?keepalive=` and SCRAM-PLUS channel binding
(`?scramplus=`). The arguments parse and round-trip; the behaviour behind them
does not exist.

### Matrix e2ee has one harness gap

The encrypted path works and is tested Go-side — request ordering, no
plaintext leak, forged device signatures rejected — but it has no Python
fixture. Upstream's e2ee flow does not terminate under the frozen clock the
golden harness pins, so an encrypting capture hangs. See
`internal/parity/providers/matrix/README.md`. A per-provider opt-out from the
frozen clock would let that case come back.

### Worth doing next

- Attachment sending. No provider actually transmits attachments, though 42
  entries declare the service supports them. That is the largest single piece
  of upstream behaviour still missing, and it is invisible to every current
  test because nothing exercises an attachment.
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
- A manifest may declare `volatile_headers` for values that cannot be
  reproduced across runs, such as SOGS signing a random nonce and the current
  time. They are asserted present, never equal. Anything listed there owes a
  pinned vector test proving how it is built — `sogs_vectors_test.go` is the
  model — because nothing else checks its contents.
- `internal/tools/parity_report` — writes a "Work Outstanding" section listing
  failing providers, and the sync workflow puts it in the PR body. The drift
  to 1.12.0 happened in the first place because the sync workflow was disabled
  and had no code in it.
