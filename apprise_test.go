package apprise

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/unraid/apprise-go/internal/testutil"
)

func TestAppriseSendJSONTarget(t *testing.T) {
	testutil.RequirePythonApprise(t)

	targetURL := "json://example.com/notify"
	body := "hello"
	title := "Greeting"
	pythonRequests := testutil.CapturePythonRequestsWithFormat(t, targetURL, body, title, "text")

	goRequests := testutil.CaptureGoRequests(t, func() error {
		client := New()
		if err := client.Add(targetURL); err != nil {
			return err
		}
		return client.Send(body, WithTitle(title))
	})

	testutil.AssertRequestSpecSequenceMatches(t, pythonRequests, goRequests)
}

func TestAppriseSendConvertsInputFormat(t *testing.T) {
	requests := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			_ = r.Body.Close()
		}()

		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Errorf("decode json: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- payload
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := Send(
		[]string{"json://" + server.Listener.Addr().String() + "/notify?format=html"},
		"_hello_",
		WithInputFormat("markdown"),
	); err != nil {
		t.Fatalf("send: %v", err)
	}

	payload := readPayload(t, requests)
	if got := payload["message"]; got != "<p><em>hello</em></p>\n" {
		t.Fatalf("message = %q, want converted html", got)
	}
}

func TestAppriseSendNoTargets(t *testing.T) {
	err := New().Send("hello")
	if !errors.Is(err, ErrNoTargets) {
		t.Fatalf("error = %v, want ErrNoTargets", err)
	}
}

func TestAppriseAddRejectsUnsupportedSchema(t *testing.T) {
	err := New().Add("unknown://example.com")
	if err == nil || !strings.Contains(err.Error(), "unsupported URL scheme: unknown") {
		t.Fatalf("error = %v, want unsupported schema", err)
	}
}

func readPayload(t *testing.T, requests <-chan map[string]any) map[string]any {
	t.Helper()

	select {
	case payload := <-requests:
		return payload
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for request")
		return nil
	}
}
