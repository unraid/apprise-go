package matrixolm

import (
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

// OlmSession is a single-use outbound Olm session. It exists to hand one
// recipient device the Megolm room key, which is all a notifier ever needs to
// send, so it only produces pre-key (type 0) messages.
type OlmSession struct {
	ourIdentity   []byte
	ephemeral     []byte
	theirOneTime  []byte
	theirIdentity []byte
	chainKey      []byte
	counter       uint32
}

// OlmMessage is the ciphertext entry of an m.olm.v1.curve25519-aes-sha2 event.
type OlmMessage struct {
	Type int    `json:"type"`
	Body string `json:"body"`
}

// NewOutboundSession performs the triple Diffie-Hellman handshake against a
// recipient's published identity key and a claimed one-time key.
func (a *Account) NewOutboundSession(theirIdentityKey, theirOneTimeKey string) (*OlmSession, error) {
	identityBytes, err := DecodeBase64(theirIdentityKey)
	if err != nil {
		return nil, fmt.Errorf("decode identity key: %w", err)
	}
	oneTimeBytes, err := DecodeBase64(theirOneTimeKey)
	if err != nil {
		return nil, fmt.Errorf("decode one time key: %w", err)
	}

	theirIdentity, err := ecdh.X25519().NewPublicKey(identityBytes)
	if err != nil {
		return nil, fmt.Errorf("parse identity key: %w", err)
	}
	theirOneTime, err := ecdh.X25519().NewPublicKey(oneTimeBytes)
	if err != nil {
		return nil, fmt.Errorf("parse one time key: %w", err)
	}

	// The ephemeral key is both the X3DH base key and the first ratchet key.
	// Using two different keys here leaves the recipient unable to decrypt.
	ephemeral, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ephemeral key: %w", err)
	}

	return a.outboundSessionWithEphemeral(ephemeral, theirIdentity, theirOneTime)
}

// outboundSessionWithEphemeral is the deterministic core of the handshake,
// separated so tests can pin the ephemeral key and compare bytes with upstream.
func (a *Account) outboundSessionWithEphemeral(
	ephemeral *ecdh.PrivateKey,
	theirIdentity, theirOneTime *ecdh.PublicKey,
) (*OlmSession, error) {
	// DH1 = X25519(IK_A, OTK_B), DH2 = X25519(E_A, IK_B), DH3 = X25519(E_A, OTK_B)
	dh1, err := a.identity.ECDH(theirOneTime)
	if err != nil {
		return nil, fmt.Errorf("triple diffie hellman step 1: %w", err)
	}
	dh2, err := ephemeral.ECDH(theirIdentity)
	if err != nil {
		return nil, fmt.Errorf("triple diffie hellman step 2: %w", err)
	}
	dh3, err := ephemeral.ECDH(theirOneTime)
	if err != nil {
		return nil, fmt.Errorf("triple diffie hellman step 3: %w", err)
	}

	// The 96 byte secret is passed straight to HKDF with no prefix; anything
	// else derives a different root and chain key.
	ikm := append(append(append([]byte{}, dh1...), dh2...), dh3...)
	keys, err := hkdf.Key(sha256.New, ikm, nil, "OLM_ROOT", 64)
	if err != nil {
		return nil, fmt.Errorf("derive olm root key: %w", err)
	}

	return &OlmSession{
		ourIdentity:   a.identity.PublicKey().Bytes(),
		ephemeral:     ephemeral.PublicKey().Bytes(),
		theirOneTime:  theirOneTime.Bytes(),
		theirIdentity: theirIdentity.Bytes(),
		chainKey:      keys[32:],
	}, nil
}

// TheirIdentityKey is the recipient device's Curve25519 key, which the event
// is addressed to.
func (s *OlmSession) TheirIdentityKey() string {
	return EncodeBase64(s.theirIdentity)
}

// Encrypt produces a pre-key message carrying plaintext, advancing the chain
// ratchet once.
func (s *OlmSession) Encrypt(plaintext []byte) (OlmMessage, error) {
	// Chain ratchet: the message key and the next chain key are separate
	// HMACs of the current chain key.
	messageKey := hmacSHA256Sum(s.chainKey, []byte{0x01})
	s.chainKey = hmacSHA256Sum(s.chainKey, []byte{0x02})

	keys, err := hkdf.Key(sha256.New, messageKey, make([]byte, 32), "OLM_KEYS", 80)
	if err != nil {
		return OlmMessage{}, fmt.Errorf("derive olm message keys: %w", err)
	}
	aesKey, macKey, iv := keys[:32], keys[32:64], keys[64:80]

	ciphertext, err := aesCBCEncrypt(aesKey, iv, plaintext)
	if err != nil {
		return OlmMessage{}, fmt.Errorf("olm encrypt: %w", err)
	}

	// Inner message: ratchet key, chain index, ciphertext. The ciphertext is
	// field 4; there is no field 3 in this format.
	inner := []byte{0x03}
	inner = append(inner, pbBytes(1, s.ephemeral)...)
	inner = append(inner, pbVarintField(2, uint64(s.counter))...)
	inner = append(inner, pbBytes(4, ciphertext)...)
	inner = append(inner, hmacSHA256Sum(macKey, inner)[:8]...)

	// Outer pre-key message: the one-time key being consumed, the base key,
	// our identity key, then the inner message. It carries no MAC of its own,
	// and trailing bytes make strict protobuf decoders reject the session.
	outer := []byte{0x03}
	outer = append(outer, pbBytes(1, s.theirOneTime)...)
	outer = append(outer, pbBytes(2, s.ephemeral)...)
	outer = append(outer, pbBytes(3, s.ourIdentity)...)
	outer = append(outer, pbBytes(4, inner)...)

	s.counter++

	return OlmMessage{Type: 0, Body: EncodeBase64(outer)}, nil
}
