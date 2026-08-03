package notify

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"maunium.net/go/mautrix/crypto/goolm/account"
	"maunium.net/go/mautrix/crypto/goolm/session"
)

// Matrix room encryption is Megolm: the sender creates an outbound group
// session, shares its key with each recipient device over Olm, and then
// encrypts every room event with that session. This file holds the primitives
// the matrix provider builds on; goolm supplies the cryptography so no libolm
// or cgo is required and the release matrix keeps cross-compiling.

// matrixIdentity is the long lived key pair that identifies this sender to a
// homeserver. Upstream persists it; we generate one per process because a
// notifier is short lived and never receives replies.
type matrixIdentity struct {
	account *account.Account
}

func newMatrixIdentity() (*matrixIdentity, error) {
	acc, err := account.NewAccount()
	if err != nil {
		return nil, fmt.Errorf("create matrix account: %w", err)
	}

	return &matrixIdentity{account: acc}, nil
}

// identityKeys returns the curve25519 and ed25519 keys a homeserver expects
// when a device uploads itself.
func (m *matrixIdentity) identityKeys() (curve25519, ed25519 string, err error) {
	keys, err := m.account.IdentityKeysJSON()
	if err != nil {
		return "", "", fmt.Errorf("read identity keys: %w", err)
	}

	var parsed struct {
		Curve25519 string `json:"curve25519"`
		Ed25519    string `json:"ed25519"`
	}
	if err := json.Unmarshal(keys, &parsed); err != nil {
		return "", "", fmt.Errorf("decode identity keys: %w", err)
	}

	return parsed.Curve25519, parsed.Ed25519, nil
}

// matrixGroupSession encrypts room events for one room.
type matrixGroupSession struct {
	outbound *session.MegolmOutboundSession
}

func newMatrixGroupSession() (*matrixGroupSession, error) {
	outbound, err := session.NewMegolmOutboundSession()
	if err != nil {
		return nil, fmt.Errorf("create megolm session: %w", err)
	}

	return &matrixGroupSession{outbound: outbound}, nil
}

// sessionID identifies this session to recipients.
func (g *matrixGroupSession) sessionID() string {
	return string(g.outbound.ID())
}

// sessionKey is what recipient devices need in order to decrypt; it is shared
// over Olm rather than sent in the clear.
//
// The ratchet advances on every encrypt and a recipient cannot decrypt any
// message sent before the key it holds, so callers must capture the key before
// encrypting anything with the session.
func (g *matrixGroupSession) sessionKey() string {
	return g.outbound.Key()
}

// encrypt returns the ciphertext for a room event payload.
func (g *matrixGroupSession) encrypt(plaintext []byte) (string, error) {
	ciphertext, err := g.outbound.Encrypt(plaintext)
	if err != nil {
		return "", fmt.Errorf("megolm encrypt: %w", err)
	}

	return string(ciphertext), nil
}

// matrixEncodeBase64 matches the unpadded base64 Matrix uses on the wire.
func matrixEncodeBase64(value []byte) string {
	return base64.RawStdEncoding.EncodeToString(value)
}
