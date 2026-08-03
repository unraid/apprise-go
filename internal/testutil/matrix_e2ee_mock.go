package testutil

import (
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
		account, err := matrixolm.NewAccount()
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
	keys, err := matrixE2EERecipient().GenerateOneTimeKeys(matrixE2EEUserID, matrixE2EEDeviceID, 1)
	if err != nil {
		return `{"one_time_keys":{}}`
	}

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
