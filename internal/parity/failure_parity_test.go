package parity

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

// TestFailureHandlingParity checks that a failed request is judged the same
// way on both sides.
//
// Every mock in the harness answers 200, so until now nothing compared what
// either implementation does with a rejection. A provider that treats a 500 as
// success — or one that gives up where upstream retries a second target —
// would look identical in every request fixture.
func TestFailureHandlingParity(t *testing.T) {
	defs := loadProviderDefinitions(t)

	checked, mismatched := 0, []string{}
	for name := range defs {
		def := defs[name]
		if len(def.Cases) == 0 {
			continue
		}

		c := def.Cases[0]
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

		// Fail everything, so neither side can reach a success path.
		_, pythonSuccess := testutil.CapturePythonRequestsWithFailuresResult(
			t, c.URL, c.Body, c.Title, 1000)

		restore := testutil.FailNextRequests(1000)
		var goErr error
		testutil.CaptureGoRequests(t, func() error {
			goErr = notify.DispatchSendWithInput(
				target, parsed, c.Body, c.Title, c.BodyFormat, notify.NotifyInfo, nil)

			return nil
		})
		restore()

		checked++
		goSucceeded := goErr == nil
		if pythonSuccess == nil {
			continue
		}
		if *pythonSuccess != goSucceeded {
			mismatched = append(mismatched, name)
		}
	}

	if checked == 0 {
		t.Fatal("no provider was exercised; the check is not running")
	}

	t.Logf("checked %d providers against a server that rejects everything", checked)

	if len(mismatched) > 0 {
		t.Errorf("%d provider(s) disagree with upstream on whether a rejected "+
			"request is a failure: %s", len(mismatched), strings.Join(mismatched, ", "))
	}
}

var _ = json.Marshal
