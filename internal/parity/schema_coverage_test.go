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

func TestSchemaCoverage(t *testing.T) {
	appriseRoot := testutil.AppriseSourceRoot(t)
	script := filepath.Join(testutil.RepoRoot(t), "internal", "testutil", "scripts", "list_schemas.py")

	stdout, stderr, err := testutil.RunPythonScript(t, script, appriseRoot)
	if err != nil {
		t.Fatalf("list schemas failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	var pythonSchemas []string
	if err := json.Unmarshal([]byte(stdout), &pythonSchemas); err != nil {
		t.Fatalf("parse schemas: %v (output: %s)", err, strings.TrimSpace(stdout))
	}

	pythonSet := map[string]struct{}{}
	for _, schema := range pythonSchemas {
		normalized := strings.ToLower(schema)
		if isIgnoredSchema(normalized) {
			continue
		}
		pythonSet[normalized] = struct{}{}
	}

	goSchemas := notify.SupportedSchemas()
	goSet := map[string]struct{}{}
	for _, schema := range goSchemas {
		goSet[strings.ToLower(schema)] = struct{}{}
	}

	missing := []string{}
	for schema := range pythonSet {
		if _, ok := goSet[schema]; !ok {
			missing = append(missing, schema)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("missing schemas in go (%d): %s", len(missing), strings.Join(missing, ", "))
	}
}

func isIgnoredSchema(schema string) bool {
	_, ok := ignoredSchemas[schema]
	return ok
}

// Non-HTTP providers are excluded from schema coverage for the initial release.
// Keep in sync with PROCESS.md.
// ignoredSchemas are the upstream schemas this port deliberately does not
// implement. Listing one here is a decision, not a shortcut — the entry says
// the gap is known and accepted, so an unlisted schema stays a test failure.
//
// blink1 drives a USB HID device attached to the machine running Apprise.
// Supporting it means cgo and a HID library, which costs the pure-Go static
// build; it is also the least likely of anything here to be reached through a
// Go port running in a container.
//
// irc and ircs need no dependency — upstream implements the protocol itself —
// but they need a stateful client (registration, nick collision, PING/PONG,
// JOIN confirmation, NickServ) and a fake IRC server for parity. That is
// scoped in .codex/upstream-1.12.0-next-steps.md and simply has not been
// written yet.
var ignoredSchemas = map[string]struct{}{
	"blink1": {},
	"irc":    {},
	"ircs":   {},
}
