package parity

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/testutil"
)

// TestFrameworkArgsAreClassified checks that every argument upstream's base
// class defines has been looked at, and that anything claimed as covered
// really is.
//
// It exists because "all fixtures pass" was taken as evidence the port was
// faithful, when the fixtures only ever used default arguments and short
// bodies. A knob nothing exercises is invisible to a request diff no matter
// how thorough the diff is.
func TestFrameworkArgsAreClassified(t *testing.T) {
	upstream := loadUpstreamFrameworkArgs(t)

	var unclassified []string
	for name := range upstream {
		if _, ok := FrameworkArgs[name]; !ok {
			unclassified = append(unclassified, name)
		}
	}
	if len(unclassified) > 0 {
		sort.Strings(unclassified)
		t.Errorf("upstream's base class defines %d argument(s) this port has "+
			"not classified: %s\n"+
			"Each one is inherited by every provider. Add it to FrameworkArgs "+
			"saying whether the port implements it, and if not, why not.",
			len(unclassified), strings.Join(unclassified, ", "))
	}

	var stale []string
	for name := range FrameworkArgs {
		if _, ok := upstream[name]; !ok {
			stale = append(stale, name)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("FrameworkArgs describes %d argument(s) upstream no longer "+
			"defines: %s\nRemove them, or the list stops describing anything.",
			len(stale), strings.Join(stale, ", "))
	}

	for name, arg := range FrameworkArgs {
		if (!arg.Implemented || !arg.FixtureCovered) && strings.TrimSpace(arg.Note) == "" {
			t.Errorf("%s is not both implemented and fixture-covered but "+
				"carries no note; a gap has to say what it is", name)
		}
	}
}

// TestFrameworkArgFixturesExist checks the fixtureCovered claims against the
// fixtures, so the table cannot drift into saying something is covered when
// the case that covered it has been renamed or deleted.
func TestFrameworkArgFixturesExist(t *testing.T) {
	defs := loadProviderDefinitions(t)

	used := map[string]int{}
	for name := range defs {
		for _, c := range defs[name].Cases {
			for arg := range FrameworkArgs {
				if strings.Contains(c.URL, arg+"=") {
					used[arg]++
				}
			}
		}
	}

	for arg, spec := range FrameworkArgs {
		if !spec.FixtureCovered {
			continue
		}
		if used[arg] == 0 {
			t.Errorf("%s is marked fixture-covered but no provider case sets "+
				"it; either add a case or correct the claim", arg)
		}
	}
}

func loadUpstreamFrameworkArgs(t *testing.T) map[string]any {
	t.Helper()

	t.Setenv("PYTHONPATH", testutil.AppriseSourceRoot(t))
	script := filepath.Join(testutil.RepoRoot(t),
		"internal", "testutil", "scripts", "framework_args.py")

	stdout, stderr, err := testutil.RunPythonScript(t, script)
	if err != nil {
		t.Fatalf("list upstream framework args: %v (stderr: %s)",
			err, strings.TrimSpace(stderr))
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(stdout), &args); err != nil {
		t.Fatalf("decode framework args: %v (stdout: %s)", err, stdout)
	}
	if _, failed := args["error"]; failed {
		t.Fatalf("upstream framework args failed: %s", stdout)
	}

	return args
}

// TestKnownDivergencesAreDeclared checks that every case excused from
// comparison says why, and reports the whole list so it is visible rather than
// buried in one provider's fixtures.
//
// A case marked this way still runs and still refreshes its golden — what it
// skips is the assertion. That is the alternative to deleting a red fixture,
// which removes the evidence of the gap along with the failure.
func TestKnownDivergencesAreDeclared(t *testing.T) {
	defs := loadProviderDefinitions(t)

	declared := []string{}
	for name := range defs {
		for _, c := range defs[name].Cases {
			if c.KnownDivergence == "" {
				continue
			}
			if len(strings.TrimSpace(c.KnownDivergence)) < 40 {
				t.Errorf("%s/%s is excused from comparison without saying "+
					"enough about why", name, c.Name)
			}
			declared = append(declared, name+"/"+c.Name)
		}
	}

	sort.Strings(declared)
	if len(declared) > 0 {
		t.Logf("%d case(s) are not compared against upstream:\n  %s",
			len(declared), strings.Join(declared, "\n  "))
	}

	// One entry per plugin that takes over its own splitting. The list is
	// expected to shrink as those are ported; a new entry should be a
	// deliberate decision rather than something that accumulates quietly.
	// One: a case where upstream raises rather than sends. A difference in
	// what this port sends is never an acceptable reason to add another.
	const allowed = 1
	if len(declared) > allowed {
		t.Errorf("%d cases are excused from comparison, up from %d; each one "+
			"is a difference in what this port sends", len(declared), allowed)
	}
}
