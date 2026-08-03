package notify

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

// The constants below were produced by upstream Apprise 1.12.0's own sogs
// plugin with the nonce and timestamp pinned. Do not "fix" a failure here by
// regenerating them. A mismatch means our signing construction drifted, and
// the symptom in production is a 401 from the server rather than anything a
// request diff would show.
//
// To re-derive, run upstream's _build_session_message and repeat its
// _sogs_auth_headers concatenation with the same pinned nonce and timestamp.
const (
	sogsVectorSeedHex      = "0101010101010101010101010101010101010101010101010101010101010101"
	sogsVectorPublicKeyHex = "abababababababababababababababababababababababababababababababab"
	sogsVectorTimestamp    = 1700000000
	sogsVectorText         = "apprise parity\r\nhello from python"
	sogsVectorPath         = "/room/general/message"

	sogsVectorMessageHex     = "0a230a2161707072697365207061726974790d0a68656c6c6f2066726f6d20707974686f6e80"
	sogsVectorMessageSigB64  = "8SOaRmy0etoWavp5tAgNSw0kSHeGmL7tKZRiVzhnE7YG8ONZ1ZSh50Al8milWnYT3YOe4+OYILJnufgNYlZtBQ=="
	sogsVectorBody           = `{"data": "CiMKIWFwcHJpc2UgcGFyaXR5DQpoZWxsbyBmcm9tIHB5dGhvboA=", "signature": "8SOaRmy0etoWavp5tAgNSw0kSHeGmL7tKZRiVzhnE7YG8ONZ1ZSh50Al8milWnYT3YOe4+OYILJnufgNYlZtBQ=="}`
	sogsVectorBotPubkeyHex   = "8a88e3dd7409f195fd52db2d3cba5d72ca6709bf1d94121bf3748801b40f6f5c"
	sogsVectorAuthSignBase64 = "EwrOaAufFDdgr470EsJlRcRkrVAcYVojTyYbcEDtd88WBPx+Z+NIBOftvRuFaTjPciINx5xv8cleGvKke32lAg=="
)

// sogsVectorNonce is bytes 0x00 through 0x0f, the pinned counterpart to the
// random 16 bytes a real request uses.
func sogsVectorNonce() []byte {
	nonce := make([]byte, 16)
	for i := range nonce {
		nonce[i] = byte(i)
	}

	return nonce
}

func TestSOGSSessionMessageMatchesUpstream(t *testing.T) {
	got := hex.EncodeToString(buildSessionMessage(sogsVectorText))
	if got != sogsVectorMessageHex {
		t.Fatalf("session message mismatch:\n want %s\n got  %s", sogsVectorMessageHex, got)
	}
}

func TestSOGSMessageSignatureMatchesUpstream(t *testing.T) {
	seed, err := hex.DecodeString(sogsVectorSeedHex)
	if err != nil {
		t.Fatalf("decode seed: %v", err)
	}

	key := ed25519.NewKeyFromSeed(seed)
	signature := ed25519.Sign(key, buildSessionMessage(sogsVectorText))

	if got := base64.StdEncoding.EncodeToString(signature); got != sogsVectorMessageSigB64 {
		t.Fatalf("message signature mismatch:\n want %s\n got  %s", sogsVectorMessageSigB64, got)
	}
}

// TestSOGSRequestBodyMatchesUpstream pins the exact bytes of the request body.
// It is not cosmetic: the auth signature covers these bytes, so compact JSON
// would produce a signature upstream never would.
func TestSOGSRequestBodyMatchesUpstream(t *testing.T) {
	target := sogsVectorTarget(t)

	specs, err := target.buildRequests("hello from python", "apprise parity", NotifyInfo)
	if err != nil {
		t.Fatalf("build requests: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("want 1 request, got %d", len(specs))
	}

	if specs[0].Body != sogsVectorBody {
		t.Fatalf("request body mismatch:\n want %s\n got  %s", sogsVectorBody, specs[0].Body)
	}
}

func TestSOGSAuthHeadersMatchUpstream(t *testing.T) {
	target := sogsVectorTarget(t)

	headers, err := target.authHeaders("POST", sogsVectorPath, []byte(sogsVectorBody))
	if err != nil {
		t.Fatalf("auth headers: %v", err)
	}

	want := map[string]string{
		"X-SOGS-Pubkey":    "00" + sogsVectorBotPubkeyHex,
		"X-SOGS-Nonce":     base64.StdEncoding.EncodeToString(sogsVectorNonce()),
		"X-SOGS-Timestamp": "1700000000",
		"X-SOGS-Signature": sogsVectorAuthSignBase64,
	}

	for key, expected := range want {
		if headers[key] != expected {
			t.Fatalf("%s mismatch:\n want %s\n got  %s", key, expected, headers[key])
		}
	}
}

// sogsVectorTarget builds a target with the nonce and timestamp pinned to the
// vector's, restoring the real sources when the test finishes.
func sogsVectorTarget(t *testing.T) *SOGSTarget {
	t.Helper()

	originalNonce, originalNow := sogsNonce, sogsNow
	t.Cleanup(func() { sogsNonce, sogsNow = originalNonce, originalNow })
	sogsNonce = func() ([]byte, error) { return sogsVectorNonce(), nil }
	sogsNow = func() int64 { return sogsVectorTimestamp }

	parsed, err := ParseURL(
		"sogs://" + sogsVectorPublicKeyHex + ":" + sogsVectorSeedHex + "@sogs.example.com/general",
	)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	target, err := NewSOGSTarget(parsed)
	if err != nil {
		t.Fatalf("build target: %v", err)
	}

	return target
}
