package parity

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

func TestMailtoStartTLSParity(t *testing.T) {
	capture := testutil.StartSMTPStartTLSCapture(t)
	defer func() {
		_ = capture.Close()
	}()

	host, port, err := net.SplitHostPort(capture.Addr())
	if err != nil {
		t.Fatalf("smtp host split failed: %v", err)
	}

	t.Setenv("SSL_CERT_FILE", testutil.TestTLSCACertPath(t))

	url := fmt.Sprintf(
		"mailtos://%s:%s?from=sender@example.com&to=recipient@example.com&mode=starttls&verify=yes",
		host,
		port,
	)

	body := "<b>apprise</b> parity body<br />second line"
	title := "apprise parity title"

	runSMTPParityCase(t, capture, url, body, title)
}

func TestMailtoSSLParity(t *testing.T) {
	capture := testutil.StartSMTPTLSCapture(t)
	defer func() {
		_ = capture.Close()
	}()

	host, port, err := net.SplitHostPort(capture.Addr())
	if err != nil {
		t.Fatalf("smtp host split failed: %v", err)
	}

	t.Setenv("SSL_CERT_FILE", testutil.TestTLSCACertPath(t))

	url := fmt.Sprintf(
		"mailtos://%s:%s?from=sender@example.com&to=recipient@example.com&mode=ssl&verify=yes",
		host,
		port,
	)

	body := "<b>apprise</b> parity body<br />second line"
	title := "apprise parity title"

	runSMTPParityCase(t, capture, url, body, title)
}

func runSMTPParityCase(t *testing.T, capture *testutil.SMTPCapture, url, body, title string) {
	t.Helper()

	appriseRoot := testutil.AppriseSourceRoot(t)
	t.Setenv("PYTHONPATH", appriseRoot)

	script := filepath.Join(testutil.RepoRoot(t), "internal", "testutil", "scripts", "capture_smtp.py")
	stdout, stderr, err := testutil.RunPythonScript(t, script, "--url", url, "--body", body, "--title", title)
	if err != nil {
		t.Fatalf("python smtp send failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	var result smtpSendResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse python smtp result failed: %v (stdout: %s)", err, strings.TrimSpace(stdout))
	}
	if !result.Success {
		t.Fatalf("python smtp send reported failure: %s", strings.TrimSpace(stdout))
	}

	pythonMsgs := capture.Messages()
	if len(pythonMsgs) == 0 {
		t.Fatalf("no smtp message captured from python")
	}
	pythonMsg := pythonMsgs[len(pythonMsgs)-1]

	capture.Reset()

	parsed, err := notify.ParseURL(url)
	if err != nil {
		t.Fatalf("parse mailto url: %v", err)
	}
	target, err := notify.NewMailtoTarget(parsed)
	if err != nil {
		t.Fatalf("build mailto target: %v", err)
	}
	if err := target.Send(body, title, notify.NotifyInfo); err != nil {
		t.Fatalf("go mailto send failed: %v", err)
	}

	goMsgs := capture.Messages()
	if len(goMsgs) == 0 {
		t.Fatalf("no smtp message captured from go")
	}
	goMsg := goMsgs[len(goMsgs)-1]

	pythonNormalized := normalizeSMTPMessage(t, pythonMsg)
	goNormalized := normalizeSMTPMessage(t, goMsg)

	if !equalNormalizedSMTP(pythonNormalized, goNormalized) {
		pyJSON, _ := json.Marshal(pythonNormalized)
		goJSON, _ := json.Marshal(goNormalized)
		t.Fatalf("smtp parity mismatch\npython=%s\ngo=%s", pyJSON, goJSON)
	}
}
