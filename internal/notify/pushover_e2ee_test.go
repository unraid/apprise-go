package notify

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/url"
	"strings"
	"testing"
)

const pushoverTestKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// decryptPushoverField reverses pushoverEncryptField so the wire format can be
// checked against the scheme documented at https://pushover.net/api#e2ee.
func decryptPushoverField(t *testing.T, encoded string, key []byte) string {
	t.Helper()

	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	if len(payload) < aes.BlockSize+sha256.Size {
		t.Fatalf("payload too short: %d bytes", len(payload))
	}

	iv := payload[:aes.BlockSize]
	ciphertext := payload[aes.BlockSize : len(payload)-sha256.Size]
	mac := payload[len(payload)-sha256.Size:]

	expected := hmac.New(sha256.New, key)
	expected.Write(iv)
	expected.Write(ciphertext)
	if !hmac.Equal(mac, expected.Sum(nil)) {
		t.Fatal("hmac mismatch")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		t.Fatalf("ciphertext is not block aligned: %d bytes", len(ciphertext))
	}

	padded := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(padded, ciphertext)

	padding := int(padded[len(padded)-1])
	if padding == 0 || padding > aes.BlockSize || padding > len(padded) {
		t.Fatalf("invalid pkcs7 padding: %d", padding)
	}
	for _, b := range padded[len(padded)-padding:] {
		if int(b) != padding {
			t.Fatal("inconsistent pkcs7 padding")
		}
	}

	reader, err := gzip.NewReader(bytes.NewReader(padded[:len(padded)-padding]))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	plaintext, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("gzip read: %v", err)
	}

	return string(plaintext)
}

func TestPushoverEncryptFieldRoundTrip(t *testing.T) {
	key, err := hex.DecodeString(pushoverTestKey)
	if err != nil {
		t.Fatalf("decode key: %v", err)
	}

	for _, plaintext := range []string{"hello", "", strings.Repeat("a", 4096), "unicode: ✓ ☃"} {
		encoded, err := pushoverEncryptField(plaintext, key)
		if err != nil {
			t.Fatalf("encrypt %q: %v", plaintext, err)
		}
		if got := decryptPushoverField(t, encoded, key); got != plaintext {
			t.Fatalf("round trip mismatch: got %q want %q", got, plaintext)
		}
	}
}

func TestPushoverEncryptFieldUsesFreshIV(t *testing.T) {
	key, err := hex.DecodeString(pushoverTestKey)
	if err != nil {
		t.Fatalf("decode key: %v", err)
	}

	first, err := pushoverEncryptField("hello", key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	second, err := pushoverEncryptField("hello", key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if first == second {
		t.Fatal("identical ciphertexts for repeated encryption; IV is not random")
	}
}

func TestPushoverSendEncryptsFields(t *testing.T) {
	parsed, err := ParseURL("pover://userkey123@token123?key=" + pushoverTestKey + "&url=https://example.com&url_title=Example")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	target, err := NewPushoverTarget(parsed)
	if err != nil {
		t.Fatalf("new target: %v", err)
	}

	spec, err := target.BuildRequest("secret body", "secret title", NotifyInfo)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	values, err := url.ParseQuery(spec.Body)
	if err != nil {
		t.Fatalf("parse body: %v", err)
	}

	if values.Get("encrypted") != "1" {
		t.Fatalf("expected encrypted=1, body was %s", spec.Body)
	}

	key, err := hex.DecodeString(pushoverTestKey)
	if err != nil {
		t.Fatalf("decode key: %v", err)
	}

	for field, want := range map[string]string{
		"message":   "secret body",
		"title":     "secret title",
		"url":       "https://example.com",
		"url_title": "Example",
	} {
		encoded := values.Get(field)
		if encoded == want {
			t.Fatalf("%s was sent as plaintext", field)
		}
		if got := decryptPushoverField(t, encoded, key); got != want {
			t.Fatalf("%s decrypted to %q, want %q", field, got, want)
		}
	}
}

func TestPushoverRejectsInvalidEncryptionKey(t *testing.T) {
	parsed, err := ParseURL("pover://userkey123@token123?key=tooshort")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	if _, err := NewPushoverTarget(parsed); err == nil {
		t.Fatal("expected an error for a malformed encryption key")
	}
}

func TestPushoverEncryptionDisabledWithoutKey(t *testing.T) {
	parsed, err := ParseURL("pover://userkey123@token123?e2ee=yes")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	target, err := NewPushoverTarget(parsed)
	if err != nil {
		t.Fatalf("new target: %v", err)
	}

	if target.e2ee {
		t.Fatal("e2ee should stay off when no encryption key is supplied")
	}
}
