package notify

import (
	"encoding/base64"
	"strings"
	"testing"

	"maunium.net/go/mautrix/crypto/goolm/session"
)

func TestMatrixIdentityProducesBothKeys(t *testing.T) {
	identity, err := newMatrixIdentity()
	if err != nil {
		t.Fatalf("new identity: %v", err)
	}

	curve, ed, err := identity.identityKeys()
	if err != nil {
		t.Fatalf("identity keys: %v", err)
	}

	for name, key := range map[string]string{"curve25519": curve, "ed25519": ed} {
		if key == "" {
			t.Fatalf("%s key is empty", name)
		}
		// Matrix keys travel as unpadded base64.
		if strings.Contains(key, "=") {
			t.Fatalf("%s key is padded: %q", name, key)
		}
		if _, err := base64.RawStdEncoding.DecodeString(key); err != nil {
			t.Fatalf("%s key is not base64: %v", name, err)
		}
	}

	if curve == ed {
		t.Fatal("curve25519 and ed25519 keys are identical")
	}
}

// A room event is only decryptable by a device holding the session key, so the
// round trip below is the property the matrix provider depends on.
func TestMatrixGroupSessionRoundTrips(t *testing.T) {
	group, err := newMatrixGroupSession()
	if err != nil {
		t.Fatalf("new group session: %v", err)
	}

	if group.sessionID() == "" {
		t.Fatal("session id is empty")
	}

	// The ratchet advances on every encrypt, and a recipient cannot decrypt a
	// message from before the key it holds, so the key must be captured first.
	sessionKey := group.sessionKey()

	plaintext := []byte(`{"msgtype":"m.text","body":"hello from apprise"}`)
	ciphertext, err := group.encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if strings.Contains(ciphertext, "hello from apprise") {
		t.Fatal("plaintext survived into the ciphertext")
	}

	inbound, err := session.NewMegolmInboundSession([]byte(sessionKey))
	if err != nil {
		t.Fatalf("new inbound session: %v", err)
	}

	decrypted, _, err := inbound.Decrypt([]byte(ciphertext))
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Fatalf("round trip mismatch: got %q want %q", decrypted, plaintext)
	}
}

func TestMatrixGroupSessionsAreDistinct(t *testing.T) {
	first, err := newMatrixGroupSession()
	if err != nil {
		t.Fatalf("new group session: %v", err)
	}
	second, err := newMatrixGroupSession()
	if err != nil {
		t.Fatalf("new group session: %v", err)
	}

	if first.sessionID() == second.sessionID() {
		t.Fatal("two sessions share an id")
	}
	if first.sessionKey() == second.sessionKey() {
		t.Fatal("two sessions share a key")
	}
}

func TestMatrixEncodeBase64IsUnpadded(t *testing.T) {
	if got := matrixEncodeBase64([]byte("abcde")); strings.Contains(got, "=") {
		t.Fatalf("encoding is padded: %q", got)
	}
}
