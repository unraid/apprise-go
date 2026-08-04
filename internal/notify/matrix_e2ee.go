package notify

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/unraid/apprise-go/internal/matrixolm"
)

// Matrix end-to-end encryption. The cryptography lives in internal/matrixolm
// and is pinned against upstream by vectors_test.go; what follows is the
// protocol flow around it.
//
// All of this leans on persistent storage. The Olm account and the device id
// the homeserver assigned have to outlive the process: register a new device
// on every notification and recipients see an endless parade of unverified
// devices, while anything sent under a previous identity stays unreadable.

// Keys live for a day short of upstream's expiry so a refresh happens before
// a server would consider them stale.
const matrixE2EECacheTTL = 7 * 24 * time.Hour

// matrixOneTimeKeyCount is how many one-time keys are published per upload;
// each is consumed by one recipient device starting a session with us.
const matrixOneTimeKeyCount = 50

type matrixDeviceKeys struct {
	Curve25519 string
	Ed25519    string
}

// e2eeAccount loads the stored Olm account or creates one, uploading its keys
// when the server has not seen them for this device identity.
func (m *MatrixTarget) e2eeAccount() (*matrixolm.Account, error) {
	if m.olmAccount != nil {
		return m.olmAccount, nil
	}

	var stored struct {
		Identity []byte `json:"identity"`
		Signing  []byte `json:"signing"`
	}
	if storeGetJSON(m.store, "e2ee_account", &stored) && len(stored.Identity) > 0 {
		if account, err := matrixolm.NewAccountFromKeys(stored.Identity, stored.Signing); err == nil {
			m.olmAccount = account
		}
	}

	if m.olmAccount == nil {
		account, err := matrixolm.NewAccount()
		if err != nil {
			return nil, err
		}
		m.olmAccount = account

		identity, signing := account.PrivateKeys()
		if err := m.store.Set("e2ee_account", map[string][]byte{
			"identity": identity,
			"signing":  signing,
		}, matrixE2EECacheTTL); err != nil {
			return nil, err
		}
	}

	// The upload is only valid for the identity that made it. A different
	// device id from the server, or a regenerated account, invalidates it.
	binding := fmt.Sprintf("%s|%s|%s|%s",
		m.userID, m.deviceID, m.olmAccount.IdentityKey(), m.olmAccount.SigningKey())

	var storedBinding string
	if !storeGetJSON(m.store, "e2ee_device_binding", &storedBinding) || storedBinding != binding {
		if err := m.uploadDeviceKeys(binding); err != nil {
			return nil, err
		}
	}

	return m.olmAccount, nil
}

func (m *MatrixTarget) uploadDeviceKeys(binding string) error {
	if m.userID == "" || m.deviceID == "" {
		return fmt.Errorf("matrix e2ee: keys cannot be uploaded before login assigns a device")
	}

	deviceKeys, err := m.olmAccount.DeviceKeys(m.userID, m.deviceID)
	if err != nil {
		return err
	}
	oneTimeKeys, err := m.olmAccount.GenerateOneTimeKeys(m.userID, m.deviceID, matrixOneTimeKeyCount)
	if err != nil {
		return err
	}

	ok, _, _ := m.fetch("/keys/upload", map[string]any{
		"device_keys":   deviceKeys,
		"one_time_keys": oneTimeKeys,
	}, nil, http.MethodPost, "")
	if !ok {
		return fmt.Errorf("matrix e2ee: device key upload failed")
	}

	return m.store.Set("e2ee_device_binding", binding, matrixE2EECacheTTL)
}

// roomEncrypted reports whether the server says the room is encrypted. The
// answer is cached, since it changes rarely and costs a request.
func (m *MatrixTarget) roomEncrypted(roomID string) bool {
	key := "e2ee_room_enc_" + roomID

	var cached bool
	if storeGetJSON(m.store, key, &cached) {
		return cached
	}

	// A 404 here means the room simply has no encryption state event.
	ok, response, _ := m.fetch(
		fmt.Sprintf("/rooms/%s/state/m.room.encryption", url.PathEscape(roomID)),
		nil, nil, http.MethodGet, "")
	encrypted := ok && len(response) > 0

	_ = m.store.Set(key, encrypted, matrixE2EECacheTTL)

	return encrypted
}

