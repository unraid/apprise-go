package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
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
	testutil.RequirePythonApprise(t)

	requests := make(chan notify.RequestSpec, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- captureRequestSpec(t, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	targetURL := "json://" + server.Listener.Addr().String() + "/notify?format=html"
	body := "_This is Italics Text_"

	pyStdout, pyStderr, err := testutil.RunApprise(t, "-i", "markdown", "-b", body, targetURL)
	if err != nil {
		t.Fatalf("python apprise failed: %v stdout=%s stderr=%s", err, pyStdout, pyStderr)
	}
	pythonRequests := readRequestSpecs(t, requests)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := Run([]string{"-i", "markdown", "-b", body, targetURL}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected success, got code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	goRequests := readRequestSpecs(t, requests)

	testutil.AssertRequestSpecSequenceMatches(t, pythonRequests, goRequests)
}

func TestRunSendsHTMLInputToTelegramHTMLTarget(t *testing.T) {
	assertRunHTTPRequestParity(t, "tgram://123456:abcdef/7890/?format=html", "<b>This is Bold Text</b>", "", "html")
}

func TestRunSendsMarkdownInputToTelegramMarkdownTarget(t *testing.T) {
	assertRunHTTPRequestParity(t, "tgram://123456:abcdef/7890/?format=markdown&mdv=v1", "_This is Italics Text_", "", "markdown")
}

func TestRunConvertsStandardMarkdownInputForTelegramMarkdownTarget(t *testing.T) {
	goSpecs := testutil.CaptureGoRequests(t, func() error {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		code := Run([]string{"-i", "markdown", "-b", "~~Strike~~ **Bold** _Italics_ Text", "tgram://123456:abcdef/7890/?format=markdown&mdv=v2"}, stdout, stderr)
		if code != 0 {
			return fmt.Errorf("Run failed with code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
		}
		return nil
	})
	if len(goSpecs) != 1 {
		t.Fatalf("expected one request, got %d", len(goSpecs))
	}

	payload := decodeJSONPayload(t, goSpecs[0].Body)
	if payload["parse_mode"] != "MarkdownV2" {
		t.Fatalf("expected MarkdownV2 parse mode, got %#v", payload["parse_mode"])
	}
	if payload["text"] != "~Strike~ *Bold* _Italics_ Text" {
		t.Fatalf("expected Telegram markdown body, got %#v", payload["text"])
	}
}

func TestRunConvertsMarkdownInputForMailtoHTMLTarget(t *testing.T) {
	testutil.RequirePythonApprise(t)

	capture := testutil.StartSMTPCapture(t)
	defer func() {
		_ = capture.Close()
	}()

	rawURL := "mailto://" + capture.Addr() + "/recipient@example.com?from=sender@example.com&format=html&mode=insecure"
	body := "_This is Italics Text_"

	pyStdout, pyStderr, err := testutil.RunApprise(t, "-i", "markdown", "-b", body, rawURL)
	if err != nil {
		t.Fatalf("python apprise failed: %v stdout=%s stderr=%s", err, pyStdout, pyStderr)
	}
	pythonMessages := capture.Messages()
	if len(pythonMessages) == 0 {
		t.Fatalf("no smtp message captured from python")
	}

	capture.Reset()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := Run([]string{"-i", "markdown", "-b", body, rawURL}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected success, got code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	goMessages := capture.Messages()
	if len(goMessages) == 0 {
		t.Fatalf("no smtp message captured from go")
	}

	testutil.AssertSMTPMessageSequencesMatch(t, pythonMessages, goMessages)
	normalized := testutil.NormalizeSMTPMessage(t, goMessages[0])
	if !strings.Contains(normalized.Body, "<em>This is Italics Text</em>") {
		t.Fatalf("expected converted markdown in email body, got %s", normalized.Body)
	}
}

func captureRequestSpec(t *testing.T, r *http.Request) notify.RequestSpec {
	t.Helper()
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	headers := map[string]string{}
	for key, values := range r.Header {
		headers[key] = strings.Join(values, ",")
	}

	return notify.RequestSpec{
		Method:  r.Method,
		URL:     "http://" + r.Host + r.URL.RequestURI(),
		Headers: headers,
		Body:    string(body),
	}
}

func readRequestSpecs(t *testing.T, requests <-chan notify.RequestSpec) []notify.RequestSpec {
	t.Helper()

	specs := []notify.RequestSpec{}
	for {
		select {
		case spec := <-requests:
			specs = append(specs, spec)
		case <-time.After(time.Second):
			if len(specs) == 0 {
				t.Fatalf("timed out waiting for request")
			}
			return specs
		}
	}
}

func assertRunHTTPRequestParity(t *testing.T, rawURL, body, title, inputFormat string) {
	t.Helper()
	testutil.RequirePythonApprise(t)

	pythonSpecs := testutil.CapturePythonRequestsWithFormat(t, rawURL, body, title, inputFormat)

	goSpecs := testutil.CaptureGoRequests(t, func() error {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		args := []string{"-i", inputFormat, "-b", body}
		if title != "" {
			args = append(args, "-t", title)
		}
		args = append(args, rawURL)

		code := Run(args, stdout, stderr)
		if code != 0 {
			return fmt.Errorf("Run failed with code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
		}
		return nil
	})

	testutil.AssertRequestSpecSequenceMatches(t, pythonSpecs, goSpecs)
}

func decodeJSONPayload(t *testing.T, body string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode json payload: %v", err)
	}
	return payload
}
