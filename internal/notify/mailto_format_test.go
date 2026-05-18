package notify_test

import (
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

type smtpFormatSendResult struct {
	Success bool `json:"success"`
}

func TestMailtoHTMLFormatAcceptsConvertedMarkdownBody(t *testing.T) {
	testutil.RequirePythonApprise(t)

	capture := testutil.StartSMTPCapture(t)
	defer func() {
		_ = capture.Close()
	}()

	host, port, err := net.SplitHostPort(capture.Addr())
	if err != nil {
		t.Fatalf("smtp host split failed: %v", err)
	}
	rawURL := fmt.Sprintf("mailto://%s/recipient@example.com?from=sender@example.com&format=html&mode=insecure", net.JoinHostPort(host, port))

	script := filepath.Join(testutil.RepoRoot(t), "internal", "testutil", "scripts", "capture_smtp.py")
	stdout, stderr, err := testutil.RunPythonScript(
		t,
		script,
		"--url", rawURL,
		"--body", "_This is Italics Text_",
		"--title", "subject",
		"--body-format", "markdown",
	)
	if err != nil {
		t.Fatalf("python smtp send failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}
	var result smtpFormatSendResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse python smtp result failed: %v (stdout: %s)", err, strings.TrimSpace(stdout))
	}
	if !result.Success {
		t.Fatalf("python smtp send reported failure: %s", strings.TrimSpace(stdout))
	}
	pythonMessages := capture.Messages()
	if len(pythonMessages) == 0 {
		t.Fatalf("no smtp message captured from python")
	}

	capture.Reset()

	parsed, err := notify.ParseURL(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	target, err := notify.NewMailtoTarget(parsed)
	if err != nil {
		t.Fatalf("new mailto target: %v", err)
	}
	body, err := notify.ConvertMessageFormat("_This is Italics Text_", "markdown", "html")
	if err != nil {
		t.Fatalf("convert body: %v", err)
	}

	if err := target.Send(body, "subject", notify.NotifyInfo); err != nil {
		t.Fatalf("go mailto send failed: %v", err)
	}
	goMessages := capture.Messages()
	if len(goMessages) == 0 {
		t.Fatalf("no smtp message captured from go")
	}

	testutil.AssertSMTPMessageSequencesMatch(t, pythonMessages, goMessages)

	normalized := testutil.NormalizeSMTPMessage(t, goMessages[0])
	if !strings.Contains(normalized.Body, "<em>This is Italics Text</em>") {
		t.Fatalf("expected converted markdown in html part, got %s", normalized.Body)
	}
}