// roomMemberDevices returns every device of every joined member, keyed by user
// then device. A device whose keys fail their own signature is left out.
func (m *MatrixTarget) roomMemberDevices(roomID string) (map[string]map[string]matrixDeviceKeys, error) {
	ok, response, _ := m.fetch(
		fmt.Sprintf("/rooms/%s/joined_members", url.PathEscape(roomID)),
		nil, nil, http.MethodGet, "")
	if !ok {
		return nil, fmt.Errorf("matrix e2ee: could not query room members")
	}

	joined, _ := response["joined"].(map[string]any)
	if len(joined) == 0 {
		return map[string]map[string]matrixDeviceKeys{}, nil
	}

	query := map[string]any{}
	for userID := range joined {
		query[userID] = []string{}
	}

	ok, response, _ = m.fetch("/keys/query", map[string]any{"device_keys": query}, nil, http.MethodPost, "")
	if !ok {
		return nil, fmt.Errorf("matrix e2ee: could not query device keys")
	}

	result := map[string]map[string]matrixDeviceKeys{}
	deviceKeys, _ := response["device_keys"].(map[string]any)
	for userID, rawDevices := range deviceKeys {
		devices, _ := rawDevices.(map[string]any)
		for deviceID, rawInfo := range devices {
			info, _ := rawInfo.(map[string]any)
			if info == nil {
				continue
			}
			// Trusting an unverified device key would let the server hand us
			// a device of its choosing to encrypt to.
			if !matrixVerifyDeviceKeys(info, userID, deviceID) {
				continue
			}

			keys, _ := info["keys"].(map[string]any)
			curve, _ := keys[fmt.Sprintf("curve25519:%s", deviceID)].(string)
			ed, _ := keys[fmt.Sprintf("ed25519:%s", deviceID)].(string)
			if curve == "" {
				continue
			}

			if result[userID] == nil {
				result[userID] = map[string]matrixDeviceKeys{}
			}
			result[userID][deviceID] = matrixDeviceKeys{Curve25519: curve, Ed25519: ed}
		}
	}

	return result, nil
}

// shareRoomKey delivers the Megolm session key to every device in the room
// through one Olm message each. A device we cannot reach is skipped rather
// than failing the send: the others should still get the message.
func (m *MatrixTarget) shareRoomKey(roomID string, session *matrixolm.MegolmSession) error {
	members, err := m.roomMemberDevices(roomID)
	if err != nil {
		return err
	}
	if len(members) == 0 {
		return nil
	}

	claim := map[string]any{}
	for userID, devices := range members {
		perDevice := map[string]string{}
		for deviceID := range devices {
			// signed_curve25519 is what servers are required to support;
			// the unsigned variant is deprecated and usually yields nothing.
			perDevice[deviceID] = "signed_curve25519"
		}
		claim[userID] = perDevice
	}

	ok, response, _ := m.fetch("/keys/claim", map[string]any{"one_time_keys": claim}, nil, http.MethodPost, "")
	if !ok {
		return fmt.Errorf("matrix e2ee: could not claim one-time keys")
	}
	claimed, _ := response["one_time_keys"].(map[string]any)

	// The session key has to be captured before any message is encrypted:
	// the ratchet advances per message and a recipient cannot read anything
	// sent before the key it holds.
	roomKey := map[string]any{
		"algorithm":   "m.megolm.v1.aes-sha2",
		"room_id":     roomID,
		"session_id":  session.SessionID(),
		"session_key": session.SessionKey(),
	}

	messages := map[string]map[string]any{}
	for userID, devices := range members {
		for deviceID, device := range devices {
			// Encrypting to ourselves would mean starting an Olm session
			// with our own device.
			if userID == m.userID && deviceID == m.deviceID {
				continue
			}

			oneTimeKey := matrixClaimedOneTimeKey(claimed, userID, deviceID, device.Ed25519)
			if oneTimeKey == "" {
				continue
			}

			olmSession, err := m.olmAccount.NewOutboundSession(device.Curve25519, oneTimeKey)
			if err != nil {
				continue
			}

			inner, err := matrixolm.MarshalEvent(matrixRoomKeyEvent{
				Type:          "m.room_key",
				Content:       roomKey,
				Sender:        m.userID,
				Recipient:     userID,
				RecipientKeys: map[string]string{"ed25519": device.Ed25519},
				Keys:          map[string]string{"ed25519": m.olmAccount.SigningKey()},
			})
			if err != nil {
				continue
			}

			ciphertext, err := olmSession.Encrypt(inner)
			if err != nil {
				continue
			}

			if messages[userID] == nil {
				messages[userID] = map[string]any{}
			}
			messages[userID][deviceID] = map[string]any{
				"algorithm": "m.olm.v1.curve25519-aes-sha2",
				"ciphertext": map[string]any{
					device.Curve25519: map[string]any{
						"type": ciphertext.Type,
						"body": ciphertext.Body,
					},
				},
				"sender_key": m.olmAccount.IdentityKey(),
			}
		}
	}

	built := false
	for _, devices := range messages {
		if len(devices) > 0 {
			built = true
			break
		}
	}
	if !built {
		return nil
	}

	path := fmt.Sprintf("/sendToDevice/m.room.encrypted/%s", url.PathEscape(m.transactionValue()))
	ok, _, _ = m.fetch(path, map[string]any{"messages": messages}, nil, http.MethodPut, "")
	if !ok {
		return fmt.Errorf("matrix e2ee: could not deliver room key")
	}
	m.advanceTransaction()

	return nil
}

