package parity

import (
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/testutil"
	"github.com/unraid/apprise-go/internal/version"
)

func TestPythonVersionMatchesGo(t *testing.T) {
	python := testutil.PythonPath(t)

	stdout, stderr, err := testutil.RunCommand(t, python, "-c", "import apprise; print(apprise.__version__)")
	if err != nil {
		t.Fatalf("python apprise version failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	pyVersion := strings.TrimSpace(stdout)
	if pyVersion == "" {
		t.Fatalf("python apprise version empty (stderr: %s)", strings.TrimSpace(stderr))
	}

	if pyVersion != version.UpstreamVersion {
		t.Fatalf("version mismatch: python=%s upstream=%s", pyVersion, version.UpstreamVersion)
	}
}
