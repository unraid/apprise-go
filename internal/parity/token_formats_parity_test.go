package parity

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

type extractedTokenFormat struct {
	Schema  string `json:"schema"`
	Field   string `json:"field"`
	Token   string `json:"token"`
	Pattern string `json:"pattern"`
}

// TestTokenFormatTableCurrent re-extracts upstream's credential format checks
// and fails if the embedded table no longer matches.
//
// Without this the table is a snapshot that silently rots: upstream tightening
// an api key format, or adding a check to a plugin that had none, would leave
// the port accepting credentials upstream rejects, and every fixture would
// still pass because they all use valid values.
func TestTokenFormatTableCurrent(t *testing.T) {
	appriseRoot := testutil.AppriseSourceRoot(t)
	repoRoot := testutil.RepoRoot(t)

	script := filepath.Join(repoRoot, "internal", "testutil", "scripts", "token_regex.py")
	stdout, stderr, err := testutil.RunPythonScript(t, script,
		"--apprise-root", appriseRoot,
		"--cases-root", filepath.Join(repoRoot, "internal", "parity", "providers"),
	)
	if err != nil {
		t.Fatalf("token regex extraction failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	var extracted []extractedTokenFormat
	if err := json.Unmarshal([]byte(stdout), &extracted); err != nil {
		t.Fatalf("decode token regexes: %v", err)
	}
	if len(extracted) == 0 {
		t.Fatalf("no token regexes extracted; the extractor is broken")
	}

	table := notify.TokenFormatRules()

	fresh := map[string]string{}
	for _, e := range extracted {
		fresh[e.Schema+"."+e.Field] = e.Pattern
	}

	var missing, extra, changed []string
	for key, pattern := range fresh {
		parts := strings.SplitN(key, ".", 2)
		rule, ok := table[parts[0]][parts[1]]
		if !ok {
			missing = append(missing, key)
			continue
		}
		if rule.Pattern != pattern {
			changed = append(changed, key)
		}
	}
	for schema, fields := range table {
		for field := range fields {
			if _, ok := fresh[schema+"."+field]; !ok {
				extra = append(extra, schema+"."+field)
			}
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	sort.Strings(changed)

	if len(missing) > 0 {
		t.Errorf("upstream enforces a credential format for these that the table does not; "+
			"regenerate internal/notify/data/token_formats.json: %v", missing)
	}
	if len(extra) > 0 {
		t.Errorf("the table enforces these but upstream no longer does; "+
			"regenerate internal/notify/data/token_formats.json: %v", extra)
	}
	if len(changed) > 0 {
		t.Errorf("upstream changed the pattern for these; "+
			"regenerate internal/notify/data/token_formats.json: %v", changed)
	}

	// Every recorded pattern has to compile here, or the check silently does
	// nothing for that schema.
	for schema, fields := range table {
		for field, rule := range fields {
			pattern := rule.Pattern
			if rule.IgnoreCase {
				pattern = "(?i)" + pattern
			}
			if _, err := regexp.Compile(pattern); err != nil {
				t.Errorf("%s.%s pattern does not compile in Go, so it enforces nothing: %v",
					schema, field, err)
			}
		}
	}
}
