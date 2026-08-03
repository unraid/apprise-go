# Upstream 1.12.0 port: what is left

Branch: `feat/upstream-1.12.0` (PR #84). Everything described as done is pushed.

Where things stand: 26 services ported this pass, missing schemas 36 → 5.
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

## Remaining 5, by what they actually cost

Everything that was a straightforward HTTP port is done. `kook`, `jira`,
`wechat`, `ringc`, `sogs`/`session`/`sessions` and `fluxer`/`fluxers` all
landed with full request parity.

### irc / ircs — no dependency needed after all (2 schemas)

Correcting an earlier note in this file: this does **not** need an IRC client
library. Upstream implements the protocol itself across `protocol.py`,
`state.py` and `client.py` — 1442 lines, no `packages_required`. So the pure-Go
static build is not at risk, and no decision is needed to start.

What it does need is real work, because IRC is stateful in a way none of the
ported providers are:

- Registration: `PASS` (server/znc auth modes only), `NICK`, `USER`, then wait
  for the welcome numeric.
- Nick collision — retry with a modified nick on 433.
- `PING`/`PONG` keepalive throughout.
- `JOIN` per channel, waiting for confirmation and handling the join-error
  numerics in `state.py`'s `JOIN_ERRORS`.
- NickServ auth mode: no `PASS` at registration, `PRIVMSG NickServ :IDENTIFY`
  afterwards.
- `PRIVMSG` per target, then `QUIT`.

Parity needs a fake IRC server rather than the HTTP harness — but that pattern
already exists. `internal/parity/smpp_parity_test.go` stands up a Go listener,
runs both implementations against it and compares the captured frames;
`capture_smpp.py` and `capture_rsyslog.py` are the Python halves. IRC is a
plain-text line protocol, so the comparison is easier than SMPP's binary PDUs.
The fake server has to answer with the right numerics or neither side gets
past registration.

Budget this as its own session. It is the largest remaining item.

### Needs a decision from you (3 schemas)

These two genuinely cost the pure-Go static build, which is why I have not
picked for you:

- `blink1` — upstream requires `hidapi`. USB HID means cgo, or reimplementing
  HID in userspace. It is a physical USB light on the machine running Apprise,
  so it is also the least likely of anything here to be used through a Go
  port in a container.
- `xmpp` / `xmpps` — upstream requires `slixmpp`, 1730 lines around it. A Go
  XMPP library exists but is a real dependency; writing enough XMPP in-tree is
  considerably more than IRC.

The options are the same for both: take the dependency, build it in-tree, or
declare them unsupported and exclude them from `TestSchemaCoverage` so the
test goes green on a documented gap rather than an open one. My read is that
`blink1` is the easiest to drop and `xmpp` the most defensible to take a
dependency for, but it is your call.

## Two things not in the schema count

- **Matrix e2ee** — crypto is done and vector-verified, harness oracle is
  ready, provider flow is not written. See `matrix-e2ee-wip.md`, which has the
  traps already paid for.
- **17 feature-backed schema drifts** — azure, discord, guilded, mailto(s),
  mastodon(s)/toot(s), matrix(s), mmost(s), o365, slack, webex, wxteams. These
  have entries but are missing 1.12.0 arguments that need real behaviour
  behind them, not just metadata.

## Guardrails that now exist

Do not remove these; each one exists because something got through.

- `registry_consistency_test.go` — the three-way registration check.
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
