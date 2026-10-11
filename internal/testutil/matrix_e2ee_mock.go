package testutil

import (
	"crypto/ecdh"
	"encoding/json"
	"sync"

	"github.com/unraid/apprise-go/internal/matrixolm"
)

// A recipient device for Matrix end-to-end encryption. The provider verifies
// the signatures on device keys and one-time keys before it will encrypt to a
// device, so the mock signs them with a real account rather than returning
// fixed strings that would — correctly — be rejected.
const (
	matrixE2EEUserID   = "@target:example.com"
	matrixE2EEDeviceID = "TARGETDEVICE"
)

var (
	matrixE2EEOnce    sync.Once
	matrixE2EEAccount *matrixolm.Account
)

func matrixE2EERecipient() *matrixolm.Account {
	matrixE2EEOnce.Do(func() {
		identity, signing := make([]byte, 32), make([]byte, 32)
		for i := range identity {
			identity[i], signing[i] = byte(200-i), byte(255-i)
		}
		account, err := matrixolm.NewAccountFromKeys(identity, signing)
		if err != nil {
			panic("matrix e2ee mock: " + err.Error())
		}
		matrixE2EEAccount = account
	})

	return matrixE2EEAccount
}

func matrixE2EEDeviceKeys() string {
	keys, err := matrixE2EERecipient().DeviceKeys(matrixE2EEUserID, matrixE2EEDeviceID)
	if err != nil {
		return `{"device_keys":{}}`
	}

	data, err := json.Marshal(map[string]any{
		"device_keys": map[string]any{
			matrixE2EEUserID: map[string]any{matrixE2EEDeviceID: keys},
		},
	})
	if err != nil {
		return `{"device_keys":{}}`
	}

	return string(data)
}

func matrixE2EEClaimedKey() string {
	// Synthetic receiver keys are known to the independent receipt verifier.
	// Sender keys and ciphertext remain random and must actually decrypt.
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(50 + i)
	}
	key, err := ecdh.X25519().NewPrivateKey(seed)
	if err != nil {
		panic(err)
	}
	object := map[string]any{"key": matrixolm.EncodeBase64(key.PublicKey().Bytes())}
	canonical, err := matrixolm.CanonicalJSON(object)
	if err != nil {
		panic(err)
	}
	object["signatures"] = map[string]any{matrixE2EEUserID: map[string]any{"ed25519:" + matrixE2EEDeviceID: matrixE2EERecipient().Sign(canonical)}}
	keys := map[string]any{"signed_curve25519:fixture": object}

	data, err := json.Marshal(map[string]any{
		"one_time_keys": map[string]any{
			matrixE2EEUserID: map[string]any{matrixE2EEDeviceID: keys},
		},
	})
	if err != nil {
		return `{"one_time_keys":{}}`
	}

	return string(data)
}
