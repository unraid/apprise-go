package parity

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/unraid/apprise-go/internal/cli"
	"github.com/unraid/apprise-go/internal/testutil"
)

func TestGoRSyslogMatchesPythonCLI(t *testing.T) {
	capture := testutil.StartUDPCapture(t)
	defer func() {
		_ = capture.Close()
	}()

	host, port, err := net.SplitHostPort(capture.Addr())
	if err != nil {
		t.Fatalf("udp host split failed: %v", err)
	}

	baseURL := fmt.Sprintf("rsyslog://%s:%s", host, port)

	cases := []struct {
		name   string
		url    string
		body   string
		title  string
		ntype  string
		logPID bool
	}{
		{
			name:   "default-logpid",
			url:    baseURL,
			body:   "apprise parity body",
			title:  "apprise parity title",
			ntype:  "warning",
			logPID: true,
		},
		{
			name:   "facility-prefix-no-logpid",
			url:    baseURL + "/d?logpid=no",
			body:   "apprise parity body",
			title:  "",
			ntype:  "failure",
			logPID: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capture.Reset()
			sendPythonRSyslogCLI(t, tc.url, tc.body, tc.title, tc.ntype)
			if !capture.WaitForPackets(1, time.Second) {
				t.Fatalf("no rsyslog payload captured from python cli")
			}
			pythonPackets := capture.Packets()
			pythonPayload := pythonPackets[len(pythonPackets)-1].Data

			capture.Reset()
			sendGoRSyslogCLI(t, tc.url, tc.body, tc.title, tc.ntype)
			if !capture.WaitForPackets(1, time.Second) {
				t.Fatalf("no rsyslog payload captured from go cli")
			}
			goPackets := capture.Packets()
			goPayload := goPackets[len(goPackets)-1].Data

			pythonNormalized := normalizeSyslogPayload(t, pythonPayload, tc.logPID)
			goNormalized := normalizeSyslogPayload(t, goPayload, tc.logPID)

			if pythonNormalized != goNormalized {
				t.Fatalf("rsyslog cli parity mismatch\npython=%+v\ngo=%+v", pythonNormalized, goNormalized)
			}
		})
	}
}

func sendPythonRSyslogCLI(t *testing.T, url, body, title, notifyType string) {
	t.Helper()

	args := []string{
		"--body", body,
		"--disable-async",
		"--notification-type", notifyType,
	}
	if strings.TrimSpace(title) != "" {
		args = append(args, "--title", title)
	}
	args = append(args, url)

	stdout, stderr, err := testutil.RunApprise(t, args...)
	if err != nil {
		t.Fatalf("python apprise failed: %v (stdout: %s, stderr: %s)", err, strings.TrimSpace(stdout), strings.TrimSpace(stderr))
	}
}

func sendGoRSyslogCLI(t *testing.T, url, body, title, notifyType string) {
	t.Helper()

	args := []string{
		"--body", body,
		"--disable-async",
		"--notification-type", notifyType,
	}
	if strings.TrimSpace(title) != "" {
		args = append(args, "--title", title)
	}
	args = append(args, url)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := cli.Run(args, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("go apprise failed: code=%d stdout=%s stderr=%s", code, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}
}
