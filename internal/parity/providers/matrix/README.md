# matrix parity cases

Encrypted rooms are covered by `TestMatrixEncryptedRequestParity` in
`internal/parity/matrix_encrypted_parity_test.go`, alongside the curated
provider cases and golden fixtures.

The capture harness fixes payload timestamps and transaction IDs at their
provider call sites. It preserves the live global clock and UUID generator so
Matrix encryption can generate distinct one-time keys and manage session
rotation normally. Only the disposable receiver keys are fixed; sender keys
and ciphertext remain random.

`internal/testutil/scripts/matrix_receipt.py` independently verifies device and
curve-key signatures, authenticates and decrypts the Olm room-key exchange,
and verifies and decrypts the Megolm message. Parity compares the recovered
room events only after those checks succeed. Fixture regression tests reject
tampered signatures, ciphertext, recipients, and missing room keys.

Additional coverage:

- `internal/notify/matrix_e2ee_flow_test.go` drives a full encrypted send and
  asserts the request sequence, that the room key is shared before the
  message it unlocks, and that no plaintext appears in the encrypted body.
- `internal/notify/matrix_e2ee_test.go` asserts a forged device signature is
  rejected.
- `internal/matrixolm/vectors_test.go` pins the cryptography against constants
  taken from upstream; the independent receiver checks those same vectors.
- `TestMatrixKeyUploadRecoveryRequestParity` in
  `internal/parity/matrix_setup_failure_test.go` compares two real sends to a
  local TLS fixture, including failed key upload, plaintext fallback, and
  successful key-upload retry.

### Encryption-state lookup failure

Once secure Matrix device-key setup succeeds, an error determining a room's
encryption state fails that room before sending text, icons, or attachments.
Only HTTP 404 confirms a plaintext room; a nonempty successful response takes
the encrypted path. Transport/server/authentication failures and empty or
malformed successful responses remain unknown and are retried on the next send.
Other rooms continue and the notification reports the failed room.

This is a deliberate security exception to Python Apprise 1.13.1, which caches
failed state lookups as false and takes its plaintext path. Successful encrypted
requests, confirmed 404 rooms, explicit plaintext transport, `e2ee=no`, and
the existing account/key-setup fallback retain their contracts. The verified
state cache uses a new namespace to exclude legacy failure-derived entries;
confirmed states keep the existing seven-day expiry. Offline regressions in
`matrix_encryption_state_test.go` cover failure/retry, no content before state
confirmation, recovery, legacy/null cache entries, and mixed-room delivery.
