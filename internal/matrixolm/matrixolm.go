// Package matrixolm implements the Olm and Megolm primitives Matrix
// end-to-end encryption needs, built on the standard library.
//
// It follows the same construction upstream Apprise uses, which in turn
// follows the Megolm specification and libolm. Matching the construction is
// what makes the bytes on the wire match: real Matrix clients have to decrypt
// what we send, so the format is not ours to choose.
package matrixolm

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
)

// EncodeBase64 returns the unpadded base64 Matrix uses on the wire.
func EncodeBase64(data []byte) string {
	return base64.RawStdEncoding.EncodeToString(data)
}

// DecodeBase64 accepts padded, unpadded and URL-safe base64, matching the
// leniency of other Matrix implementations.
func DecodeBase64(value string) ([]byte, error) {
	normalized := make([]byte, 0, len(value))
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '-':
			normalized = append(normalized, '+')
		case '_':
			normalized = append(normalized, '/')
		case '=':
		default:
			normalized = append(normalized, value[i])
		}
	}

	return base64.RawStdEncoding.DecodeString(string(normalized))
}

func hmacSHA256Sum(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// aesCBCEncrypt applies PKCS7 padding and encrypts under AES-CBC.
func aesCBCEncrypt(key, iv, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	padded := make([]byte, 0, len(plaintext)+padding)
	padded = append(padded, plaintext...)
	for range padding {
		padded = append(padded, byte(padding))
	}

	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)
	return ciphertext, nil
}

// pbVarint encodes a protobuf base 128 varint.
func pbVarint(value uint64) []byte {
	buf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(buf, value)
	return buf[:n]
}

// pbVarintField encodes a varint field with its tag.
func pbVarintField(field int, value uint64) []byte {
	return append(pbVarint(uint64(field)<<3|0), pbVarint(value)...)
}

// pbBytes encodes a length delimited field with its tag.
func pbBytes(field int, value []byte) []byte {
	out := append(pbVarint(uint64(field)<<3|2), pbVarint(uint64(len(value)))...)
	return append(out, value...)
}

// CanonicalJSON returns the sorted-key, whitespace-free encoding Matrix signs.
func CanonicalJSON(value any) ([]byte, error) {
	// encoding/json already sorts map keys and omits insignificant space, but
	// it escapes HTML characters, which changes the bytes being signed.
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	var normalized any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}

	var buf []byte
	buf, err = appendCanonical(buf, normalized)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

func appendCanonical(dst []byte, value any) ([]byte, error) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		dst = append(dst, '{')
		for i, key := range keys {
			if i > 0 {
				dst = append(dst, ',')
			}
			encoded, err := encodeJSONString(key)
			if err != nil {
				return nil, err
			}
			dst = append(dst, encoded...)
			dst = append(dst, ':')
			dst, err = appendCanonical(dst, typed[key])
			if err != nil {
				return nil, err
			}
		}
		return append(dst, '}'), nil

	case []any:
		dst = append(dst, '[')
		for i, entry := range typed {
			if i > 0 {
				dst = append(dst, ',')
			}
			var err error
			dst, err = appendCanonical(dst, entry)
			if err != nil {
				return nil, err
			}
		}
		return append(dst, ']'), nil

	case string:
		encoded, err := encodeJSONString(typed)
		if err != nil {
			return nil, err
		}
		return append(dst, encoded...), nil

	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return nil, err
		}
		return append(dst, encoded...), nil
	}
}

// Account is the device identity: a Curve25519 key that other devices
// encrypt to, and an Ed25519 key that signs everything this device publishes.
type Account struct {
	identity *ecdh.PrivateKey
	signing  ed25519.PrivateKey
	oneTime  map[string]*ecdh.PrivateKey
}

// NewAccount generates a fresh device identity.
func NewAccount() (*Account, error) {
	identity, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate identity key: %w", err)
	}

	_, signing, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate signing key: %w", err)
	}

	return &Account{
		identity: identity,
		signing:  signing,
		oneTime:  map[string]*ecdh.PrivateKey{},
	}, nil
}

