package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunNoArgsDoesNotReadStdin(t *testing.T) {
	oldStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = reader
	defer func() {
		os.Stdin = oldStdin
		_ = reader.Close()
		_ = writer.Close()
	}()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	done := make(chan int, 1)
	go func() {
		done <- Run([]string{}, stdout, stderr)
	}()

	select {
	case code := <-done:
		if code != 1 {
			t.Fatalf("expected exit code 1, got %d", code)
		}
	case <-time.After(500 * time.Millisecond):
		_ = writer.Close()
		select {
		case code := <-done:
			t.Fatalf("Run blocked on stdin (exit code %d)", code)
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("Run blocked on stdin after closing pipe")
		}
	}
}

func TestRunConvertsMarkdownInputForHTMLTargetFormat(t *testing.T) {
	requests := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- payload
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	targetURL := "json://" + server.Listener.Addr().String() + "/notify?format=html"
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := Run([]string{"-i", "markdown", "-b", "_This is Italics Text_", targetURL}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected success, got code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	select {
	case payload := <-requests:
		message, ok := payload["message"].(string)
		if !ok {
			t.Fatalf("missing message payload: %#v", payload)
		}
		if !strings.Contains(message, "<em>This is Italics Text</em>") {
			t.Fatalf("expected markdown converted to HTML, got %q", message)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for request")
	}
}
