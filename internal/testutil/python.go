package testutil

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func PythonPath(t *testing.T) string {
	t.Helper()

	path := filepath.Join(VenvBinDir(t), "python")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("python not found in venv: %s", path)
	}

	return path
}

func RequirePythonApprise(t *testing.T) {
	t.Helper()

	path := filepath.Join(VenvBinDir(t), "python")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("python not found in venv: %s", path)
	}

	_, stderr, err := RunCommand(t, path, "-c", "import apprise")
	if err != nil {
		t.Fatalf("python apprise import failed: %v (stderr: %s)", err, stderr)
	}
}

func AppriseCLIPath(t *testing.T) string {
	t.Helper()

	path := filepath.Join(VenvBinDir(t), "apprise")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("apprise CLI not found in venv: %s", path)
	}

	return path
}

func RunCommand(t *testing.T, name string, args ...string) (string, string, error) {
	t.Helper()
	return runCommand(t.Context(), name, args...)
}

func runCommand(ctx context.Context, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = os.Environ()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func RunPythonScript(t *testing.T, script string, args ...string) (string, string, error) {
	t.Helper()

	python := PythonPath(t)
	cmdArgs := append([]string{script}, args...)
	if filepath.Base(script) == "capture_request.py" {
		// Bound each notification capture. Aggregate commands such as the
		// golden check use the test's context and can compare many providers.
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		return runCommand(ctx, python, cmdArgs...)
	}
	return RunCommand(t, python, cmdArgs...)
}

// RunApprise executes the apprise CLI via python -m to avoid shebang issues
// when the repo is moved.
func RunApprise(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	python := PythonPath(t)
	cmdArgs := append([]string{
		"-c",
		"import sys; from apprise.cli import main; sys.argv = ['apprise'] + sys.argv[1:]; main()",
	}, args...)
	return RunCommand(t, python, cmdArgs...)
}