// matrixRoomKeyEvent is a struct rather than a map because the Olm ciphertext
// covers the serialized bytes: a map would be re-sorted by encoding/json and
// produce a ciphertext upstream never would.
type matrixRoomKeyEvent struct {
	Type          string            `json:"type"`
	Content       map[string]any    `json:"content"`
	Sender        string            `json:"sender"`
	Recipient     string            `json:"recipient"`
	RecipientKeys map[string]string `json:"recipient_keys"`
	Keys          map[string]string `json:"keys"`
}

// sendEncrypted encrypts one message with Megolm and sends it to the room,
// sharing the session key first whenever the session is new.
func (m *MatrixTarget) sendEncrypted(roomID, body, title string) error {
	account, err := m.e2eeAccount()
	if err != nil {
		return err
	}

	session, fresh, err := m.megolmSession(roomID)
	if err != nil {
		return err
	}

	var sharedID string
	if fresh || !storeGetJSON(m.store, "e2ee_key_shared_"+roomID, &sharedID) || sharedID != session.SessionID() {
		if err := m.shareRoomKey(roomID, session); err != nil {
			return err
		}
		if err := m.store.Set("e2ee_key_shared_"+roomID, session.SessionID(), matrixE2EECacheTTL); err != nil {
			return err
		}
	}

	content := map[string]any{
		"msgtype": fmt.Sprintf("m.%s", m.msgType),
		"body":    matrixBodyWithTitle(title, body),
	}
	switch m.notifyFormat {
	case "html":
		content["format"] = "org.matrix.custom.html"
		content["formatted_body"] = matrixHTMLBody(title, body)
	case "markdown":
		content["format"] = "org.matrix.custom.html"
		content["formatted_body"] = matrixMarkdownBody(title, body)
	}

	inner, err := matrixolm.MarshalEvent(matrixEncryptedEvent{
		Type:    "m.room.message",
		Content: content,
		RoomID:  roomID,
	})
	if err != nil {
		return err
	}

	ciphertext, err := session.Encrypt(inner)
	if err != nil {
		return err
	}
	if err := m.saveMegolmSession(roomID, session); err != nil {
		return err
	}

	path := fmt.Sprintf("/rooms/%s/send/m.room.encrypted/%s",
		url.PathEscape(roomID), url.PathEscape(m.transactionValue()))
	ok, _, _ := m.fetch(path, map[string]any{
		"algorithm":  "m.megolm.v1.aes-sha2",
		"ciphertext": ciphertext,
		"sender_key": account.IdentityKey(),
		"session_id": session.SessionID(),
		"device_id":  m.deviceID,
	}, nil, http.MethodPut, "")
	if !ok {
		return fmt.Errorf("matrix e2ee: encrypted send failed")
	}
	m.advanceTransaction()

	return nil
}

