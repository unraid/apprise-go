package parity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/unraid/apprise-go/internal/testutil"
)

// urlVectorGap is a URL where this port and upstream still disagree about
// whether the URL is valid.
//
// This is a baseline of known gaps, not a list of approved exceptions. Most
// entries are simply not fixed yet, and calling them deliberate would be a lie
// that removes the pressure to fix them. What the baseline buys is a ratchet:
// a disagreement not listed here fails the build, and an entry that has stopped
// disagreeing also fails the build so it has to be deleted. The number can go
// down and cannot go up without someone editing this file.
//
// Direction matters more than the count. "go rejects, upstream accepts" is the
// damaging one -- a working configuration that stops working on the port. "go
// accepts, upstream rejects" means the port is more permissive, which lets a
// misconfigured URL through but never breaks a good one.
type urlVectorGap struct {
	URL      string `json:"url"`
	Schema   string `json:"schema"`
	Go       string `json:"go"`
	Upstream string `json:"upstream"`
	// Note carries the reason when a gap is genuinely intended rather than
	// outstanding. Most entries have none.
	Note string `json:"note,omitempty"`
}

func urlVectorGapsPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(testutil.RepoRoot(t), "internal", "parity", "fixtures", "url_vector_known_gaps.json")
}

func loadURLVectorAllowlist(t *testing.T) map[string]string {
	t.Helper()

	path := urlVectorGapsPath(t)
	raw, err := os.ReadFile(path) // #nosec G304 -- fixture path built from the repo root
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}
		}
		t.Fatalf("read url vector known gaps: %v", err)
	}

	var entries []urlVectorGap
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("decode url vector known gaps: %v", err)
	}

	allowed := make(map[string]string, len(entries))
	for _, e := range entries {
		allowed[e.URL] = e.Go
	}
	return allowed
}
