package notify

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"testing"

	"golang.org/x/crypto/hkdf"
)

// Proves encryptWebPush produces an RFC 8291 (aes128gcm) payload that decrypts
// back to the original plaintext using the recipient's private key.
func TestEncryptWebPushRoundTrip(t *testing.T) {
	recipient, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	authSecret := make([]byte, 16)
	if _, err := rand.Read(authSecret); err != nil {
		t.Fatal(err)
	}

	msg := []byte("hello web push migration")
	payload, err := encryptWebPush(msg, recipient.PublicKey(), authSecret)
	if err != nil {
		t.Fatalf("encryptWebPush: %v", err)
	}

	// Parse the aes128gcm header: salt(16) | rs(4) | idlen(1) | keyid(idlen) | ciphertext
	salt := payload[0:16]
	idlen := int(payload[20])
	ephemeralPub := payload[21 : 21+idlen]
	ciphertext := payload[21+idlen:]

	ephPubKey, err := ecdh.P256().NewPublicKey(ephemeralPub)
	if err != nil {
		t.Fatalf("parse ephemeral pub: %v", err)
	}

	// Recipient derives the same shared secret from its private key.
	sharedSecret, err := recipient.ECDH(ephPubKey)
	if err != nil {
		t.Fatal(err)
	}

	recipientPub := recipient.PublicKey().Bytes()
	info := append([]byte("WebPush: info\x00"), recipientPub...)
	info = append(info, ephemeralPub...)

	prk := hkdf.New(sha256.New, sharedSecret, authSecret, info)
	ikm := make([]byte, 32)
	if _, err := prk.Read(ikm); err != nil {
		t.Fatal(err)
	}

	cekReader := hkdf.New(sha256.New, ikm, salt, []byte("Content-Encoding: aes128gcm\x00"))
	cek := make([]byte, 16)
	if _, err := cekReader.Read(cek); err != nil {
		t.Fatal(err)
	}
	nonceReader := hkdf.New(sha256.New, ikm, salt, []byte("Content-Encoding: nonce\x00"))
	nonce := make([]byte, 12)
	if _, err := nonceReader.Read(nonce); err != nil {
		t.Fatal(err)
	}

	block, err := aes.NewCipher(cek)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.Fatalf("decrypt failed (migration broke crypto): %v", err)
	}

	// encryptWebPush appends a 0x02 padding-delimiter byte per RFC 8188.
	plaintext = bytes.TrimRight(plaintext, "\x02")
	if !bytes.Equal(plaintext, msg) {
		t.Fatalf("round-trip mismatch: got %q want %q", plaintext, msg)
	}
}

// Confirms loadVapidKey produces a 65-byte uncompressed P-256 public point
// (0x04 prefix), as required by the VAPID "k=" header parameter.
func TestLoadVapidKeyPublicShape(t *testing.T) {
	_, pub, err := loadVapidKey("../testutil/fixtures/vapid_test_key.pem")
	if err != nil {
		t.Fatalf("loadVapidKey: %v", err)
	}
	raw, err := decodeBase64URL(pub)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 65 || raw[0] != 0x04 {
		t.Fatalf("unexpected public key encoding: len=%d prefix=0x%02x", len(raw), raw[0])
	}
}
