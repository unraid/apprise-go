package parity

import (
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

func TestProviderRequestParity(t *testing.T) {
	defs := loadProviderDefinitions(t)

	for _, name := range sortedProviderNames(defs) {
		def := defs[name]
		golden := loadProviderGolden(t, def.Dir)
		goldenByName := map[string]goldenCase{}
		for _, g := range golden {
			goldenByName[g.Name] = g
		}
		builder, ok := providerBuilders[name]
		if !ok {
			t.Fatalf("missing provider builder for %s", name)
		}

		for _, c := range def.Cases {
			c := c
			t.Run(name+"/"+c.Name, func(t *testing.T) {
				maybeParallel(t)
				logProgress(t, "python-vs-go "+name+"/"+c.Name)
				notifyType := notify.NotifyInfo
				if strings.TrimSpace(c.Type) != "" {
					parsed, ok := notify.ParseNotifyType(c.Type)
					if !ok {
						t.Fatalf("invalid notify type %s for %s", c.Type, c.Name)
					}
					notifyType = parsed
				}

				pythonSpecs, pythonSuccess := testutil.CapturePythonRequestsWithTypeResult(t, c.URL, c.Body, c.Title, notifyType)
				if expected, ok := goldenByName[c.Name]; ok {
					assertRequestSpecSequenceMatches(t, pythonSpecs, expected.Requests)
				} else {
					t.Fatalf("missing golden case for %s/%s", name, c.Name)
				}
				parsedURL, err := notify.ParseURL(c.URL)
				if err != nil {
					t.Fatalf("parse url: %v", err)
				}

				target, err := builder(parsedURL)
				if err != nil {
					t.Fatalf("build target: %v", err)
				}

				goSpecs, err := testutil.CaptureGoRequestsResult(t, func() error {
					return target.Send(c.Body, c.Title, notifyType)
				})
				if shouldSkip := assertNotifySuccessMatches(t, pythonSuccess, err); shouldSkip {
					return
				}
				if err != nil {
					t.Fatalf("send request failed: %v", err)
				}

				assertRequestSpecSequenceMatches(t, pythonSpecs, goSpecs)
			})
		}
	}
}
