package notify_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

// TestMatrixE2EEFlow drives a full encrypted send and checks the request
// sequence, because the pieces passing individually says nothing about
// whether they run in the right order — a room key shared after the message
// it unlocks is useless to the recipient.
func TestMatrixE2EEFlow(t *testing.T) {
	notify.ConfigureStorage("", 8, nil)
	t.Cleanup(func() { notify.ConfigureStorage("", 8, nil) })

	specs := testutil.CaptureGoRequests(t, func() error {
		return notify.SendTargetURL(
			"matrixs://user:pass@matrix.example.com/%23room:example.com?e2ee=yes",
			"secret body", "secret title", "", notify.NotifyInfo)
	})

	seen := map[string]bool{}
	encryptedIndex, toDeviceIndex := -1, -1
	for i, spec := range specs {
		switch {
		case strings.HasSuffix(spec.URL, "/keys/upload"):
			seen["keys/upload"] = true
		case strings.Contains(spec.URL, "/state/m.room.encryption"):
			seen["m.room.encryption"] = true
		case strings.HasSuffix(spec.URL, "/joined_members"):
			seen["joined_members"] = true
		case strings.HasSuffix(spec.URL, "/keys/query"):
			seen["keys/query"] = true
		case strings.HasSuffix(spec.URL, "/keys/claim"):
			seen["keys/claim"] = true
		case strings.Contains(spec.URL, "/sendToDevice/m.room.encrypted"):
			seen["sendToDevice"] = true
			toDeviceIndex = i
		case strings.Contains(spec.URL, "/send/m.room.encrypted"):
			encryptedIndex = i
		}
	}

	for _, step := range []string{
		"keys/upload", "m.room.encryption", "joined_members",
		"keys/query", "keys/claim", "sendToDevice",
	} {
		if !seen[step] {
			t.Errorf("encrypted send never issued %s", step)
		}
	}

	if encryptedIndex < 0 {
		t.Fatal("no m.room.encrypted send was issued")
	}
	// A recipient cannot read a message encrypted under a session key it has
	// not been given yet.
	if toDeviceIndex > encryptedIndex {
		t.Fatalf("room key shared after the message it unlocks (key at %d, message at %d)",
			toDeviceIndex, encryptedIndex)
	}

	final := specs[encryptedIndex]
	if strings.Contains(final.Body, "secret body") || strings.Contains(final.Body, "secret title") {
		t.Fatalf("plaintext leaked into the encrypted send: %s", final.Body)
	}

	var payload struct {
		Algorithm  string `json:"algorithm"`
		Ciphertext string `json:"ciphertext"`
		SessionID  string `json:"session_id"`
		SenderKey  string `json:"sender_key"`
	}
	if err := json.Unmarshal([]byte(final.Body), &payload); err != nil {
		t.Fatalf("encrypted payload is not json: %v", err)
	}
	if payload.Algorithm != "m.megolm.v1.aes-sha2" {
		t.Fatalf("unexpected algorithm %q", payload.Algorithm)
	}
	if payload.Ciphertext == "" || payload.SessionID == "" || payload.SenderKey == "" {
		t.Fatalf("encrypted payload is missing fields: %+v", payload)
	}
}

// TestMatrixE2EEDisabledSendsPlaintext confirms ?e2ee=no is a real off
// switch rather than advisory.
func TestMatrixE2EEDisabledSendsPlaintext(t *testing.T) {
	notify.ConfigureStorage("", 8, nil)
	t.Cleanup(func() { notify.ConfigureStorage("", 8, nil) })

	specs := testutil.CaptureGoRequests(t, func() error {
		return notify.SendTargetURL(
			"matrixs://user:pass@matrix.example.com/%23room:example.com?e2ee=no",
			"body", "title", "", notify.NotifyInfo)
	})

	for _, spec := range specs {
		if strings.Contains(spec.URL, "m.room.encrypted") {
			t.Fatal("e2ee=no still produced an encrypted send")
		}
	}
}
