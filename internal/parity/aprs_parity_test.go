package parity

import (
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

type aprsSendResult struct {
	Success bool `json:"success"`
}

func TestAprsParity(t *testing.T) {
	capture := testutil.StartAPRSCapture(t)
	defer func() {
		_ = capture.Close()
	}()

	host, port, err := net.SplitHostPort(capture.Addr())
	if err != nil {
		t.Fatalf("aprs host split failed: %v", err)
	}

	appriseRoot := testutil.AppriseSourceRoot(t)
	t.Setenv("PYTHONPATH", appriseRoot)
	t.Setenv("APPRISE_APRS_TEST_HOST", host)
	t.Setenv("APPRISE_APRS_TEST_PORT", port)

	script := filepath.Join(testutil.RepoRoot(t), "internal", "testutil", "scripts", "capture_aprs.py")
	url := "aprs://AB1CD:12345@XY1ZZ/ZZ9PL?locale=EURO"
	body := "apprise parity body"
	title := ""

	stdout, stderr, err := testutil.RunPythonScript(t, script,
		"--url", url,
		"--body", body,
		"--title", title,
		"--host", host,
		"--port", port,
	)
	if err != nil {
		t.Fatalf("python aprs send failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	var result aprsSendResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse python aprs result failed: %v (stdout: %s)", err, strings.TrimSpace(stdout))
	}
	if !result.Success {
		t.Fatalf("python aprs send reported failure: %s", strings.TrimSpace(stdout))
	}

	waitForAPRSMessages(t, capture, 2)
	pythonMessages := capture.Messages()

	capture.Reset()

	parsed, err := notify.ParseURL(url)
	if err != nil {
		t.Fatalf("parse aprs url: %v", err)
	}
	target, err := notify.NewAprsTarget(parsed)
	if err != nil {
		t.Fatalf("build aprs target: %v", err)
	}
	if err := target.Send(body, title, notify.NotifyInfo); err != nil {
		t.Fatalf("go aprs send failed: %v", err)
	}

	waitForAPRSMessages(t, capture, 2)
	goMessages := capture.Messages()

	if len(pythonMessages) != len(goMessages) {
		t.Fatalf("aprs message count mismatch: python=%d go=%d", len(pythonMessages), len(goMessages))
	}

	for i := range pythonMessages {
		if pythonMessages[i] != goMessages[i] {
			t.Fatalf("aprs message mismatch at %d\npython=%q\ngo=%q", i, pythonMessages[i], goMessages[i])
		}
	}
}

func waitForAPRSMessages(t *testing.T, capture *testutil.APRSCapture, count int) {
	t.Helper()

	deadline := time.After(2 * time.Second)
	for {
		if len(capture.Messages()) >= count {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("aprs messages not captured (expected %d)", count)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
