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

type probedArgRule struct {
	Schema string `json:"schema"`
	Arg    string `json:"arg"`
	Kind   string `json:"kind"`
}

// TestArgRuleTableCurrent re-probes upstream and fails if the embedded table of
// bounded-float and regex-guarded arguments no longer matches.
//
// The same guard as the choice, integer, credential and host tables: every
// fixture uses valid values, so a table going stale is invisible to everything
// else in the suite.
func TestArgRuleTableCurrent(t *testing.T) {
	appriseRoot := testutil.AppriseSourceRoot(t)
	repoRoot := testutil.RepoRoot(t)

	script := filepath.Join(repoRoot, "internal", "testutil", "scripts", "arg_probe.py")
	stdout, stderr, err := testutil.RunPythonScript(t, script,
		"--apprise-root", appriseRoot,
		"--cases-root", filepath.Join(repoRoot, "internal", "parity", "providers"),
	)
	if err != nil {
		t.Fatalf("arg probe failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	var probed []probedArgRule
	if err := json.Unmarshal([]byte(stdout), &probed); err != nil {
		t.Fatalf("decode arg probe: %v", err)
	}
	if len(probed) == 0 {
		t.Fatalf("arg probe produced nothing; the probe or its base URLs are broken")
	}

	table := notify.ArgRules()

	fresh := map[string]string{}
	for _, p := range probed {
		fresh[p.Schema+"."+p.Arg] = p.Kind
	}

	var problems []string
	for key, kind := range fresh {
		parts := strings.SplitN(key, ".", 2)
		rule, ok := table[parts[0]][parts[1]]
		if !ok {
			problems = append(problems, key+": upstream enforces it, the table does not")
			continue
		}
		if rule.Kind != kind {
			problems = append(problems, key+": kind differs")
		}
	}
	for schema, rules := range table {
		for arg := range rules {
			if _, ok := fresh[schema+"."+arg]; !ok {
				problems = append(problems, schema+"."+arg+": in the table but upstream no longer enforces it")
			}
		}
	}
	sort.Strings(problems)

	if len(problems) > 0 {
		t.Errorf("arg rule table is out of date; regenerate "+
			"internal/notify/data/arg_rules.json:\n  %s", strings.Join(problems, "\n  "))
	}
}
