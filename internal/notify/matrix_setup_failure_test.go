package notify

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// matrixUploadFailureTarget rejects the first device-key upload and serves a
// plaintext room, allowing delivery and later key publication to be observed.
func matrixUploadFailureTarget(t *testing.T) (*MatrixTarget, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	var uploads, messages atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/account/whoami"):
			_, _ = fmt.Fprint(w, `{"user_id":"@user:example.com","device_id":"DEVICE"}`)
		case strings.HasSuffix(r.URL.Path, "/keys/upload"):
			if uploads.Add(1) == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = fmt.Fprint(w, `{"error":"key upload unavailable"}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"one_time_key_counts":{"signed_curve25519":10}}`)
		case strings.Contains(r.URL.Path, "/join/"):
			_, _ = fmt.Fprint(w, `{"room_id":"!room:example.com"}`)
		case strings.Contains(r.URL.Path, "/state/m.room.encryption"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{}`)
		case strings.Contains(r.URL.Path, "/send/m.room.message/"):
			messages.Add(1)
			_, _ = fmt.Fprint(w, `{"event_id":"$message"}`)
		default:
			t.Errorf("unexpected Matrix request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	previousClient := http.DefaultClient
	http.DefaultClient = server.Client()
	t.Cleanup(func() {
		http.DefaultClient = previousClient
		server.Close()
	})
	parsed, err := ParseURL("matrixs://token@" + strings.TrimPrefix(server.URL, "https://") + "/!room?discovery=no")
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewMatrixTarget(parsed)
	if err != nil {
		t.Fatal(err)
	}
	return target, &uploads, &messages
}

// TestMatrixSetupFailureStillDeliversPlaintext catches a device-key endpoint
// outage incorrectly blocking rooms that do not require encryption.
func TestMatrixSetupFailureStillDeliversPlaintext(t *testing.T) {
	target, uploads, messages := matrixUploadFailureTarget(t)
	if err := target.Send("body", "title", NotifyInfo); err != nil {
		t.Fatalf("key upload failure blocked plaintext delivery: %v", err)
	}
	if uploads.Load() != 1 || messages.Load() != 1 {
		t.Fatalf("expected a failed key upload followed by a message: uploads=%d messages=%d", uploads.Load(), messages.Load())
	}
	if !target.e2ee {
		t.Fatal("setup failure changed the configured encryption setting")
	}
}

// TestMatrixAccountRetriesFailedKeyUpload catches an in-memory account being
// mistaken for successful device-key publication on the next attempt.
func TestMatrixAccountRetriesFailedKeyUpload(t *testing.T) {
	target, uploads, _ := matrixUploadFailureTarget(t)
	target.userID, target.deviceID = "@user:example.com", "DEVICE"
	if _, err := target.e2eeAccount(); err == nil {
		t.Fatal("failed key upload reported success")
	}
	if _, err := target.e2eeAccount(); err != nil {
		t.Fatalf("key upload did not recover: %v", err)
	}
	if uploads.Load() != 2 {
		t.Fatalf("device keys were not retried: uploads=%d", uploads.Load())
	}
}
