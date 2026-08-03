package matrixolm

import (
	"crypto/ecdh"
	"testing"
)

// These vectors were produced by upstream Apprise 1.12.0's own Olm and Megolm
// implementation and confirmed byte-identical to ours. They are checked in so
// a change to our construction fails here rather than silently producing
// messages no Matrix client can decrypt.
const (
	upstreamOlmPreKeyMessage = "Awog4Fsa4yICSDgJX/3v2GVGjdt3sUst0nUXXqCrQpT6bXoSIH2cJDFlOYJcGJblfygZd0Z5POYMvuOtR9qdB7hfpV4qGiAHo3y8FCCTyLdV3BsQ6Gy0JjdK0WqoU+0L38CyuG0cfCJfAwogfZwkMWU5glwYluV/KBl3Rnk85gy+461H2p0HuF+lXioQACIwWPzx6q7ASx+etnhbGVmOdjon0KqKtv4vUAPHlltbkvMM0OecQVF0N6pvuQsDFTTeIiD+MoRSAQU"
	upstreamMegolmSessionKey = "AgAAAAAAAQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyAhIiMkJSYnKCkqKywtLi8wMTIzNDU2Nzg5Ojs8PT4/QEFCQ0RFRkdISUpLTE1OT1BRUlNUVVZXWFlaW1xdXl9gYWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXp7fH1+f7r8cb6tOsXktj6cghbucaNKrsZXIu7bynKLTps8zOOW2sfxfDg1xFrrIVkF0ZhnM5IZZxAIyf6HHYVwgHT1uAWw5DA1gfXQYezLiLNVJgi6tq8Fw03Bh9gc+LuWpaZNAA"
	upstreamMegolmCiphertext = "AwgAEjDZAAdm6mLmU2ua7CxhBCeC7wQIRVsxpnysuaFIzIRVCnlPsm9jdHDht8eWHxAeb3K2VCdtsL9pm3mTNKd0DqJWNdxI7vh3Wvw/t5llFZHobwR1FhGCkDKy1BvGVe+HqyfujLG+1v7hlhgXJCO4wr1pPx9qeM+jlgA"
)

func seededKey(t *testing.T, seed []byte) *ecdh.PrivateKey {
	t.Helper()
	key, err := ecdh.X25519().NewPrivateKey(seed)
	if err != nil {
		t.Fatalf("seed x25519 key: %v", err)
	}
	return key
}

func filledSeed(fill func(i int) byte) []byte {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = fill(i)
	}
	return seed
}

func TestOlmPreKeyMessageMatchesUpstream(t *testing.T) {
	account, err := NewAccountFromKeys(filledSeed(func(i int) byte { return byte(i + 1) }), make([]byte, 32))
	if err != nil {
		t.Fatalf("account: %v", err)
	}

	ephemeral := seededKey(t, filledSeed(func(i int) byte { return byte(100 + i) }))
	theirIdentity := seededKey(t, filledSeed(func(i int) byte { return byte(200 - i) }))
	theirOneTime := seededKey(t, filledSeed(func(i int) byte { return byte(50 + i) }))

	session, err := account.outboundSessionWithEphemeral(ephemeral, theirIdentity.PublicKey(), theirOneTime.PublicKey())
	if err != nil {
		t.Fatalf("outbound session: %v", err)
	}

	message, err := session.Encrypt([]byte(`{"algorithm": "m.megolm.v1.aes-sha2"}`))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if message.Type != 0 {
		t.Fatalf("message type is %d, want 0 for a pre-key message", message.Type)
	}
	if message.Body != upstreamOlmPreKeyMessage {
		t.Fatalf("olm pre-key message diverged from upstream:\ngot  %s\nwant %s", message.Body, upstreamOlmPreKeyMessage)
	}
}

func TestMegolmOutputMatchesUpstream(t *testing.T) {
	session, err := NewMegolmSessionFromState(fixedRatchet(), 0, fixedSigningSeed())
	if err != nil {
		t.Fatalf("restore session: %v", err)
	}

	if key := session.SessionKey(); key != upstreamMegolmSessionKey {
		t.Fatalf("session key diverged from upstream:\ngot  %s\nwant %s", key, upstreamMegolmSessionKey)
	}

	plaintext, err := MarshalEvent(struct {
		Body    string `json:"body"`
		MsgType string `json:"msgtype"`
	}{"cross check", "m.text"})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	ciphertext, err := session.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if ciphertext != upstreamMegolmCiphertext {
		t.Fatalf("megolm ciphertext diverged from upstream:\ngot  %s\nwant %s", ciphertext, upstreamMegolmCiphertext)
	}
}

func TestOlmSessionAdvancesChainPerMessage(t *testing.T) {
	account, err := NewAccount()
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	peer, err := NewAccount()
	if err != nil {
		t.Fatalf("peer account: %v", err)
	}
	otks, err := peer.GenerateOneTimeKeys("@peer:example.com", "PEER", 1)
	if err != nil {
		t.Fatalf("one time keys: %v", err)
	}

	var oneTimeKey string
	for _, object := range otks {
		oneTimeKey = object.(map[string]any)["key"].(string)
	}

	session, err := account.NewOutboundSession(peer.IdentityKey(), oneTimeKey)
	if err != nil {
		t.Fatalf("outbound session: %v", err)
	}
	if session.TheirIdentityKey() != peer.IdentityKey() {
		t.Fatal("session addressed the wrong device")
	}

	first, err := session.Encrypt([]byte("one"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	second, err := session.Encrypt([]byte("one"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if first.Body == second.Body {
		t.Fatal("identical ciphertext for two messages; the chain did not advance")
	}
}
