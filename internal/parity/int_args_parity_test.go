package parity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

type probedIntArg struct {
	Schema            string `json:"schema"`
	Arg               string `json:"arg"`
	RejectsNonNumeric bool   `json:"rejects_nonnumeric"`
	RejectsBelow      *bool  `json:"rejects_below"`
	RejectsAbove      *bool  `json:"rejects_above"`
}

// enforced reports whether upstream rejects anything at all for this argument.
func (p probedIntArg) enforced() bool {
	return p.RejectsNonNumeric ||
		(p.RejectsBelow != nil && *p.RejectsBelow) ||
		(p.RejectsAbove != nil && *p.RejectsAbove)
}

// TestIntArgTableCurrent re-probes upstream and fails if the embedded integer
// argument table no longer matches.
//
// The same guard as TestChoiceArgTableCurrent, for the same reason: the table
// decides which out-of-range values are rejected, every fixture uses valid
// ones, and nothing else in the suite would notice it going stale.
func TestIntArgTableCurrent(t *testing.T) {
	appriseRoot := testutil.AppriseSourceRoot(t)
	repoRoot := testutil.RepoRoot(t)

	schemaPath := filepath.Join(t.TempDir(), "schema.json")
	data, err := notify.SchemaJSON()
	if err != nil {
		t.Fatalf("schema json: %v", err)
	}
	if err := os.WriteFile(schemaPath, data, 0o600); err != nil {
		t.Fatalf("write schema json: %v", err)
	}

	script := filepath.Join(repoRoot, "internal", "testutil", "scripts", "int_probe.py")
	stdout, stderr, err := testutil.RunPythonScript(t, script,
		"--apprise-root", appriseRoot,
		"--cases-root", filepath.Join(repoRoot, "internal", "parity", "providers"),
		"--schema-json", schemaPath,
	)
	if err != nil {
		t.Fatalf("int probe failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	var probed []probedIntArg
	if err := json.Unmarshal([]byte(stdout), &probed); err != nil {
		t.Fatalf("decode int probe: %v", err)
	}
	if len(probed) == 0 {
		t.Fatalf("int probe produced nothing; the probe or its base URLs are broken")
	}

	fresh := map[string]map[string]bool{}
	for _, p := range probed {
		if !p.enforced() {
			continue
		}
		if fresh[p.Schema] == nil {
			fresh[p.Schema] = map[string]bool{}
		}
		fresh[p.Schema][p.Arg] = true
	}

	table := notify.IntArgRules()

	var missing, extra []string
	for schema, args := range fresh {
		for arg := range args {
			if _, ok := table[schema][arg]; !ok {
				missing = append(missing, schema+"."+arg)
			}
		}
	}
	for schema, args := range table {
		for arg := range args {
			if !fresh[schema][arg] {
				extra = append(extra, schema+"."+arg)
			}
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) > 0 {
		t.Errorf("upstream now rejects out-of-range values for these arguments but the table does not "+
			"enforce them; regenerate internal/notify/data/int_args.json: %v", missing)
	}
	if len(extra) > 0 {
		t.Errorf("the table enforces these arguments but upstream no longer rejects bad values for them; "+
			"regenerate internal/notify/data/int_args.json: %v", extra)
	}
}
