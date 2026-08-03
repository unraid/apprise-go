package matrixolm

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
)

func fixedRatchet() [4][]byte {
	var ratchet [4][]byte
	for i := range ratchet {
		part := make([]byte, 32)
		for j := range part {
			part[j] = byte(i*32 + j)
		}
		ratchet[i] = part
	}
	return ratchet
}

func fixedSigningSeed() []byte {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(255 - i)
	}
	return seed
}

func TestCanonicalJSONSortsAndLeavesHTMLLiteral(t *testing.T) {
	got, err := CanonicalJSON(map[string]any{
		"b": "second",
		"a": map[string]any{"z": 1, "y": []any{"<tag>", "&amp"}},
	})
	if err != nil {
		t.Fatalf("canonical json: %v", err)
	}

	want := `{"a":{"y":["<tag>","&amp"],"z":1},"b":"second"}`
	if string(got) != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestBase64RoundTripIsUnpadded(t *testing.T) {
	encoded := EncodeBase64([]byte{1, 2, 3, 4, 5})
	if strings.Contains(encoded, "=") {
		t.Fatalf("encoding is padded: %q", encoded)
	}

	decoded, err := DecodeBase64(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(decoded, []byte{1, 2, 3, 4, 5}) {
		t.Fatalf("round trip mismatch: %v", decoded)
	}

	// Other implementations emit padded and URL-safe forms.
	if _, err := DecodeBase64("AQIDBAU="); err != nil {
		t.Fatalf("padded decode: %v", err)
	}
}

// The session key is what recipients import, so its layout is not ours to
// choose: version, counter, the whole ratchet, the signing key, signature.
func TestMegolmSessionKeyLayout(t *testing.T) {
	session, err := NewMegolmSessionFromState(fixedRatchet(), 7, fixedSigningSeed())
	if err != nil {
		t.Fatalf("restore session: %v", err)
	}

	raw, err := DecodeBase64(session.SessionKey())
	if err != nil {
		t.Fatalf("decode session key: %v", err)
	}

	if len(raw) != 1+4+128+32+64 {
		t.Fatalf("session key is %d bytes, want 229", len(raw))
	}
	if raw[0] != 0x02 {
		t.Fatalf("version byte is %#x, want 0x02", raw[0])
	}
	if counter := binary.BigEndian.Uint32(raw[1:5]); counter != 7 {
		t.Fatalf("counter is %d, want 7", counter)
	}

	ratchet := fixedRatchet()
	var flat []byte
	for _, part := range ratchet {
		flat = append(flat, part...)
	}
	if !bytes.Equal(raw[5:133], flat) {
		t.Fatal("ratchet bytes do not match the session state")
	}

	pub := ed25519.NewKeyFromSeed(fixedSigningSeed()).Public().(ed25519.PublicKey)
	if !bytes.Equal(raw[133:165], pub) {
		t.Fatal("signing key does not match")
	}
	if !ed25519.Verify(pub, raw[:165], raw[165:]) {
		t.Fatal("session key signature does not verify")
	}
}

func TestMegolmEncryptWireFormat(t *testing.T) {
	session, err := NewMegolmSessionFromState(fixedRatchet(), 0, fixedSigningSeed())
	if err != nil {
		t.Fatalf("restore session: %v", err)
	}

	ciphertext, err := session.Encrypt([]byte(`{"body":"hello"}`))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	raw, err := DecodeBase64(ciphertext)
	if err != nil {
		t.Fatalf("decode ciphertext: %v", err)
	}

	if raw[0] != 0x03 {
		t.Fatalf("version byte is %#x, want 0x03", raw[0])
	}

	pub := ed25519.NewKeyFromSeed(fixedSigningSeed()).Public().(ed25519.PublicKey)
	signed := raw[:len(raw)-64]
	if !ed25519.Verify(pub, signed, raw[len(raw)-64:]) {
		t.Fatal("message signature does not verify")
	}
	if bytes.Contains(raw, []byte("hello")) {
		t.Fatal("plaintext survived into the ciphertext")
	}
}

// The ratchet must advance exactly once per message, or a recipient's
// message index no longer lines up with ours.
func TestMegolmRatchetAdvancesPerMessage(t *testing.T) {
	session, err := NewMegolmSessionFromState(fixedRatchet(), 0, fixedSigningSeed())
	if err != nil {
		t.Fatalf("restore session: %v", err)
	}

	first, err := session.Encrypt([]byte(`{"body":"one"}`))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	second, err := session.Encrypt([]byte(`{"body":"one"}`))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if first == second {
		t.Fatal("identical ciphertext for two messages; the ratchet did not advance")
	}

	_, counter, _ := session.State()
	if counter != 2 {
		t.Fatalf("counter is %d after two messages, want 2", counter)
	}
}

func TestDeviceKeysAreSignedOverCanonicalJSON(t *testing.T) {
	account, err := NewAccount()
	if err != nil {
		t.Fatalf("new account: %v", err)
	}

	keys, err := account.DeviceKeys("@user:example.com", "DEVICE")
	if err != nil {
		t.Fatalf("device keys: %v", err)
	}

	signatures, ok := keys["signatures"].(map[string]any)
	if !ok {
		t.Fatal("device keys carry no signatures")
	}
	user, ok := signatures["@user:example.com"].(map[string]any)
	if !ok {
		t.Fatal("signatures are not keyed by user")
	}
	signature, ok := user["ed25519:DEVICE"].(string)
	if !ok {
		t.Fatal("no signature for the device signing key")
	}

	// Verifiers strip signatures before checking, so we must too.
	unsigned := map[string]any{}
	for key, value := range keys {
		if key != "signatures" && key != "unsigned" {
			unsigned[key] = value
		}
	}
	canonical, err := CanonicalJSON(unsigned)
	if err != nil {
		t.Fatalf("canonical json: %v", err)
	}

	pub, err := DecodeBase64(account.SigningKey())
	if err != nil {
		t.Fatalf("decode signing key: %v", err)
	}
	sig, err := DecodeBase64(signature)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if !ed25519.Verify(pub, canonical, sig) {
		t.Fatal("device key signature does not verify")
	}
}

func TestOneTimeKeysAreSignedAndDistinct(t *testing.T) {
	account, err := NewAccount()
	if err != nil {
		t.Fatalf("new account: %v", err)
	}

	payload, err := account.GenerateOneTimeKeys("@user:example.com", "DEVICE", 3)
	if err != nil {
		t.Fatalf("one time keys: %v", err)
	}
	if len(payload) != 3 {
		t.Fatalf("got %d one time keys, want 3", len(payload))
	}

	seen := map[string]struct{}{}
	for id, object := range payload {
		if !strings.HasPrefix(id, "signed_curve25519:") {
			t.Fatalf("unexpected key id %q", id)
		}
		entry, ok := object.(map[string]any)
		if !ok {
			t.Fatalf("key %q is not an object", id)
		}
		key, _ := entry["key"].(string)
		if key == "" {
			t.Fatalf("key %q carries no public key", id)
		}
		if _, dup := seen[key]; dup {
			t.Fatal("two one time keys share a public key")
		}
		seen[key] = struct{}{}
		if _, ok := entry["signatures"]; !ok {
			t.Fatalf("key %q is unsigned", id)
		}
	}
}

func TestAccountKeysSurvivePersistence(t *testing.T) {
	account, err := NewAccount()
	if err != nil {
		t.Fatalf("new account: %v", err)
	}

	identity, signing := account.PrivateKeys()
	restored, err := NewAccountFromKeys(identity, signing)
	if err != nil {
		t.Fatalf("restore account: %v", err)
	}

	if restored.IdentityKey() != account.IdentityKey() {
		t.Fatal("identity key changed across persistence")
	}
	if restored.SigningKey() != account.SigningKey() {
		t.Fatal("signing key changed across persistence")
	}
}

func TestDeviceKeysAdvertiseBothAlgorithms(t *testing.T) {
	account, err := NewAccount()
	if err != nil {
		t.Fatalf("new account: %v", err)
	}

	keys, err := account.DeviceKeys("@user:example.com", "DEVICE")
	if err != nil {
		t.Fatalf("device keys: %v", err)
	}

	encoded, err := json.Marshal(keys["algorithms"])
	if err != nil {
		t.Fatalf("marshal algorithms: %v", err)
	}
	for _, algorithm := range []string{"m.olm.v1.curve25519-aes-sha2", "m.megolm.v1.aes-sha2"} {
		if !strings.Contains(string(encoded), algorithm) {
			t.Fatalf("device keys do not advertise %s", algorithm)
		}
	}
}

// Upstream serialises the event with Python's json.dumps defaults before
// encrypting, so our serialisation has to produce the same bytes or the
// ciphertext diverges even when the crypto is correct.
func TestMarshalEventMatchesPythonSpacing(t *testing.T) {
	payload := struct {
		Body    string `json:"body"`
		MsgType string `json:"msgtype"`
	}{Body: "cross check", MsgType: "m.text"}

	got, err := MarshalEvent(payload)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	want := `{"body": "cross check", "msgtype": "m.text"}`
	if string(got) != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestMarshalEventHandlesNestingAndArrays(t *testing.T) {
	payload := struct {
		Algorithm string   `json:"algorithm"`
		Targets   []string `json:"targets"`
		Nested    struct {
			Key string `json:"key"`
		} `json:"nested"`
	}{Algorithm: "m.megolm.v1.aes-sha2", Targets: []string{"a", "b"}}
	payload.Nested.Key = "value"

	got, err := MarshalEvent(payload)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	want := `{"algorithm": "m.megolm.v1.aes-sha2", "targets": ["a", "b"], "nested": {"key": "value"}}`
	if string(got) != want {
		t.Fatalf("got %s want %s", got, want)
	}
}
