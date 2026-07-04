package notify

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadHTTPAttachmentRejectsOversizeResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("12345"))
	}))
	defer server.Close()

	_, err := readHTTPAttachmentWithClient(server.URL, server.Client(), 4)
	if err == nil || !strings.Contains(err.Error(), "attachment exceeds maximum size") {
		t.Fatalf("error = %v, want maximum size error", err)
	}
}

func TestReadHTTPAttachmentAllowsMaxSizeResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("1234"))
	}))
	defer server.Close()

	data, err := readHTTPAttachmentWithClient(server.URL, server.Client(), 4)
	if err != nil {
		t.Fatalf("read attachment: %v", err)
	}
	if string(data) != "1234" {
		t.Fatalf("data = %q, want %q", string(data), "1234")
	}
}
