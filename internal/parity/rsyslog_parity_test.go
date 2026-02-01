package parity

import (
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

type rsyslogSendResult struct {
	Success bool `json:"success"`
}

type normalizedSyslog struct {
	Priority int
	Message  string
}

func TestRSyslogParity(t *testing.T) {
	capture := testutil.StartUDPCapture(t)
	defer func() {
		_ = capture.Close()
	}()

	host, port, err := net.SplitHostPort(capture.Addr())
	if err != nil {
		t.Fatalf("udp host split failed: %v", err)
	}

	appriseRoot := testutil.AppriseSourceRoot(t)
	t.Setenv("PYTHONPATH", appriseRoot)

	script := filepath.Join(testutil.RepoRoot(t), "internal", "testutil", "scripts", "capture_rsyslog.py")
	baseURL := fmt.Sprintf("rsyslog://%s:%s", host, port)

	cases := []struct {
		name       string
		url        string
		body       string
		title      string
		notifyType notify.NotifyType
		pythonType string
		logPID     bool
	}{
		{
			name:       "default-logpid",
			url:        baseURL,
			body:       "apprise parity body",
			title:      "apprise parity title",
			notifyType: notify.NotifyWarning,
			pythonType: "warning",
			logPID:     true,
		},
		{
			name:       "facility-prefix-no-logpid",
			url:        baseURL + "/d?logpid=no",
			body:       "apprise parity body",
			title:      "",
			notifyType: notify.NotifyFailure,
			pythonType: "failure",
			logPID:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capture.Reset()

			stdout, stderr, err := testutil.RunPythonScript(t, script,
				"--url", tc.url,
				"--body", tc.body,
				"--title", tc.title,
				"--type", tc.pythonType,
			)
			if err != nil {
				t.Fatalf("python rsyslog send failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
			}

			var result rsyslogSendResult
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatalf("parse python rsyslog result failed: %v (stdout: %s)", err, strings.TrimSpace(stdout))
			}
			if !result.Success {
				t.Fatalf("python rsyslog send reported failure: %s", strings.TrimSpace(stdout))
			}

			if !capture.WaitForPackets(1, time.Second) {
				t.Fatalf("no rsyslog payload captured from python")
			}
			pythonPackets := capture.Packets()
			pythonPayload := pythonPackets[len(pythonPackets)-1].Data

			capture.Reset()

			parsed, err := notify.ParseURL(tc.url)
			if err != nil {
				t.Fatalf("parse rsyslog url: %v", err)
			}
			target, err := notify.NewRSyslogTarget(parsed)
			if err != nil {
				t.Fatalf("build rsyslog target: %v", err)
			}
			if err := target.Send(tc.body, tc.title, tc.notifyType); err != nil {
				t.Fatalf("go rsyslog send failed: %v", err)
			}

			if !capture.WaitForPackets(1, time.Second) {
				t.Fatalf("no rsyslog payload captured from go")
			}
			goPackets := capture.Packets()
			goPayload := goPackets[len(goPackets)-1].Data

			pythonNormalized := normalizeSyslogPayload(t, pythonPayload, tc.logPID)
			goNormalized := normalizeSyslogPayload(t, goPayload, tc.logPID)

			if pythonNormalized != goNormalized {
				pyJSON, _ := json.Marshal(pythonNormalized)
				goJSON, _ := json.Marshal(goNormalized)
				t.Fatalf("rsyslog parity mismatch\npython=%s\ngo=%s", pyJSON, goJSON)
			}
		})
	}
}

func normalizeSyslogPayload(t *testing.T, payload string, logPID bool) normalizedSyslog {
	t.Helper()

	payload = strings.TrimSpace(payload)
	if payload == "" || !strings.HasPrefix(payload, "<") {
		t.Fatalf("unexpected syslog payload: %q", payload)
	}

	endPriority := strings.Index(payload, ">- ")
	if endPriority == -1 {
		t.Fatalf("missing syslog prefix: %q", payload)
	}

	priorityStr := payload[1:endPriority]
	priority, err := strconv.Atoi(priorityStr)
	if err != nil {
		t.Fatalf("invalid priority %q: %v", priorityStr, err)
	}

	message := payload[endPriority+3:]
	if logPID {
		if idx := strings.Index(message, " "); idx != -1 {
			message = message[idx+1:]
		} else {
			message = ""
		}
	}

	return normalizedSyslog{
		Priority: priority,
		Message:  message,
	}
}
