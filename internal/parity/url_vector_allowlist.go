package parity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/testutil"
)

// urlVectorAllowance records a URL where this port deliberately disagrees with
// upstream about whether the URL is valid, and why.
//
// Every entry is a liability: it is a place where the port and upstream behave
// differently and the test will not tell you. The reason field has to say what
// the difference is and why it is intentional, and TestURLVectorParity fails on
// any entry that has stopped disagreeing, so the list cannot quietly accumulate
// entries that no longer describe reality.
type urlVectorAllowance struct {
	URL    string `json:"url"`
	Reason string `json:"reason"`
}

func urlVectorAllowlistPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(testutil.RepoRoot(t), "internal", "parity", "fixtures", "url_vector_allowlist.json")
}

func loadURLVectorAllowlist(t *testing.T) map[string]string {
	t.Helper()

	path := urlVectorAllowlistPath(t)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}
		}
		t.Fatalf("read url vector allowlist: %v", err)
	}

	var entries []urlVectorAllowance
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("decode url vector allowlist: %v", err)
	}

	allowed := make(map[string]string, len(entries))
	for _, e := range entries {
		if strings.TrimSpace(e.Reason) == "" {
			t.Errorf("url vector allowlist entry has no reason: %s", e.URL)
			continue
		}
		allowed[e.URL] = e.Reason
	}
	return allowed
}
