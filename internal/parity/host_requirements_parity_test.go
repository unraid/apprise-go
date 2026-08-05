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

// TestHostRequirementTableCurrent re-probes upstream and fails if the embedded
// host requirement table no longer matches.
//
// The same guard as the choice, int and credential tables. This one decides
// whether a URL with no host at all, or a host that is not a hostname, is an
// error -- and every fixture uses a well formed URL, so nothing else would
// notice the table going stale.
func TestHostRequirementTableCurrent(t *testing.T) {
	appriseRoot := testutil.AppriseSourceRoot(t)
	repoRoot := testutil.RepoRoot(t)

	script := filepath.Join(repoRoot, "internal", "testutil", "scripts", "host_probe.py")
	stdout, stderr, err := testutil.RunPythonScript(t, script,
		"--apprise-root", appriseRoot,
		"--cases-root", filepath.Join(repoRoot, "internal", "parity", "providers"),
	)
	if err != nil {
		t.Fatalf("host probe failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	var probed map[string]struct {
		RejectsEmpty   bool `json:"rejects_empty"`
		RejectsInvalid bool `json:"rejects_invalid"`
	}
	if err := json.Unmarshal([]byte(stdout), &probed); err != nil {
		t.Fatalf("decode host probe: %v", err)
	}
	if len(probed) == 0 {
		t.Fatalf("host probe produced nothing; the probe or its base URLs are broken")
	}

	table := notify.HostRequirements()

	var problems []string
	for schema, want := range probed {
		got, ok := table[schema]
		if !ok {
			problems = append(problems, schema+": missing from the table")
			continue
		}
		if got.RejectsEmpty != want.RejectsEmpty {
			problems = append(problems, schema+": rejects_empty differs")
		}
		if got.RejectsInvalid != want.RejectsInvalid {
			problems = append(problems, schema+": rejects_invalid differs")
		}
	}
	for schema := range table {
		if _, ok := probed[schema]; !ok {
			problems = append(problems, schema+": in the table but upstream no longer requires it")
		}
	}
	sort.Strings(problems)

	if len(problems) > 0 {
		t.Errorf("host requirement table is out of date; regenerate "+
			"internal/notify/data/host_requirements.json:\n  %s",
			strings.Join(problems, "\n  "))
	}
}
