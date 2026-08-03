# Matrix end-to-end encryption: state and next steps

Branch: `feat/upstream-1.12.0` (PR #84). Everything below is pushed.

## What is done

`internal/matrixolm` implements the cryptography on the Go standard library —
`crypto/ecdh`, `crypto/ed25519`, `crypto/hkdf`, `crypto/aes`, `crypto/hmac`.
No third-party dependency; the module graph is back to nine.

- `Account` — identity and signing keys, signed device keys, signed one-time
  keys, persistence via `PrivateKeys` / `NewAccountFromKeys`.
- `MegolmSession` — ratchet, message wire format, session key export.
- `OlmSession` — triple Diffie-Hellman handshake and the pre-key message that
  carries the room key to one device.
- `MarshalEvent` — event serialisation matching upstream's byte for byte.

### The verification that matters

`vectors_test.go` pins three outputs that were confirmed byte-identical to
upstream Apprise 1.12.0's own implementation: the Megolm session key, the
Megolm ciphertext, and the Olm pre-key message. Do not "fix" a failure there
by regenerating the constants. A failure means our construction drifted, and
the symptom in production is messages that every Matrix client silently
rejects rather than an error anyone sees.

To re-derive them, pin the same state in both implementations and compare:
Python's `MatrixMegOlmSession(ratchet=..., counter=..., sk_priv_b64=...)` and
Go's `NewMegolmSessionFromState`, then `MatrixOlmSession(...)` against
`Account.outboundSessionWithEphemeral`.

### Traps already paid for

- The ephemeral key is both the X3DH base key and the first ratchet key.
- The 96 byte shared secret goes into HKDF with no prefix.
- Megolm derives its keys from all 128 bytes of the ratchet, not `R[3]`.
- The Olm inner ciphertext is protobuf field 4, not field 3.
- The Olm outer pre-key message carries no MAC of its own; trailing bytes make
  strict decoders reject the session.
- Upstream encrypts the event as serialised by Python's `json.dumps` defaults:
  a space after every colon and comma, member order as constructed. Compact
  JSON encrypts fine and produces a ciphertext upstream never would. Use
  `MarshalEvent`, and pass a struct — a map gets re-sorted by `encoding/json`.

## What is left

### 1. Harness blocker, do this first

`internal/testutil/scripts/capture_request.py` now mocks `m.room.encryption`
room state, `joined_members`, `keys/upload`, `keys/query`, `keys/claim` and
`sendToDevice`, and `/login` returns a `device_id` without which upstream
refuses to upload keys at all.

Upstream now reaches `POST /keys/upload` and then does not return from
`_e2ee_setup`; a capture runs past 120 seconds. The canned upload response
(`{"one_time_key_counts":{"signed_curve25519":50}}`) evidently does not
satisfy it — suspect the one-time key replenish path looping. Reproduce with:

    APPRISE_CAPTURE_CACHE=0 .venv/bin/python \
      internal/testutil/scripts/capture_request.py \
      --url 'matrixs://user:pass@matrix.example.com/%23room:example.com?e2ee=yes' \
      --body hello --title t --type info

Until this returns, no e2ee parity case can be written: without it upstream
silently sends plaintext and a parity case would compare two unencrypted
sends and prove nothing.

### 2. Provider flow in `internal/notify/matrix.go`

Mirrors `apprise/plugins/matrix/base.py`:

- `_e2ee_setup` (base.py:1993) — create or restore the account, upload device
  keys and one-time keys. Persist through the existing storage layer so a
  notifier does not register a new device per send.
- `_e2ee_room_encrypted` (base.py:1965) — `GET /rooms/{id}/state/m.room.encryption`,
  cached; only encrypt when the room says it is encrypted.
- Device discovery — `/joined_members`, then `/keys/query`, verifying each
  device's signatures before trusting it, then `/keys/claim` for a one-time key.
- Room key sharing — one Olm pre-key message per device, delivered in an
  `m.room.encrypted` to-device event via `/sendToDevice`. Capture the Megolm
  session key **before** the first `Encrypt`: the ratchet advances per message
  and recipients cannot decrypt anything sent before the key they hold.
- Sending — `send/m.room.encrypted` with the Megolm ciphertext, rather than
  `send/m.room.message`.

### 3. Schema

`e2ee`, `hsreq`, `path` and the `hookshot` mode value are deliberately absent
from the matrix schema entry until the behaviour exists. Add them with the
implementation, not before: advertising `?e2ee=yes` while sending plaintext is
worse than the gap.

## Verification plan

1. `go test ./internal/matrixolm/` — vectors must stay green.
2. Request-sequence parity once the harness returns: the control plane
   (`/keys/upload`, `/keys/query`, `/keys/claim`, `/sendToDevice`, the
   `m.room.encrypted` send) is comparable even though key material is random
   per run, so compare structure rather than bytes for the envelope.
3. A cross-decrypt as the end-to-end check, the technique that caught the
   serialisation trap above.
