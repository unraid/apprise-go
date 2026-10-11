package parity

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

// TestMatrixSetupFailureRequestParity proves setup fallback uses upstream's
// request flow; a failed account lookup leaves device-key setup unavailable.
func TestMatrixSetupFailureRequestParity(t *testing.T) {
	const url = "matrixs://token@example.com:1/!room?discovery=no"
	pythonSpecs, pythonSuccess := testutil.CapturePythonRequestsWithFailuresResult(t, url, "body", "title", 1)
	parsed, err := notify.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	restore := testutil.FailNextRequests(1)
	t.Cleanup(restore)
	goSpecs, err := testutil.CaptureGoRequestsResult(t, func() error {
		// Match the fresh per-instance Python memory store for this capture.
		parsed.Query["store"] = "no"
		target, err := notify.NewMatrixTarget(parsed)
		if err != nil {
			return err
		}
		return target.Send("body", "title", notify.NotifyInfo)
	})
	if pythonSuccess == nil || !*pythonSuccess || err != nil {
		t.Fatalf("setup fallback failed: python=%v go=%v", pythonSuccess, err)
	}
	assertRequestSpecSequenceMatches(t, pythonSpecs, goSpecs)
}

// TestMatrixKeyUploadRecoveryRequestParity compares two real Send calls against
// a plaintext room: a failed key upload falls back, then the next Send retries.
func TestMatrixKeyUploadRecoveryRequestParity(t *testing.T) {
	testMatrixKeyUploadRecoveryRequestParity(t, false)
}

func TestMatrixWhoamiPreservesDeviceRequestParity(t *testing.T) {
	testMatrixKeyUploadRecoveryRequestParity(t, true)
}

func testMatrixKeyUploadRecoveryRequestParity(t *testing.T, login bool) {
	testutil.RequirePythonApprise(t)
	var mu sync.Mutex
	var requests []notify.RequestSpec
	var uploads, messages int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		headers := make(map[string]string, len(r.Header))
		for key, values := range r.Header {
			headers[key] = strings.Join(values, ",")
		}
		mu.Lock()
		defer mu.Unlock()
		requests = append(requests, notify.RequestSpec{
			Method: r.Method, URL: "https://" + r.Host + r.URL.RequestURI(),
			Headers: headers, Body: string(body),
		})
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/login"):
			_, _ = fmt.Fprint(w, `{"access_token":"token","device_id":"DEVICE"}`)
		case strings.HasSuffix(r.URL.Path, "/account/whoami"):
			if login {
				_, _ = fmt.Fprint(w, `{"user_id":"@user:example.com"}`)
			} else {
				_, _ = fmt.Fprint(w, `{"user_id":"@user:example.com","device_id":"DEVICE"}`)
			}
		case strings.HasSuffix(r.URL.Path, "/keys/upload"):
			uploads++
			if uploads == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = fmt.Fprint(w, `{"error":"key upload unavailable"}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"one_time_key_counts":{"signed_curve25519":10}}`)
		case strings.Contains(r.URL.Path, "/join/"):
			if !strings.HasSuffix(r.URL.Path, "/join/#alerts:example.com") {
				t.Errorf("room was resolved against the API endpoint instead of account domain: %s", r.URL.Path)
			}
			_, _ = fmt.Fprint(w, `{"room_id":"!room:example.com"}`)
		case strings.Contains(r.URL.Path, "/state/m.room.encryption"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{}`)
		case strings.Contains(r.URL.Path, "/send/m.room.message/"):
			messages++
			_, _ = fmt.Fprint(w, `{"event_id":"$message"}`)
		default:
			t.Errorf("unexpected Matrix request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	certPath := filepath.Join(t.TempDir(), "matrix-recovery-ca.pem")
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(certPath, cert, 0600); err != nil {
		t.Fatal(err)
	}
	credentials := "token"
	if login {
		credentials = "user:password"
	}
	url := "matrixs://" + credentials + "@" + strings.TrimPrefix(server.URL, "https://") + "/alerts?discovery=no"
	script := filepath.Join(testutil.RepoRoot(t), "internal/testutil/scripts/matrix_upload_recovery.py")
	stdout, stderr, err := testutil.RunPythonScript(t, script, url, certPath)
	if err != nil {
		t.Fatalf("Python recovery sends failed: %v (%s)", err, stderr)
	}
	var pythonSuccess []bool
	if err := json.Unmarshal([]byte(stdout), &pythonSuccess); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	pythonSpecs := requests
	pythonUploads, pythonMessages := uploads, messages
	requests, uploads, messages = nil, 0, 0
	mu.Unlock()
	if pythonUploads != 2 || pythonMessages != 2 {
		t.Fatalf("Python did not retry keys and deliver both messages: uploads=%d messages=%d", pythonUploads, pythonMessages)
	}
	previousClient := http.DefaultClient
	http.DefaultClient = server.Client()
	t.Cleanup(func() { http.DefaultClient = previousClient })
	parsed, err := notify.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	// This task-local TLS endpoint is fresh. Keep one default provider Store
	// across both sends so successful room caching and key retry are exercised.
	target, err := notify.NewMatrixTarget(parsed)
	if err != nil {
		t.Fatal(err)
	}
	goSuccess := make([]bool, 2)
	for i := range goSuccess {
		err := target.Send(fmt.Sprintf("body%d", i), fmt.Sprintf("title%d", i), notify.NotifyInfo)
		goSuccess[i] = err == nil
		if err != nil {
			t.Errorf("Go Send %d failed: %v", i, err)
		}
	}
	mu.Lock()
	goSpecs := requests
	goUploads, goMessages := uploads, messages
	mu.Unlock()
	if !reflect.DeepEqual(pythonSuccess, []bool{true, true}) || !reflect.DeepEqual(pythonSuccess, goSuccess) {
		t.Fatalf("delivery outcomes differ: python=%v go=%v", pythonSuccess, goSuccess)
	}
	if goUploads != 2 || goMessages != 2 {
		t.Fatalf("Go did not retry keys and deliver both messages: uploads=%d messages=%d", goUploads, goMessages)
	}
	// The existing receiver verifies both signed uploads before normalization;
	// every request, including the failed upload and both deliveries, is compared.
	for _, side := range []struct {
		name  string
		specs []notify.RequestSpec
	}{{"Python", pythonSpecs}, {"Go", goSpecs}} {
		for _, request := range side.specs {
			t.Logf("%s request: %s %s", side.name, request.Method, request.URL)
		}
	}
	assertRequestSpecSequenceMatches(t, pythonSpecs, goSpecs)
	t.Logf("both implementations delivered two sends after keys/upload500, then recovery; requests=%d", len(goSpecs))
}
