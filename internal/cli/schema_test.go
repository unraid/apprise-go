package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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

	if !reflect.DeepEqual(want, got) {
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		t.Fatalf("schema mismatch:\nwant:\n%s\n\ngot:\n%s", wantJSON, gotJSON)
	}
}
