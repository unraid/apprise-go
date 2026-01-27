package notify_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/testutil"
)

type schemaCase struct {
	Schema string `json:"schema"`
	URL    string `json:"url"`
}

func loadSchemaCases(t *testing.T) []schemaCase {
	t.Helper()

	appriseRoot := testutil.AppriseSourceRoot(t)
	t.Setenv("PYTHONPATH", appriseRoot)

	script := filepath.Join(testutil.RepoRoot(t), "internal", "testutil", "scripts", "schema_parity_cases.py")
	stdout, stderr, err := testutil.RunPythonScript(t, script)
	if err != nil {
		t.Fatalf("schema parity cases failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	var cases []schemaCase
	if err := json.Unmarshal([]byte(stdout), &cases); err != nil {
		t.Fatalf("decode schema cases: %v (stdout: %s)", err, strings.TrimSpace(stdout))
	}

	return cases
}

func normalizeValues(values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = normalizeValue(value)
	}
	return out
}

func normalizeValue(value any) any {
	switch typed := value.(type) {
	case []string:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = normalizeValue(item)
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = normalizeValue(item)
		}
		return out
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case float32:
		return float64(typed)
	default:
		return value
	}
}

func TestSchemaInputsParityAllSchemas(t *testing.T) {
	cases := loadSchemaCases(t)
	if len(cases) == 0 {
		t.Fatalf("no schema cases generated")
	}

	for _, entry := range cases {
		entry := entry
		t.Run(entry.Schema, func(t *testing.T) {
			python := loadPythonSchemaInputs(t, entry.Schema, entry.URL)

			inputs := loadGoSchemaInputs(t, entry.Schema, entry.URL)
			goValues := normalizeValues(inputs.ValuesMap())
			pyValues := normalizeValues(python.Values)

			if !reflect.DeepEqual(pyValues, goValues) {
				pyJSON, _ := json.Marshal(pyValues)
				goJSON, _ := json.Marshal(goValues)
				t.Fatalf("values mismatch url=%s\npython=%s\ngo=%s", entry.URL, pyJSON, goJSON)
			}

			if !reflect.DeepEqual(python.Kwargs, inputs.Kwargs) {
				pyJSON, _ := json.Marshal(python.Kwargs)
				goJSON, _ := json.Marshal(inputs.Kwargs)
				t.Fatalf("kwargs mismatch url=%s\npython=%s\ngo=%s", entry.URL, pyJSON, goJSON)
			}

			if !reflect.DeepEqual(python.Aliases, inputs.Aliases) {
				pyJSON, _ := json.Marshal(python.Aliases)
				goJSON, _ := json.Marshal(inputs.Aliases)
				t.Fatalf("aliases mismatch url=%s\npython=%s\ngo=%s", entry.URL, pyJSON, goJSON)
			}
		})
	}
}
