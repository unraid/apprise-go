package parity

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

func TestSchemaCoverage(t *testing.T) {
	appriseRoot := testutil.AppriseSourceRoot(t)
	script := filepath.Join(testutil.RepoRoot(t), "internal", "testutil", "scripts", "list_schemas.py")

	stdout, stderr, err := testutil.RunPythonScript(t, script, appriseRoot)
	if err != nil {
		t.Fatalf("list schemas failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	var pythonSchemas []string
	if err := json.Unmarshal([]byte(stdout), &pythonSchemas); err != nil {
		t.Fatalf("parse schemas: %v (output: %s)", err, strings.TrimSpace(stdout))
	}

	pythonSet := map[string]struct{}{}
	for _, schema := range pythonSchemas {
		normalized := strings.ToLower(schema)
		if isIgnoredSchema(normalized) {
			continue
		}
		pythonSet[normalized] = struct{}{}
	}

	goSchemas := notify.SupportedSchemas()
	goSet := map[string]struct{}{}
	for _, schema := range goSchemas {
		goSet[strings.ToLower(schema)] = struct{}{}
	}

	missing := []string{}
	for schema := range pythonSet {
		if _, ok := goSet[schema]; !ok {
			missing = append(missing, schema)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("missing schemas in go (%d): %s", len(missing), strings.Join(missing, ", "))
	}
}

func isIgnoredSchema(schema string) bool {
	_, ok := ignoredSchemas[schema]
	return ok
}

// Non-HTTP providers are excluded from schema coverage for the initial release.
// Keep in sync with PROCESS.md.
var ignoredSchemas = map[string]struct{}{}