// sendEncryptedAttachment encrypts the file itself before uploading it, so
// the media server only ever holds ciphertext, then sends an ordinary
// encrypted event whose file field carries the key needed to read it back.
func (m *MatrixTarget) sendEncryptedAttachment(roomID string, attachment Attachment) error {
	account, err := m.e2eeAccount()
	if err != nil {
		return err
	}

	session, _, err := m.megolmSession(roomID)
	if err != nil {
		return err
	}

	ciphertext, fileInfo, err := encryptMatrixAttachment(attachment.Data)
	if err != nil {
		return err
	}

	uploadURL, err := m.buildURL("/upload", url.Values{"filename": {attachmentNameOrDefault(attachment)}}, "")
	if err != nil {
		return err
	}

	status, response, err := matrixSend(RequestSpec{
		Method: http.MethodPost,
		URL:    uploadURL,
		Headers: map[string]string{
			"User-Agent":    matrixDefaultUserAgent,
			"Content-Type":  "application/octet-stream",
			"Accept":        "application/json",
			"Authorization": "Bearer " + m.accessToken,
		},
		Body: string(ciphertext),
	})
	if err != nil {
		return err
	}

	contentURI, _ := response["content_uri"].(string)
	if status < http.StatusOK || status >= http.StatusMultipleChoices || contentURI == "" {
		return fmt.Errorf("matrix e2ee: media upload failed")
	}
	fileInfo["url"] = contentURI

	name := attachmentNameOrDefault(attachment)
	isImage := strings.HasPrefix(strings.ToLower(attachment.MimeType), "image/")

	content := map[string]any{
		"msgtype": "m.image",
		"body":    name,
		"file":    fileInfo,
		"info": map[string]any{
			"mimetype": attachment.MimeType,
			"size":     len(attachment.Data),
		},
	}
	if !isImage {
		content["msgtype"] = "m.file"
		content["filename"] = name
	}

	inner, err := matrixolm.MarshalEvent(matrixEncryptedEvent{
		Type:    "m.room.message",
		Content: content,
		RoomID:  roomID,
	})
	if err != nil {
		return err
	}

	encrypted, err := session.Encrypt(inner)
	if err != nil {
		return err
	}
	if err := m.saveMegolmSession(roomID, session); err != nil {
		return err
	}

	path := fmt.Sprintf("/rooms/%s/send/m.room.encrypted/%s",
		url.PathEscape(roomID), url.PathEscape(m.transactionValue()))
	ok, _, _ := m.fetch(path, map[string]any{
		"algorithm":  "m.megolm.v1.aes-sha2",
		"ciphertext": encrypted,
		"sender_key": account.IdentityKey(),
		"session_id": session.SessionID(),
		"device_id":  m.deviceID,
	}, nil, http.MethodPut, "")
	if !ok {
		return fmt.Errorf("matrix e2ee: encrypted attachment send failed")
	}
	m.advanceTransaction()

	return nil
}

func attachmentNameOrDefault(attachment Attachment) string {
	if attachment.Name == "" {
		return "file"
	}

	return attachment.Name
}

// encryptMatrixAttachment encrypts a file with AES-256-CTR under a one-time
// key, returning the ciphertext and the EncryptedFile object describing how to
// undo it. The IV is eight random bytes followed by eight zero bytes so the
// counter cannot wrap into another block's keystream.
func encryptMatrixAttachment(data []byte) ([]byte, map[string]any, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, nil, err
	}

	iv := make([]byte, 16)
	if _, err := rand.Read(iv[:8]); err != nil {
		return nil, nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}

	ciphertext := make([]byte, len(data))
	cipher.NewCTR(block, iv).XORKeyStream(ciphertext, data)

	digest := sha256.Sum256(ciphertext)

	return ciphertext, map[string]any{
		"v": "v2",
		"key": map[string]any{
			"kty":     "oct",
			"key_ops": []string{"encrypt", "decrypt"},
			"alg":     "A256CTR",
			"k":       base64.RawURLEncoding.EncodeToString(key),
			"ext":     true,
		},
		"iv":     base64.RawURLEncoding.EncodeToString(iv),
		"hashes": map[string]any{"sha256": base64.RawStdEncoding.EncodeToString(digest[:])},
	}, nil
}

