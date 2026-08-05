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

// urlVector is one URL from upstream's own test suite, paired with what
// upstream's parser actually does with it.
type urlVector struct {
	Schema   string `json:"schema"`
	URL      string `json:"url"`
	Accepted bool   `json:"accepted"`
	Declared string `json:"declared"`
	Source   string `json:"source"`
}

func loadURLVectors(t *testing.T) []urlVector {
	t.Helper()

	appriseRoot := testutil.AppriseSourceRoot(t)
	script := filepath.Join(testutil.RepoRoot(t), "internal", "testutil", "scripts", "url_vectors.py")
	stdout, stderr, err := testutil.RunPythonScript(t, script, "--apprise-root", appriseRoot)
	if err != nil {
		t.Fatalf("url vector extraction failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	var vectors []urlVector
	if err := json.Unmarshal([]byte(stdout), &vectors); err != nil {
		t.Fatalf("decode url vectors: %v", err)
	}
	return vectors
}

// acceptsURL reports whether this port would build a working target from the
// URL -- the same question upstream answers by returning a plugin instance or
// not from Apprise.instantiate().
func acceptsURL(raw string) bool {
	parsed, err := notify.ParseURL(raw)
	if err != nil {
		return false
	}
	if _, err := notify.NewTarget(parsed); err != nil {
		return false
	}
	return true
}

// TestURLVectorParity checks this port against every URL upstream's own test
// suite exercises, for the schemas this port implements.
//
// This is a deliberately different oracle from the golden fixtures. Those prove
// the bytes on the wire match for URLs written here -- which means they can only
// ever cover URL shapes someone here thought to write down. Upstream's vectors
// were written by the people who wrote the plugins, and they are dense with the
// malformed and the half-specified: empty hosts, missing tokens, bad ports,
// credentials with no user. Accepting a URL upstream rejects means this port
// silently sends something upstream never would; rejecting one upstream accepts
// means a working configuration breaks on the port.
func TestURLVectorParity(t *testing.T) {
	vectors := loadURLVectors(t)
	if len(vectors) == 0 {
		t.Fatalf("no url vectors extracted from upstream")
	}

	type disagreement struct {
		vector urlVector
		goSide bool
	}

	var (
		compared int
		diffs    []disagreement
		bySchema = map[string]int{}
	)

	for _, v := range vectors {
		if !notify.SupportsSchema(v.Schema) {
			continue
		}
		compared++
		got := acceptsURL(v.URL)
		if got != v.Accepted {
			diffs = append(diffs, disagreement{vector: v, goSide: got})
			bySchema[v.Schema]++
		}
	}

	if compared == 0 {
		t.Fatalf("no upstream url vectors matched a supported schema; extraction or schema list is broken")
	}
	t.Logf("compared %d upstream url vectors across supported schemas", compared)

	allowed := loadURLVectorAllowlist(t)
	var unexpected []disagreement
	for _, d := range diffs {
		if _, ok := allowed[d.vector.URL]; !ok {
			unexpected = append(unexpected, d)
		}
	}

	sort.Slice(unexpected, func(i, j int) bool {
		if unexpected[i].vector.Schema != unexpected[j].vector.Schema {
			return unexpected[i].vector.Schema < unexpected[j].vector.Schema
		}
		return unexpected[i].vector.URL < unexpected[j].vector.URL
	})

	for _, d := range unexpected {
		verb := "accepts"
		if !d.goSide {
			verb = "rejects"
		}
		upstreamVerb := "accepts"
		if !d.vector.Accepted {
			upstreamVerb = "rejects"
		}
		t.Errorf("url vector disagreement (%s): go %s, upstream %s\n  %s\n  declared in %s as %s",
			d.vector.Schema, verb, upstreamVerb, d.vector.URL, d.vector.Source, d.vector.Declared)
	}

	// An allowlist entry that no longer disagrees is stale: the underlying
	// difference was fixed and the entry now hides nothing. Fail so it gets
	// removed, otherwise the allowlist grows into a place where real
	// regressions can hide behind a URL that used to be a known problem.
	live := map[string]bool{}
	for _, d := range diffs {
		live[d.vector.URL] = true
	}
	for url := range allowed {
		if !live[url] {
			t.Errorf("stale url vector allowlist entry (now agrees with upstream, remove it): %s", url)
		}
	}
}
