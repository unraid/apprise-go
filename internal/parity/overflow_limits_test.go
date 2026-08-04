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

// overflowOverrides are the plugins that do their own splitting instead of
// taking the framework's. They convert or repair markup around the split —
// telegram merges the title itself and repairs markdown across chunks, and
// evolution, google_chat and slack convert CommonMark before splitting — so
// the limits table cannot describe them and ApplyOverflow does not try.
var overflowOverrides = map[string]string{
	"telegram":    "merges the title itself and repairs markdown across chunks",
	"evolution":   "converts HTML-derived CommonMark before splitting",
	"google_chat": "converts HTML-derived CommonMark before splitting",
	"slack":       "markdown-aware splitting that protects links",
}

// TestOverflowOverridesAreKnown fails when upstream adds a plugin that takes
// over its own splitting, so a new one is a decision rather than a silent
// difference in what gets sent.
//
// This list exists because the first two were found by tripping over them: a
// fixture failed, and only then did anyone look. Nothing was enumerating which
// plugins override the framework, which is the same shape as the framework
// arguments themselves going unnoticed.
func TestOverflowOverridesAreKnown(t *testing.T) {
	root := testutil.AppriseSourceRoot(t)

	found := map[string]bool{}
	err := filepath.Walk(filepath.Join(root, "apprise", "plugins"),
		func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".py") {
				return err
			}

			source, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !strings.Contains(string(source), "def _build_send_calls") &&
				!strings.Contains(string(source), "def _apply_overflow") {
				return nil
			}

			name := strings.TrimSuffix(filepath.Base(path), ".py")
			if name == "base" {
				// The framework's own definition, which is what the others
				// are overriding.
				name = filepath.Base(filepath.Dir(path))
				if name == "plugins" {
					return nil
				}
			}
			found[name] = true

			return nil
		})
	if err != nil {
		t.Fatalf("walk upstream plugins: %v", err)
	}

	var unlisted []string
	for name := range found {
		if _, ok := overflowOverrides[name]; !ok {
			unlisted = append(unlisted, name)
		}
	}
	if len(unlisted) > 0 {
		sort.Strings(unlisted)
		t.Errorf("%d plugin(s) override the framework's splitting and are not "+
			"listed: %s\nApplyOverflow does not describe them, so each is a "+
			"difference in what gets sent. Add it to overflowOverrides with "+
			"what it does differently.",
			len(unlisted), strings.Join(unlisted, ", "))
	}

	var stale []string
	for name := range overflowOverrides {
		if !found[name] {
			stale = append(stale, name)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("overflowOverrides names %d plugin(s) that no longer override "+
			"anything: %s", len(stale), strings.Join(stale, ", "))
	}
}
