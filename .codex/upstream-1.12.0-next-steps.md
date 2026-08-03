# Upstream 1.12.0 port: what is left

Branch: `feat/upstream-1.12.0` (PR #84). Everything described as done is pushed.

Where things stand: 22 services ported this pass, missing schemas 36 → 10.
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

## Remaining 10, by what they actually cost

`kook`, `jira`, `wechat` and `ringc` are done. The token-lifecycle worry about
the latter two turned out to be smaller than it looked: both fetch their token
in the same send, so a single-shot case captures the whole flow. Neither
caches, matching what upstream does against a cold store.

### Larger (2 schemas)

`fluxer` / `fluxers`, 1122 lines. No new mechanism, just volume.

### Request signing (3 schemas)

`sogs`, `session` / `sessions` — Ed25519 plus blake2b in `X-SOGS-*` headers.

Better news than expected after reading it: `golang.org/x/crypto` is already a
dependency so blake2b is free, the protobuf message build has no random
padding, and Ed25519 signing is deterministic — which makes the whole request
*body* byte-comparable.

What is not comparable is the four `X-SOGS-*` headers: the signature covers a
random 16-byte nonce and the current timestamp. The harness compares every
`x-`-prefixed header strictly, so it needs a way to assert a header is present
without pinning its value, plus a checked-in vector — nonce and timestamp
pinned on both sides — proving the signing construction itself. That is the
Matrix technique, and it is the only thing that catches a wrong signature: the
server rejects it, a diff does not show it.

### Token lifecycle: resolved

Worth recording since the concern was raised and turned out wrong. Neither
`wechat` nor `ringc` needed a multi-send harness mode. Upstream caches in its
persistent store, the Go port has none, and a fresh store behaves identically —
so the two agree request-for-request on a single send. If a store ever lands,
these are the two to revisit.

### Needs a dependency decision from you (5 schemas)

These are blocked on a call I should not make alone, because each one costs
the static cross-compile story that makes this port deployable:

- `blink1` — USB HID. Needs a HID library with cgo, or a userspace
  reimplementation.
- `irc` / `ircs` — an IRC client library, or writing enough of the protocol.
- `xmpp` / `xmpps` — same shape, larger protocol.

Options, roughly: take the dependency and lose pure-Go static builds; build
minimal protocol clients in-tree; or declare these three out of scope and
document them as unsupported. Worth noting the schema-coverage test will keep
failing until they are either ported or explicitly excluded.

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
- `internal/tools/parity_report` — writes a "Work Outstanding" section listing
  failing providers, and the sync workflow puts it in the PR body. The drift
  to 1.12.0 happened in the first place because the sync workflow was disabled
  and had no code in it.
