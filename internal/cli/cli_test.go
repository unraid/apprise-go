package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
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

func TestRunSendsHTMLInputToTelegramHTMLTarget(t *testing.T) {
	payloads := captureHTTPPayloads(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := Run([]string{
		"-i", "html",
		"-b", "<b>This is Bold Text</b>",
		"tgram://123456:abcdef/7890/?format=html",
	}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected success, got code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	payload := readPayload(t, payloads)
	if payload["parse_mode"] != "HTML" {
		t.Fatalf("expected HTML parse mode, got %#v", payload["parse_mode"])
	}
	if payload["text"] != "<b>This is Bold Text</b>" {
		t.Fatalf("expected unescaped HTML body, got %#v", payload["text"])
	}
}

func TestRunSendsMarkdownInputToTelegramMarkdownTarget(t *testing.T) {
	payloads := captureHTTPPayloads(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := Run([]string{
		"-i", "markdown",
		"-b", "_This is Italics Text_",
		"tgram://123456:abcdef/7890/?format=markdown&mdv=v1",
	}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected success, got code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	payload := readPayload(t, payloads)
	if payload["parse_mode"] != "Markdown" {
		t.Fatalf("expected Markdown parse mode, got %#v", payload["parse_mode"])
	}
	if payload["text"] != "_This is Italics Text_" {
		t.Fatalf("expected markdown body, got %#v", payload["text"])
	}
}

func TestRunConvertsMarkdownInputForMailtoHTMLTarget(t *testing.T) {
	messages := startSMTPServer(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := Run([]string{
		"-i", "markdown",
		"-b", "_This is Italics Text_",
		"mailto://" + messages.addr + "/recipient@example.com?from=sender@example.com&format=html&mode=insecure",
	}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected success, got code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	message := messages.read(t)
	if !strings.Contains(message, "Content-Type: text/html") {
		t.Fatalf("expected html email, got %s", message)
	}
	if !strings.Contains(message, "<em>This is Italics Text</em>") {
		t.Fatalf("expected converted markdown in email body, got %s", message)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func captureHTTPPayloads(t *testing.T) <-chan map[string]any {
	t.Helper()
	payloads := make(chan map[string]any, 1)
	oldClient := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			defer req.Body.Close()
			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Errorf("decode request: %v", err)
			}
			payloads <- payload
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("{}")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	}
	t.Cleanup(func() {
		http.DefaultClient = oldClient
	})
	return payloads
}

func readPayload(t *testing.T, payloads <-chan map[string]any) map[string]any {
	t.Helper()
	select {
	case payload := <-payloads:
		return payload
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for request")
		return nil
	}
}

type smtpMessages struct {
	addr string
	ch   <-chan string
}

func startSMTPServer(t *testing.T) smtpMessages {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen smtp: %v", err)
	}
	messages := make(chan string, 1)
	done := make(chan struct{})
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
	})

	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		writer := bufio.NewWriter(conn)
		writeSMTPLine(t, writer, "220 localhost ESMTP")
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			command := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(command, "EHLO"), strings.HasPrefix(command, "HELO"):
				writeSMTPLine(t, writer, "250-localhost")
				writeSMTPLine(t, writer, "250 OK")
			case strings.HasPrefix(command, "MAIL FROM:"), strings.HasPrefix(command, "RCPT TO:"):
				writeSMTPLine(t, writer, "250 OK")
			case command == "DATA":
				writeSMTPLine(t, writer, "354 End data with <CR><LF>.<CR><LF>")
				var data strings.Builder
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					if strings.TrimSpace(line) == "." {
						break
					}
					data.WriteString(line)
				}
				messages <- data.String()
				writeSMTPLine(t, writer, "250 OK")
			case command == "QUIT":
				writeSMTPLine(t, writer, "221 Bye")
				return
			default:
				writeSMTPLine(t, writer, "250 OK")
			}
		}
	}()

	return smtpMessages{
		addr: listener.Addr().String(),
		ch:   messages,
	}
}

func (s smtpMessages) read(t *testing.T) string {
	t.Helper()
	select {
	case message := <-s.ch:
		return message
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for smtp message")
		return ""
	}
}

func writeSMTPLine(t *testing.T, writer *bufio.Writer, line string) {
	t.Helper()
	if _, err := writer.WriteString(line + "\r\n"); err != nil {
		t.Errorf("write smtp response: %v", err)
		return
	}
	if err := writer.Flush(); err != nil {
		t.Errorf("flush smtp response: %v", err)
	}
}
