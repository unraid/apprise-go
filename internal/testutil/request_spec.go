package testutil

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
)

type pythonCapturePayload struct {
	Requests []notify.RequestSpec `json:"requests"`
	Success  *bool                `json:"success"`
}

func CapturePythonRequests(t *testing.T, url, body, title string) []notify.RequestSpec {
	t.Helper()

	specs, _ := CapturePythonRequestsResult(t, url, body, title)
	return specs
}

func CapturePythonRequestsWithType(t *testing.T, url, body, title string, notifyType notify.NotifyType) []notify.RequestSpec {
	t.Helper()

	specs, _ := CapturePythonRequestsWithTypeResult(t, url, body, title, notifyType)
	return specs
}

func CapturePythonRequestsWithFormat(t *testing.T, url, body, title, bodyFormat string) []notify.RequestSpec {
	t.Helper()

	specs, _ := CapturePythonRequestsWithFormatAndTypeResult(t, url, body, title, bodyFormat, notify.NotifyInfo)
	return specs
}

func CapturePythonRequestsWithFormatAndTypeResult(t *testing.T, url, body, title, bodyFormat string, notifyType notify.NotifyType) ([]notify.RequestSpec, *bool) {
	t.Helper()

	script := filepath.Join(RepoRoot(t), "internal", "testutil", "scripts", "capture_request.py")
	stdout, stderr, err := RunPythonScript(
		t,
		script,
		"--url", url,
		"--body", body,
		"--title", title,
		"--type", string(notifyType),
		"--body-format", bodyFormat,
	)
	if err != nil {
		t.Fatalf("capture request failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	payload := parsePythonCapturePayload(t, stdout)
	return payload.Requests, payload.Success
}

func CapturePythonRequestsResult(t *testing.T, url, body, title string) ([]notify.RequestSpec, *bool) {
	t.Helper()

	script := filepath.Join(RepoRoot(t), "internal", "testutil", "scripts", "capture_request.py")
	stdout, stderr, err := RunPythonScript(t, script, "--url", url, "--body", body, "--title", title)
	if err != nil {
		t.Fatalf("capture request failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	payload := parsePythonCapturePayload(t, stdout)
	return payload.Requests, payload.Success
}

func CapturePythonRequestsWithTypeResult(t *testing.T, url, body, title string, notifyType notify.NotifyType, attachments ...string) ([]notify.RequestSpec, *bool) {
	t.Helper()

	args := []string{
		"--url", url,
		"--body", body,
		"--title", title,
		"--type", string(notifyType),
	}
	for _, attachment := range attachments {
		args = append(args, "--attach", attachment)
	}

	script := filepath.Join(RepoRoot(t), "internal", "testutil", "scripts", "capture_request.py")
	stdout, stderr, err := RunPythonScript(t, script, args...)
	if err != nil {
		t.Fatalf("capture request failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	payload := parsePythonCapturePayload(t, stdout)
	return payload.Requests, payload.Success
}

func parsePythonCapturePayload(t *testing.T, stdout string) pythonCapturePayload {
	t.Helper()

	var payload pythonCapturePayload
	if err := json.Unmarshal([]byte(stdout), &payload); err == nil && (payload.Requests != nil || payload.Success != nil) {
		return payload
	}

	var specs []notify.RequestSpec
	if err := json.Unmarshal([]byte(stdout), &specs); err != nil {
		t.Fatalf("parse request specs: %v (output: %s)", err, strings.TrimSpace(stdout))
	}

	payload.Requests = specs
	return payload
}

// CapturePythonRequestsWithAttachments captures what upstream sends for a
// notification carrying attachments, so the Go side can be diffed against it
// rather than checked against a hand-written expectation.
func CapturePythonRequestsWithAttachments(
	t *testing.T,
	url, body, title string,
	notifyType notify.NotifyType,
	attachments []string,
) []notify.RequestSpec {
	t.Helper()

	args := []string{
		"--url", url,
		"--body", body,
		"--title", title,
		"--type", string(notifyType),
	}
	for _, attachment := range attachments {
		args = append(args, "--attach", attachment)
	}

	script := filepath.Join(RepoRoot(t), "internal", "testutil", "scripts", "capture_request.py")
	stdout, stderr, err := RunPythonScript(t, script, args...)
	if err != nil {
		t.Fatalf("capture request with attachments failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	return parsePythonCapturePayload(t, stdout).Requests
}
