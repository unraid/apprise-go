package notify

import (
	"testing"

	"github.com/unraid/apprise-go/internal/matrixolm"
)

// TestMatrixE2EERejectsUnsignedDeviceKeys is the security property: a
// homeserver that hands back a device whose keys are not signed by that device
// must not receive a room key. Without this check the server could name a
// device of its choosing and read everything.
func TestMatrixE2EERejectsUnsignedDeviceKeys(t *testing.T) {
	account, err := matrixolm.NewAccount()
	if err != nil {
		t.Fatalf("account: %v", err)
	}

	keys, err := account.DeviceKeys("@target:example.com", "TARGETDEVICE")
	if err != nil {
		t.Fatalf("device keys: %v", err)
	}
	if !matrixVerifyDeviceKeys(keys, "@target:example.com", "TARGETDEVICE") {
		t.Fatal("a device's own signature was rejected")
	}

	// Same keys, signature belonging to someone else.
	tampered := map[string]any{}
	for key, value := range keys {
		tampered[key] = value
	}
	tampered["signatures"] = map[string]any{
		"@target:example.com": map[string]any{
			"ed25519:TARGETDEVICE": "aW52YWxpZHNpZ25hdHVyZQ",
		},
	}
	if matrixVerifyDeviceKeys(tampered, "@target:example.com", "TARGETDEVICE") {
		t.Fatal("a forged device signature was accepted")
	}
}
