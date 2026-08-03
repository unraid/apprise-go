# matrix parity cases

`e2ee-encrypted-room` is deliberately absent.

The golden harness pins the clock (`APPRISE_FIXED_TIME`) so captures are
reproducible. Upstream's end-to-end encryption path does not terminate under a
frozen clock — a capture of an encrypting send hangs indefinitely, while the
same URL captures in about six seconds with the clock running. Every other
matrix case, including `e2ee=no`, is unaffected.

So the encrypted path is verified on the Go side instead, and more strictly
than a request diff would manage:

- `internal/notify/matrix_e2ee_flow_test.go` drives a full encrypted send and
  asserts the request sequence, that the room key is shared *before* the
  message it unlocks, and that no plaintext appears in the encrypted body.
- `internal/notify/matrix_e2ee_test.go` asserts a forged device signature is
  rejected, which is what stops a homeserver naming a device of its choosing.
- `internal/matrixolm/vectors_test.go` pins the cryptography itself against
  constants taken from upstream.

If the harness ever grows a per-provider opt-out from the frozen clock, this
case is worth adding back.