// NewAccountFromKeys restores an account from stored raw private keys, so a
// notifier does not register a new device on every send.
func NewAccountFromKeys(identityPriv, signingPriv []byte) (*Account, error) {
	identity, err := ecdh.X25519().NewPrivateKey(identityPriv)
	if err != nil {
		return nil, fmt.Errorf("restore identity key: %w", err)
	}
	if len(signingPriv) != ed25519.SeedSize {
		return nil, fmt.Errorf("signing key must be %d bytes", ed25519.SeedSize)
	}

	return &Account{
		identity: identity,
		signing:  ed25519.NewKeyFromSeed(signingPriv),
		oneTime:  map[string]*ecdh.PrivateKey{},
	}, nil
}

// IdentityKey is the Curve25519 public key other devices encrypt to.
func (a *Account) IdentityKey() string {
	return EncodeBase64(a.identity.PublicKey().Bytes())
}

// SigningKey is the Ed25519 public key that verifies this device's signatures.
func (a *Account) SigningKey() string {
	return EncodeBase64(a.signing.Public().(ed25519.PublicKey))
}

// PrivateKeys returns the raw private key material for persistence.
func (a *Account) PrivateKeys() (identity, signing []byte) {
	return a.identity.Bytes(), a.signing.Seed()
}

// Sign returns the unpadded base64 Ed25519 signature of data.
func (a *Account) Sign(data []byte) string {
	return EncodeBase64(ed25519.Sign(a.signing, data))
}

// signObject signs a JSON object in place under the Matrix scheme: the
// signature covers the canonical JSON of the object with any signatures and
// unsigned members removed.
func (a *Account) signObject(object map[string]any, userID, deviceID string) error {
	unsigned, hadUnsigned := object["unsigned"]
	delete(object, "unsigned")
	delete(object, "signatures")

	canonical, err := CanonicalJSON(object)
	if err != nil {
		return err
	}

	object["signatures"] = map[string]any{
		userID: map[string]any{
			"ed25519:" + deviceID: a.Sign(canonical),
		},
	}
	if hadUnsigned {
		object["unsigned"] = unsigned
	}

	return nil
}

// DeviceKeys builds the signed device key object for POST /keys/upload.
func (a *Account) DeviceKeys(userID, deviceID string) (map[string]any, error) {
	keys := map[string]any{
		"algorithms": []any{"m.olm.v1.curve25519-aes-sha2", "m.megolm.v1.aes-sha2"},
		"device_id":  deviceID,
		"keys": map[string]any{
			"curve25519:" + deviceID: a.IdentityKey(),
			"ed25519:" + deviceID:    a.SigningKey(),
		},
		"user_id": userID,
	}

	if err := a.signObject(keys, userID, deviceID); err != nil {
		return nil, fmt.Errorf("sign device keys: %w", err)
	}

	return keys, nil
}

// GenerateOneTimeKeys tops the pool up to count keys and returns the signed
// one_time_keys object for POST /keys/upload.
func (a *Account) GenerateOneTimeKeys(userID, deviceID string, count int) (map[string]any, error) {
	for len(a.oneTime) < count {
		key, err := ecdh.X25519().GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate one time key: %w", err)
		}

		id := make([]byte, 5)
		if _, err := rand.Read(id); err != nil {
			return nil, fmt.Errorf("generate one time key id: %w", err)
		}
		a.oneTime[fmt.Sprintf("%x", id)] = key
	}

	payload := map[string]any{}
	for id, key := range a.oneTime {
		object := map[string]any{"key": EncodeBase64(key.PublicKey().Bytes())}
		if err := a.signObject(object, userID, deviceID); err != nil {
			return nil, fmt.Errorf("sign one time key: %w", err)
		}
		payload["signed_curve25519:"+id] = object
	}

	return payload, nil
}

