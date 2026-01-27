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

type pythonSchemaDetails map[string]any

func loadPythonSchemaDetails(t *testing.T, schemas []string) map[string]pythonSchemaDetails {
	t.Helper()

	appriseRoot := testutil.AppriseSourceRoot(t)
	t.Setenv("PYTHONPATH", appriseRoot)

	script := filepath.Join(testutil.RepoRoot(t), "internal", "testutil", "scripts", "schema_metadata.py")
	stdout, stderr, err := testutil.RunPythonScript(t, script, schemas...)
	if err != nil {
		t.Fatalf("schema details failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	raw := map[string]pythonSchemaDetails{}
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("decode schema details: %v (stdout: %s)", err, strings.TrimSpace(stdout))
	}

	return raw
}

func normalizeSchemaDetails(value any, key string) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[k] = normalizeSchemaDetails(v, k)
		}
		return out
	case []string:
		normalized := make([]any, len(typed))
		for i, entry := range typed {
			normalized[i] = entry
		}
		return normalizeSchemaDetails(normalized, key)
	case []int:
		normalized := make([]any, len(typed))
		for i, entry := range typed {
			normalized[i] = entry
		}
		return normalizeSchemaDetails(normalized, key)
	case []float64:
		normalized := make([]any, len(typed))
		for i, entry := range typed {
			normalized[i] = entry
		}
		return normalizeSchemaDetails(normalized, key)
	case []any:
		normalized := make([]any, len(typed))
		for i, entry := range typed {
			normalized[i] = normalizeSchemaDetails(entry, key)
		}
		if shouldSortSchemaList(key, normalized) {
			strs := make([]string, 0, len(normalized))
			for _, entry := range normalized {
				str, ok := entry.(string)
				if !ok {
					return normalized
				}
				strs = append(strs, str)
			}
			sort.Strings(strs)
			out := make([]any, len(strs))
			for i, str := range strs {
				out[i] = str
			}
			return out
		}
		return normalized
	default:
		return value
	}
}

func shouldSortSchemaList(key string, values []any) bool {
	switch strings.ToLower(key) {
	case "templates", "values", "group", "delim":
		return true
	default:
		return false
	}
}

func normalizeDetailsMap(details map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range details {
		out[key] = normalizeSchemaDetails(value, key)
	}
	return out
}

func TestSchemaMetadataParity(t *testing.T) {
	schemas := notify.SupportedSchemas()
	if len(schemas) == 0 {
		t.Fatalf("no supported schemas discovered")
	}

	pythonDetails := loadPythonSchemaDetails(t, schemas)

	for _, schema := range schemas {
		schema := strings.ToLower(schema)
		t.Run(schema, func(t *testing.T) {
			py, ok := pythonDetails[schema]
			if !ok {
				t.Fatalf("missing python schema details for %s", schema)
			}

			entry, ok := notify.SchemaEntryForSchema(schema)
			if !ok {
				t.Fatalf("missing go schema entry for %s", schema)
			}

			rawDetails, ok := entry["details"].(map[string]any)
			if !ok {
				t.Fatalf("missing go schema details for %s", schema)
			}

			goDetails := map[string]any{
				"args":      rawDetails["args"],
				"tokens":    rawDetails["tokens"],
				"kwargs":    rawDetails["kwargs"],
				"templates": rawDetails["templates"],
			}

			normalizedGo := normalizeDetailsMap(goDetails)
			normalizedPy := normalizeDetailsMap(map[string]any(py))

			pyJSON, _ := json.Marshal(normalizedPy)
			goJSON, _ := json.Marshal(normalizedGo)
			if string(pyJSON) != string(goJSON) {
				t.Fatalf("schema metadata mismatch\npython=%s\ngo=%s", pyJSON, goJSON)
			}
		})
	}
}
