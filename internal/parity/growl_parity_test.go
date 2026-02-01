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

func TestGrowlParity(t *testing.T) {
	capture := testutil.StartGrowlCapture(t)
	defer func() {
		_ = capture.Close()
	}()

	host, port, err := net.SplitHostPort(capture.Addr())
	if err != nil {
		t.Fatalf("growl host split failed: %v", err)
	}

	url := fmt.Sprintf("growl://%s:%s?image=no&sticky=no&priority=normal&version=2", host, port)
	body := "apprise parity body"
	title := "apprise parity title"

	capture.Reset()
	pythonSuccess, pythonErr := runPythonNotify(t, url, body, title, "info", "")
	if !pythonSuccess {
		t.Fatalf("python growl send reported failure: %s", pythonErr)
	}
	pythonMessages := waitForGrowlMessages(t, capture, 2)

	capture.Reset()
	parsed, err := notify.ParseURL(url)
	if err != nil {
		t.Fatalf("parse growl url: %v", err)
	}
	target, err := notify.NewGrowlTarget(parsed)
	if err != nil {
		t.Fatalf("build growl target: %v", err)
	}
	if err := target.Send(body, title, notify.NotifyInfo); err != nil {
		t.Fatalf("go growl send failed: %v", err)
	}
	goMessages := waitForGrowlMessages(t, capture, 2)

	assertGrowlMessagesMatch(t, pythonMessages, goMessages)
}

func waitForGrowlMessages(t *testing.T, capture *testutil.GrowlCapture, count int) []testutil.GrowlMessage {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		messages := capture.Messages()
		if len(messages) >= count {
			return messages
		}
		if time.Now().After(deadline) {
			t.Fatalf("growl messages not captured (expected %d)", count)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func assertGrowlMessagesMatch(t *testing.T, pythonMessages, goMessages []testutil.GrowlMessage) {
	t.Helper()

	if len(pythonMessages) != len(goMessages) {
		t.Fatalf("growl message count mismatch: python=%d go=%d", len(pythonMessages), len(goMessages))
	}

	for i := range pythonMessages {
		py := pythonMessages[i]
		goMsg := goMessages[i]
		if strings.ToUpper(py.Type) != strings.ToUpper(goMsg.Type) {
			t.Fatalf("growl type mismatch: python=%s go=%s", py.Type, goMsg.Type)
		}

		for _, key := range []string{
			"Application-Name",
			"Notification-Name",
			"Notification-Title",
			"Notification-Text",
			"Notification-Priority",
			"Notification-Sticky",
		} {
			pyVal := py.Headers[key]
			goVal := goMsg.Headers[key]
			if strings.TrimSpace(pyVal) != strings.TrimSpace(goVal) {
				t.Fatalf("growl header mismatch %s: python=%q go=%q", key, pyVal, goVal)
			}
		}
	}
}
