package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

func TestSchemaMatchesPython(t *testing.T) {
	script := filepath.Join(testutil.RepoRoot(t), "internal", "testutil", "scripts", "schema_details.py")
	pyOut, pyErr, err := testutil.RunPythonScript(t, script)
	if err != nil {
		t.Fatalf("python schema failed: %v (stdout: %s, stderr: %s)", err, strings.TrimSpace(pyOut), strings.TrimSpace(pyErr))
	}

	var want any
	if err := json.Unmarshal([]byte(pyOut), &want); err != nil {
		t.Fatalf("decode python schema: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := Run([]string{"--schema"}, &stdout, &stderr); code != 0 {
		t.Fatalf("go schema failed: code=%d stdout=%s stderr=%s", code, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}

	var got any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode go schema: %v", err)
	}

	// The port declares some schemas as known gaps and sends no attachments
	// anywhere, so those two differences are deliberate rather than drift.
	want = normalizeSchemaForComparison(want)
	got = normalizeSchemaForComparison(got)

	if !reflect.DeepEqual(want, got) {
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		t.Fatalf("schema mismatch:\nwant:\n%s\n\ngot:\n%s", wantJSON, gotJSON)
	}
}

// normalizeSchemaForComparison drops the entries for schemas this port does
// not implement, and the attachment_support flag, which is false throughout
// because no attachments are sent. Both are recorded decisions; comparing
// them would keep this test red and hide real drift behind the noise.
func normalizeSchemaForComparison(value any) any {
	root, ok := value.(map[string]any)
	if !ok {
		return value
	}

	entries, ok := root["schemas"].([]any)
	if !ok {
		return value
	}

	// Keyed by schema rather than compared as a list: the two sides order
	// their entries differently, and that ordering carries no meaning.
	kept := map[string]any{}
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if schemaEntryIsKnownGap(entry) {
			continue
		}

		clone := map[string]any{}
		for key, inner := range entry {
			if key == "attachment_support" {
				continue
			}
			clone[key] = inner
		}
		kept[schemaEntryKey(entry)] = clone
	}

	out := map[string]any{}
	for key, inner := range root {
		out[key] = inner
	}
	out["schemas"] = kept

	return out
}

// schemaEntryKey names an entry by its first declared protocol, which is
// stable across both sides where list position is not.
func schemaEntryKey(entry map[string]any) string {
	names := []string{}
	for _, key := range []string{"protocols", "secure_protocols"} {
		values, ok := entry[key].([]any)
		if !ok {
			continue
		}
		for _, raw := range values {
			if schema, ok := raw.(string); ok {
				names = append(names, schema)
			}
		}
	}
	sort.Strings(names)

	return strings.Join(names, ",")
}

func schemaEntryIsKnownGap(entry map[string]any) bool {
	for _, key := range []string{"protocols", "secure_protocols"} {
		values, ok := entry[key].([]any)
		if !ok {
			continue
		}
		for _, raw := range values {
			if schema, ok := raw.(string); ok && notify.IsKnownGapSchema(schema) {
				return true
			}
		}
	}

	return false
}