// MegolmSession encrypts room events. The ratchet advances after every
// message, and recipients cannot decrypt anything sent before the session key
// they hold, so SessionKey must be captured before the first Encrypt.
type MegolmSession struct {
	ratchet [4][]byte
	counter uint32
	signing ed25519.PrivateKey
}

// NewMegolmSession starts a session with a random ratchet.
func NewMegolmSession() (*MegolmSession, error) {
	session := &MegolmSession{}
	for i := range session.ratchet {
		part := make([]byte, 32)
		if _, err := rand.Read(part); err != nil {
			return nil, fmt.Errorf("seed megolm ratchet: %w", err)
		}
		session.ratchet[i] = part
	}

	_, signing, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate megolm signing key: %w", err)
	}
	session.signing = signing

	return session, nil
}

// NewMegolmSessionFromState restores a session, which is what allows a sender
// to keep using one session across sends rather than re-sharing keys.
func NewMegolmSessionFromState(ratchet [4][]byte, counter uint32, signingPriv []byte) (*MegolmSession, error) {
	if len(signingPriv) != ed25519.SeedSize {
		return nil, fmt.Errorf("signing key must be %d bytes", ed25519.SeedSize)
	}

	session := &MegolmSession{counter: counter, signing: ed25519.NewKeyFromSeed(signingPriv)}
	for i, part := range ratchet {
		if len(part) != 32 {
			return nil, fmt.Errorf("ratchet part %d must be 32 bytes", i)
		}
		session.ratchet[i] = append([]byte(nil), part...)
	}

	return session, nil
}

// SessionID is the base64 Ed25519 public key identifying this session.
func (s *MegolmSession) SessionID() string {
	return EncodeBase64(s.signing.Public().(ed25519.PublicKey))
}

// advance moves the ratchet on by one step. The part that stays constant is
// the highest index whose period does not divide the next counter value, and
// every derived part is computed from the original value of that part before
// it is itself overwritten.
func (s *MegolmSession) advance() {
	next := s.counter + 1

	switch {
	case next%(1<<24) == 0:
		original := s.ratchet[0]
		s.ratchet[3] = hmacSHA256Sum(original, []byte{0x03})
		s.ratchet[2] = hmacSHA256Sum(original, []byte{0x02})
		s.ratchet[1] = hmacSHA256Sum(original, []byte{0x01})
		s.ratchet[0] = hmacSHA256Sum(original, []byte{0x00})

	case next%(1<<16) == 0:
		original := s.ratchet[1]
		s.ratchet[3] = hmacSHA256Sum(original, []byte{0x03})
		s.ratchet[2] = hmacSHA256Sum(original, []byte{0x02})
		s.ratchet[1] = hmacSHA256Sum(original, []byte{0x01})

	case next%(1<<8) == 0:
		original := s.ratchet[2]
		s.ratchet[3] = hmacSHA256Sum(original, []byte{0x03})
		s.ratchet[2] = hmacSHA256Sum(original, []byte{0x02})

	default:
		s.ratchet[3] = hmacSHA256Sum(s.ratchet[3], []byte{0x03})
	}

	s.counter = next
}

// messageKeys derives the AES key, MAC key and IV from the whole 128 byte
// ratchet. Deriving from R[3] alone would produce keys no standard client
// agrees with.
func (s *MegolmSession) messageKeys() (aesKey, macKey, iv []byte, err error) {
	ikm := make([]byte, 0, 128)
	for _, part := range s.ratchet {
		ikm = append(ikm, part...)
	}

	keys, err := hkdf.Key(sha256.New, ikm, nil, "MEGOLM_KEYS", 80)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("derive megolm keys: %w", err)
	}

	return keys[:32], keys[32:64], keys[64:80], nil
}

