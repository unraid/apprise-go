//go:build !windows && !plan9 && !js && !wasip1

package parity

import (
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
)

func TestSyslogParity(t *testing.T) {
	url := "syslog://user?logpid=yes&logperror=no"
	body := "apprise parity body"
	title := "apprise parity title"

	pythonSuccess, pythonErr := runPythonNotify(t, url, body, title, "warning", "")

	parsed, err := notify.ParseURL(url)
	if err != nil {
		t.Fatalf("parse syslog url: %v", err)
	}
	target, err := notify.NewSyslogTarget(parsed)
	if err != nil {
		t.Fatalf("build syslog target: %v", err)
	}
	goErr := target.Send(body, title, notify.NotifyWarning)

	goSuccess := goErr == nil
	if pythonSuccess != goSuccess {
		t.Fatalf("syslog parity mismatch: python=%v go=%v (pythonErr=%s goErr=%v)", pythonSuccess, goSuccess, pythonErr, goErr)
	}

	if goErr != nil && pythonSuccess {
		t.Fatalf("go syslog send failed: %s", strings.TrimSpace(goErr.Error()))
	}
}