type matrixEncryptedEvent struct {
	Type    string         `json:"type"`
	Content map[string]any `json:"content"`
	RoomID  string         `json:"room_id"`
}

// megolmSession restores the room's ratchet or starts one, reporting whether
// it is new so the caller knows the key needs sharing.
func (m *MatrixTarget) megolmSession(roomID string) (*matrixolm.MegolmSession, bool, error) {
	var stored struct {
		Ratchet [4][]byte `json:"ratchet"`
		Counter uint32    `json:"counter"`
		Signing []byte    `json:"signing"`
	}
	if storeGetJSON(m.store, "e2ee_megolm_"+roomID, &stored) && len(stored.Signing) > 0 {
		session, err := matrixolm.NewMegolmSessionFromState(stored.Ratchet, stored.Counter, stored.Signing)
		if err == nil {
			return session, false, nil
		}
	}

	session, err := matrixolm.NewMegolmSession()
	if err != nil {
		return nil, false, err
	}

	return session, true, nil
}

func (m *MatrixTarget) saveMegolmSession(roomID string, session *matrixolm.MegolmSession) error {
	ratchet, counter, signing := session.State()

	return m.store.Set("e2ee_megolm_"+roomID, map[string]any{
		"ratchet": ratchet,
		"counter": counter,
		"signing": signing,
	}, matrixE2EECacheTTL)
}

// matrixClaimedOneTimeKey pulls the signed one-time key for a device out of a
// /keys/claim response, verifying the signature before trusting it.
func matrixClaimedOneTimeKey(claimed map[string]any, userID, deviceID, ed25519Key string) string {
	byUser, _ := claimed[userID].(map[string]any)
	byDevice, _ := byUser[deviceID].(map[string]any)
	if byDevice == nil || ed25519Key == "" {
		return ""
	}

	for keyID, raw := range byDevice {
		if len(keyID) < len("signed_curve25519:") || keyID[:len("signed_curve25519:")] != "signed_curve25519:" {
			continue
		}
		object, _ := raw.(map[string]any)
		if object == nil {
			continue
		}
		if !matrixVerifySignedObject(object, userID, deviceID, ed25519Key) {
			return ""
		}
		key, _ := object["key"].(string)

		return key
	}

	return ""
}

// matrixVerifyDeviceKeys checks that a device signed its own key bundle.
func matrixVerifyDeviceKeys(info map[string]any, userID, deviceID string) bool {
	keys, _ := info["keys"].(map[string]any)
	ed, _ := keys[fmt.Sprintf("ed25519:%s", deviceID)].(string)
	if ed == "" {
		return false
	}

	return matrixVerifySignedObject(info, userID, deviceID, ed)
}

// matrixVerifySignedObject verifies a Matrix signature over the canonical JSON
// of an object with its signatures and unsigned fields removed.
func matrixVerifySignedObject(object map[string]any, userID, deviceID, ed25519Key string) bool {
	signatures, _ := object["signatures"].(map[string]any)
	byUser, _ := signatures[userID].(map[string]any)
	signature, _ := byUser[fmt.Sprintf("ed25519:%s", deviceID)].(string)
	if signature == "" {
		return false
	}

	stripped := map[string]any{}
	for key, value := range object {
		if key == "signatures" || key == "unsigned" {
			continue
		}
		stripped[key] = value
	}

	canonical, err := matrixolm.CanonicalJSON(stripped)
	if err != nil {
		return false
	}

	return matrixolm.VerifySignature(ed25519Key, canonical, signature)
}

// transactionValue is the transaction id for the next request.
func (m *MatrixTarget) transactionValue() string {
	if m.transactionIDString != "" {
		return m.transactionIDString
	}

	return fmt.Sprintf("%d", m.transactionID)
}

// advanceTransaction moves the counter on, keeping it in storage so a later
// process does not reuse an id the server has already seen.
func (m *MatrixTarget) advanceTransaction() {
	if m.transactionIDString != "" {
		return
	}

	m.transactionID++
	_ = m.store.Set("transaction_id", m.transactionID, matrixE2EECacheTTL)
}
