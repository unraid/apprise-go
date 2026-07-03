package parity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

type goldenCase struct {
	Name     string               `json:"name"`
	Requests []notify.RequestSpec `json:"requests"`
}

func TestProviderGoldenRequests(t *testing.T) {
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
			t.Run(name+"/"+c.Name, func(t *testing.T) {
				logProgress(t, "golden "+name+"/"+c.Name)
				expected, ok := goldenByName[c.Name]
				if !ok {
					t.Fatalf("missing golden case for %s/%s", name, c.Name)
				}

				notifyType := notify.NotifyInfo
				if strings.TrimSpace(c.Type) != "" {
					parsed, ok := notify.ParseNotifyType(c.Type)
					if !ok {
						t.Fatalf("invalid notify type %s for %s", c.Type, c.Name)
					}
					notifyType = parsed
				}

				parsedURL, err := notify.ParseURL(c.URL)
				if err != nil {
					t.Fatalf("parse url: %v", err)
				}

				target, err := builder(parsedURL)
				if err != nil {
					t.Fatalf("build target: %v", err)
				}

				goSpecs := testutil.CaptureGoRequests(t, func() error {
					return target.Send(c.Body, c.Title, notifyType)
				})

				assertRequestSpecSequenceMatches(t, expected.Requests, goSpecs)
			})
		}
	}
}

func loadProviderGolden(t *testing.T, providerDir string) []goldenCase {
	t.Helper()

	goldenPath := filepath.Join(providerDir, "golden.json")
	data, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}

	var cases []goldenCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("parse golden %s: %v", goldenPath, err)
	}

	if len(cases) == 0 {
		t.Fatalf("golden %s empty", goldenPath)
	}

	seen := map[string]struct{}{}
	for _, c := range cases {
		if strings.TrimSpace(c.Name) == "" {
			t.Fatalf("golden %s contains empty name", goldenPath)
		}
		if _, ok := seen[c.Name]; ok {
			t.Fatalf("golden %s has duplicate name %s", goldenPath, c.Name)
		}
		seen[c.Name] = struct{}{}
		if len(c.Requests) == 0 {
			t.Fatalf("golden %s missing requests for %s", goldenPath, c.Name)
		}
	}

	return cases
}
