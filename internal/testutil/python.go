package testutil

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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

	cmd := exec.Command(name, args...)
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
