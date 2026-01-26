package notify_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

func loadPythonSchemaDetails(t *testing.T) map[string]any {
	t.Helper()

	appriseRoot := testutil.AppriseSourceRoot(t)
	t.Setenv("PYTHONPATH", appriseRoot)

	script := filepath.Join(testutil.RepoRoot(t), "internal", "testutil", "scripts", "schema_details.py")
	stdout, stderr, err := testutil.RunPythonScript(t, script)
	if err != nil {
		t.Fatalf("python schema details failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode python schema details: %v (stdout: %s)", err, strings.TrimSpace(stdout))
	}

	return result
}

func loadGoSchemaDetails(t *testing.T) map[string]any {
	t.Helper()

	data, err := notify.SchemaJSON()
	if err != nil {
		t.Fatalf("go schema json: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("decode go schema json: %v", err)
	}

	return result
}

func normalizeSchemaDetails(t *testing.T, details map[string]any) map[string]any {
	t.Helper()

	out := map[string]any{}
	if value, ok := details["version"]; ok {
		out["version"] = value
	}
	if value, ok := details["asset"]; ok {
		out["asset"] = value
	}
	out["schemas"] = normalizeSchemaEntries(t, details["schemas"])
	return out
}

func normalizeSchemaEntries(t *testing.T, raw any) map[string]any {
	t.Helper()

	entries, ok := raw.([]any)
	if !ok {
		t.Fatalf("schema entries type mismatch: %T", raw)
	}

	out := map[string]any{}
	for _, item := range entries {
		entry, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("schema entry type mismatch: %T", item)
		}
		name, ok := entry["service_name"].(string)
		if !ok || name == "" {
			t.Fatalf("schema entry missing service_name")
		}
		out[name] = entry
	}

	return out
}

func TestSchemaDetailsParity(t *testing.T) {
	python := normalizeSchemaDetails(t, loadPythonSchemaDetails(t))
	goDetails := normalizeSchemaDetails(t, loadGoSchemaDetails(t))

	if !reflect.DeepEqual(python, goDetails) {
		pythonJSON, _ := json.MarshalIndent(python, "", "  ")
		goJSON, _ := json.MarshalIndent(goDetails, "", "  ")
		t.Fatalf("schema details mismatch\npython=%s\ngo=%s", pythonJSON, goJSON)
	}
}
