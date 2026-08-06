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

type probedChoiceArg struct {
	Schema         string   `json:"schema"`
	Arg            string   `json:"arg"`
	Values         []string `json:"values"`
	RejectsInvalid bool     `json:"rejects_invalid"`
	Aliases        []string `json:"aliases"`
}

// TestChoiceArgTableCurrent re-probes upstream and fails if the embedded table
// no longer matches.
//
// The table decides which bad argument values are rejected. If upstream adds a
// choice argument, or changes one from silently-defaulting to rejecting, a
// stale table means the port keeps accepting a value upstream now refuses --
// and nothing else in the suite would notice, because every fixture uses valid
// values. This is the thing that enumerates the set.
func TestChoiceArgTableCurrent(t *testing.T) {
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

	script := filepath.Join(repoRoot, "internal", "testutil", "scripts", "choice_probe.py")
	stdout, stderr, err := testutil.RunPythonScript(t, script,
		"--apprise-root", appriseRoot,
		"--cases-root", filepath.Join(repoRoot, "internal", "parity", "providers"),
		"--schema-json", schemaPath,
	)
	if err != nil {
		t.Fatalf("choice probe failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	var probed []probedChoiceArg
	if err := json.Unmarshal([]byte(stdout), &probed); err != nil {
		t.Fatalf("decode choice probe: %v", err)
	}
	if len(probed) == 0 {
		t.Fatalf("choice probe produced nothing; the probe or its base URLs are broken")
	}

	// Only the rejecting arguments are enforced, so only they are recorded.
	fresh := map[string]map[string]bool{}
	for _, p := range probed {
		if !p.RejectsInvalid {
			continue
		}
		if fresh[p.Schema] == nil {
			fresh[p.Schema] = map[string]bool{}
		}
		fresh[p.Schema][p.Arg] = true
	}

	table := notify.ChoiceArgRules()

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
		t.Errorf("upstream now rejects bad values for these arguments but the table does not enforce them; "+
			"regenerate internal/notify/data/choice_args.json: %v", missing)
	}
	if len(extra) > 0 {
		t.Errorf("the table enforces these arguments but upstream no longer rejects bad values for them; "+
			"regenerate internal/notify/data/choice_args.json: %v", extra)
	}
}