// Encrypt returns the base64 Megolm ciphertext for a room event payload:
// version byte, protobuf body, truncated HMAC, then an Ed25519 signature.
//
// The plaintext is encrypted verbatim, so callers must serialise the event
// exactly as upstream does or the ciphertext will not match it byte for byte.
// Upstream uses Python's json.dumps defaults, which put a space after every
// colon and comma and preserve the order the payload was built in. See
// MarshalEvent.
func (s *MegolmSession) Encrypt(plaintext []byte) (string, error) {
	aesKey, macKey, iv, err := s.messageKeys()
	if err != nil {
		return "", err
	}

	ciphertext, err := aesCBCEncrypt(aesKey, iv, plaintext)
	if err != nil {
		return "", fmt.Errorf("megolm encrypt: %w", err)
	}

	body := append([]byte{0x03}, pbVarintField(1, uint64(s.counter))...)
	body = append(body, pbBytes(2, ciphertext)...)

	mac := hmacSHA256Sum(macKey, body)[:8]
	signed := append(append([]byte(nil), body...), mac...)
	signature := ed25519.Sign(s.signing, signed)

	s.advance()

	return EncodeBase64(append(signed, signature...)), nil
}

// SessionKey is what recipients need to decrypt, shared over Olm in an
// m.room_key event. The trailing signature lets them confirm it came from the
// device that published the matching Ed25519 key.
func (s *MegolmSession) SessionKey() string {
	payload := make([]byte, 0, 165)
	payload = append(payload, 0x02)
	payload = binary.BigEndian.AppendUint32(payload, s.counter)
	for _, part := range s.ratchet {
		payload = append(payload, part...)
	}
	payload = append(payload, s.signing.Public().(ed25519.PublicKey)...)

	return EncodeBase64(append(payload, ed25519.Sign(s.signing, payload)...))
}

// State exports the session for persistence.
func (s *MegolmSession) State() (ratchet [4][]byte, counter uint32, signingPriv []byte) {
	for i, part := range s.ratchet {
		ratchet[i] = append([]byte(nil), part...)
	}
	return ratchet, s.counter, s.signing.Seed()
}

// encodeJSONString escapes a string the way canonical JSON requires, which
// differs from encoding/json's default in that HTML characters stay literal.
func encodeJSONString(value string) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}

	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// MarshalEvent serialises an event payload the way upstream does before
// encrypting it: a space after every colon and comma, and member order taken
// from the value rather than sorted. Pass a struct, whose field order
// encoding/json preserves; a map would be re-ordered and produce a ciphertext
// upstream never would.
func MarshalEvent(payload any) ([]byte, error) {
	compact, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(bytes.NewReader(compact))
	decoder.UseNumber()

	type frame struct {
		object    bool
		expectKey bool
		empty     bool
	}

	var out bytes.Buffer
	var stack []frame

	// separator writes the comma between members, and reports whether the
	// token being written is an object key.
	separator := func() bool {
		if len(stack) == 0 {
			return false
		}

		top := &stack[len(stack)-1]
		key := top.object && top.expectKey
		if !top.empty && (!top.object || key) {
			out.WriteString(", ")
		}
		top.empty = false
		return key
	}

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch typed := token.(type) {
		case json.Delim:
			switch typed {
			case '{', '[':
				separator()
				out.WriteRune(rune(typed))
				stack = append(stack, frame{object: typed == '{', expectKey: typed == '{', empty: true})
			default:
				out.WriteRune(rune(typed))
				stack = stack[:len(stack)-1]
				if len(stack) > 0 && stack[len(stack)-1].object {
					stack[len(stack)-1].expectKey = true
				}
			}

		default:
			key := separator()

			encoded, err := encodeJSONValue(typed)
			if err != nil {
				return nil, err
			}
			out.Write(encoded)

			if key {
				out.WriteString(": ")
				stack[len(stack)-1].expectKey = false
			} else if len(stack) > 0 && stack[len(stack)-1].object {
				stack[len(stack)-1].expectKey = true
			}
		}
	}

	return out.Bytes(), nil
}

func encodeJSONValue(value any) ([]byte, error) {
	if text, ok := value.(string); ok {
		return encodeJSONString(text)
	}
	return json.Marshal(value)
}
