package parity

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

// TestPartialFailureParity checks multi-target sends where the first target is
// rejected and the rest would succeed.
//
// TestFailureHandlingParity rejects every request, so both implementations take
// the same path: everything fails, both report failure, and a provider that
// abandons the send on the first error looks exactly like one that carries on.
// The interesting case is the one in between. Upstream records the error and
// keeps going, so a four-target notification with one bad target still reaches
// the other three and reports failure at the end. A port that returns on the
// first error reports failure too -- the verdict matches, the test passes, and
// three people never got told.
//
// So the verdict alone is not the assertion. What matters is which requests
// were actually issued after the failure, and that is compared here.
// knownPartialFailureGaps are provider cases where mid-send handling still
// differs from upstream, keyed "provider/case".
//
// Same ratchet as the URL vector baseline: a mismatch that is not listed fails
// the build, and a listed entry that has started passing also fails so it gets
// deleted. These are not approved -- they are unfixed, and the reason says what
// is actually wrong.
var knownPartialFailureGaps = map[string]string{
	"matrix/e2ee-disabled": "the port issues 4 requests to upstream's 6 after a rejected login. " +
		"Room sends now record-and-continue, but the login/register/join sequence " +
		"still diverges once the first request fails.",
	"twitter/x-default": "the port reports success where upstream reports failure; the failed " +
		"request is not making it into the verdict.",
}

func TestPartialFailureParity(t *testing.T) {
	defs := loadProviderDefinitions(t)

	type mismatch struct {
		provider string
		caseName string
		detail   string
	}

	var (
		checked    int
		multiCases int
		problems   []mismatch
	)

	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		def := defs[name]

		for _, c := range def.Cases {
			if c.SendsNothing || c.KnownDivergence != "" || len(c.Attachments) > 0 {
				continue
			}

			builder, ok := providerBuilders[name]
			if !ok {
				continue
			}
			parsed, err := notify.ParseURL(c.URL)
			if err != nil {
				continue
			}
			target, err := builder(parsed)
			if err != nil {
				continue
			}

			// A single-request case cannot show the difference: there is no
			// "rest of the send" to abandon.
			baseline := testutil.CapturePythonRequests(t, c.URL, c.Body, c.Title)
			if len(baseline) < 2 {
				continue
			}
			multiCases++

			pythonSpecs, pythonSuccess := testutil.CapturePythonRequestsWithFailuresResult(
				t, c.URL, c.Body, c.Title, 1)

			restore := testutil.FailNextRequests(1)
			var goErr error
			goSpecs := testutil.CaptureGoRequests(t, func() error {
				goErr = notify.DispatchSendWithInput(
					target, parsed, c.Body, c.Title, c.BodyFormat, notify.NotifyInfo, nil)
				return nil
			})
			restore()

			checked++

			if len(goSpecs) != len(pythonSpecs) {
				problems = append(problems, mismatch{
					provider: name,
					caseName: c.Name,
					detail: fmt.Sprintf("case %q: after the first request was rejected, "+
						"upstream issued %d request(s) and this port issued %d -- "+
						"the remaining targets are not being attempted",
						c.Name, len(pythonSpecs), len(goSpecs)),
				})
				continue
			}

			if pythonSuccess != nil && (*pythonSuccess) != (goErr == nil) {
				problems = append(problems, mismatch{
					provider: name,
					caseName: c.Name,
					detail: fmt.Sprintf("case %q: upstream reported success=%v, this port reported success=%v",
						c.Name, *pythonSuccess, goErr == nil),
				})
			}

			// Only the first case per provider is needed; the send loop is the
			// same for the rest.
			break
		}
	}

	if multiCases == 0 {
		t.Fatal("no multi-request case was found; this check is not exercising anything")
	}
	if checked == 0 {
		t.Fatal("no provider was exercised; the check is not running")
	}
	t.Logf("checked %d providers whose first request is rejected mid-send", checked)

	seen := map[string]bool{}
	var unexpected []string
	for _, p := range problems {
		key := p.provider + "/" + p.caseName
		seen[key] = true
		if _, known := knownPartialFailureGaps[key]; known {
			continue
		}
		unexpected = append(unexpected, "  "+p.provider+": "+p.detail)
	}

	if len(unexpected) > 0 {
		t.Errorf("%d provider(s) handle a mid-send failure differently from upstream:\n%s",
			len(unexpected), strings.Join(unexpected, "\n"))
	}

	for key := range knownPartialFailureGaps {
		if !seen[key] {
			t.Errorf("stale known-gap entry (mid-send handling now matches upstream, remove it): %s", key)
		}
	}
}
