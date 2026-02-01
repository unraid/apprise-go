package parity

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

func TestMQTTParity(t *testing.T) {
	cases := []struct {
		name        string
		schema      string
		captureFunc func(*testing.T) *testutil.MQTTCapture
		verifyParam string
		caPath      func(*testing.T) string
	}{
		{"mqtt", "mqtt", testutil.StartMQTTCapture, "", nil},
		{"mqtts", "mqtts", testutil.StartMQTTSCapture, "no", testutil.TestTLSCACertPath},
		{"mqtts-verify", "mqtts", testutil.StartMQTTSCapture, "yes", testutil.TestTLSCACertPath},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			capture := tc.captureFunc(t)
			defer func() {
				_ = capture.Close()
			}()

			host, port, err := net.SplitHostPort(capture.Addr())
			if err != nil {
				t.Fatalf("mqtt host split failed: %v", err)
			}

			verify := ""
			if tc.verifyParam != "" {
				verify = "&verify=" + tc.verifyParam
			}
			url := fmt.Sprintf("%s://user:pass@%s:%s/topic?client_id=apprise-go&version=v3.1.1&qos=0&retain=no&session=no%s", tc.schema, host, port, verify)
			body := "apprise parity body"

			capture.Reset()
			caPath := ""
			if tc.caPath != nil {
				caPath = tc.caPath(t)
			}
			pythonSuccess, pythonErr := runPythonNotify(t, url, body, "", "info", caPath)
			if !pythonSuccess {
				t.Fatalf("python mqtt send reported failure: %s", pythonErr)
			}
			if !capture.WaitForMessages(1, time.Second) {
				t.Fatalf("no mqtt message captured from python")
			}
			pythonMessages := capture.Messages()

			capture.Reset()
			if strings.TrimSpace(caPath) != "" {
				t.Setenv("SSL_CERT_FILE", caPath)
			}

			parsed, err := notify.ParseURL(url)
			if err != nil {
				t.Fatalf("parse mqtt url: %v", err)
			}
			target, err := notify.NewMQTTTarget(parsed)
			if err != nil {
				t.Fatalf("build mqtt target: %v", err)
			}
			if err := target.Send(body, "", notify.NotifyInfo); err != nil {
				t.Fatalf("go mqtt send failed: %v", err)
			}
			if !capture.WaitForMessages(1, time.Second) {
				t.Fatalf("no mqtt message captured from go")
			}
			goMessages := capture.Messages()

			if len(pythonMessages) != len(goMessages) {
				t.Fatalf("mqtt message count mismatch: python=%d go=%d", len(pythonMessages), len(goMessages))
			}

			for i := range pythonMessages {
				py := pythonMessages[i]
				goMsg := goMessages[i]
				if py.Topic != goMsg.Topic {
					t.Fatalf("mqtt topic mismatch: python=%s go=%s", py.Topic, goMsg.Topic)
				}
				if strings.TrimSpace(py.Payload) != strings.TrimSpace(goMsg.Payload) {
					t.Fatalf("mqtt payload mismatch: python=%q go=%q", py.Payload, goMsg.Payload)
				}
				if py.QoS != goMsg.QoS || py.Retain != goMsg.Retain {
					t.Fatalf("mqtt flags mismatch: python=%v/%v go=%v/%v", py.QoS, py.Retain, goMsg.QoS, goMsg.Retain)
				}
			}
		})
	}
}
