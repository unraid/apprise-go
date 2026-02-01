package parity

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

type notifyResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

func TestLocalNotifyParity(t *testing.T) {
	cases := []string{
		"dbus",
		"glib",
		"gio",
		"gnome",
		"kde",
		"qt",
		"macosx",
		"windows",
	}

	for _, schema := range cases {
		schema := schema
		t.Run(schema, func(t *testing.T) {
			url := schema + "://"
			body := "apprise parity body"
			title := "apprise parity title"

			pythonSuccess, _ := runPythonNotify(t, url, body, title, "info", "")
			goSuccess := runGoNotify(t, url, body, title)

			if pythonSuccess != goSuccess {
				t.Fatalf("notify parity mismatch for %s: python=%v go=%v", schema, pythonSuccess, goSuccess)
			}
		})
	}
}

func runPythonNotify(t *testing.T, url, body, title, notifyType, caPath string) (bool, string) {
	t.Helper()

	appriseRoot := testutil.AppriseSourceRoot(t)
	t.Setenv("PYTHONPATH", appriseRoot)

	script := filepath.Join(testutil.RepoRoot(t), "internal", "testutil", "scripts", "capture_notify.py")
	args := []string{
		"--url", url,
		"--body", body,
		"--title", title,
		"--type", notifyType,
	}
	if strings.TrimSpace(caPath) != "" {
		args = append(args, "--ca", caPath)
	}
	stdout, stderr, err := testutil.RunPythonScript(t, script, args...)
	if err != nil {
		t.Fatalf("python notify failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	var result notifyResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse python notify result failed: %v (stdout: %s)", err, strings.TrimSpace(stdout))
	}
	return result.Success, strings.TrimSpace(result.Error)
}

func runGoNotify(t *testing.T, url, body, title string) bool {
	t.Helper()

	parsed, err := notify.ParseURL(url)
	if err != nil {
		t.Fatalf("parse url failed: %v", err)
	}

	var sendErr error
	switch parsed.Scheme {
	case "dbus", "glib", "gio", "gnome", "kde", "qt":
		target, err := notify.NewLocalNotifyTarget(parsed)
		if err != nil {
			return false
		}
		sendErr = target.Send(body, title, notify.NotifyInfo)
	case "macosx":
		target, err := notify.NewMacOSXTarget(parsed)
		if err != nil {
			return false
		}
		sendErr = target.Send(body, title, notify.NotifyInfo)
	case "windows":
		target, err := notify.NewWindowsTarget(parsed)
		if err != nil {
			return false
		}
		sendErr = target.Send(body, title, notify.NotifyInfo)
	default:
		t.Fatalf("unsupported schema: %s", parsed.Scheme)
	}

	return sendErr == nil
}
