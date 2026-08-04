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

type upstreamLimits struct {
	BodyMaxLen  *int `json:"body_maxlen"`
	TitleMaxLen *int `json:"title_maxlen"`
	LineMax     *int `json:"body_max_line_count"`
	Amalgamate  bool `json:"overflow_amalgamate_title"`
	Buffer      *int `json:"overflow_buffer"`
}

// TestOverflowLimitsMatchUpstream checks the generated limits table against
// upstream, so a service whose limit changes upstream — or one added later —
// shows up rather than silently keeping a stale number.
//
// The limits are class attributes rather than URL arguments, so nothing else
// in the harness can see them: they appear in no schema entry, and a request
// fixture only exercises them when a body is long enough to overflow.
func TestOverflowLimitsMatchUpstream(t *testing.T) {
	upstream := loadUpstreamOverflowLimits(t)

	var missing, mismatched []string
	for schema, limits := range upstream {
		if limits.BodyMaxLen == nil && limits.TitleMaxLen == nil {
			// Computed per instance upstream, so there is no constant to
			// record; ApplyOverflow leaves these alone.
			continue
		}
		if notify.IsKnownGapSchema(schema) {
			continue
		}

		if !notify.OverflowSchemaKnown(schema) {
			missing = append(missing, schema)

			continue
		}

		got := notify.OverflowLimitsFor(schema)
		want := []int{
			derefOr(limits.BodyMaxLen, -1),
			derefOr(limits.TitleMaxLen, -1),
			derefOr(limits.LineMax, 0),
			derefOr(limits.Buffer, 0),
		}
		if got.BodyMax != want[0] || got.TitleMax != want[1] ||
			got.LineMax != want[2] || got.Buffer != want[3] ||
			got.AmalgamateTitle != limits.Amalgamate {
			mismatched = append(mismatched, schema)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("%d schema(s) have overflow limits upstream but none here: %s\n"+
			"Regenerate with testutil/scripts/overflow_limits.py.",
			len(missing), strings.Join(missing, ", "))
	}
	if len(mismatched) > 0 {
		sort.Strings(mismatched)
		t.Errorf("%d schema(s) carry limits that differ from upstream: %s",
			len(mismatched), strings.Join(mismatched, ", "))
	}
}

func derefOr(value *int, fallback int) int {
	if value == nil {
		return fallback
	}

	return *value
}

func loadUpstreamOverflowLimits(t *testing.T) map[string]upstreamLimits {
	t.Helper()

	t.Setenv("PYTHONPATH", testutil.AppriseSourceRoot(t))
	script := filepath.Join(testutil.RepoRoot(t),
		"internal", "testutil", "scripts", "overflow_limits.py")

	stdout, stderr, err := testutil.RunPythonScript(t, script)
	if err != nil {
		t.Fatalf("list upstream overflow limits: %v (stderr: %s)",
			err, strings.TrimSpace(stderr))
	}

	var limits map[string]upstreamLimits
	if err := json.Unmarshal([]byte(stdout), &limits); err != nil {
		t.Fatalf("decode overflow limits: %v", err)
	}

	return limits
}
