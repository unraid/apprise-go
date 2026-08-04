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
		golden := loadProviderGolden(t, def.Dir, def.Cases)
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

				attachments := loadCaseAttachments(t, c)
				goSpecs := testutil.CaptureGoRequests(t, func() error {
					return notify.SendWithAttachments(target, c.Body, c.Title, notifyType, attachments)
				})

				assertRequestSpecSequenceMatchesExcept(t, expected.Requests, goSpecs, def.VolatileHeaders)
			})
		}
	}
}

func loadProviderGolden(t *testing.T, providerDir string, defined []providerCase) []goldenCase {
	t.Helper()

	sendsNothing := map[string]bool{}
	for _, c := range defined {
		sendsNothing[c.Name] = c.SendsNothing
	}

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
		switch {
		case len(c.Requests) == 0 && !sendsNothing[c.Name]:
			t.Fatalf("golden %s missing requests for %s; if upstream really "+
				"sends nothing here, mark the case sends_nothing", goldenPath, c.Name)
		case len(c.Requests) > 0 && sendsNothing[c.Name]:
			t.Fatalf("golden %s has %d requests for %s but the case is marked "+
				"sends_nothing", goldenPath, len(c.Requests), c.Name)
		}
	}

	return cases
}

// loadCaseAttachments reads the files a case names, so a golden can pin an
// attachment request rather than only the plain notification.
func loadCaseAttachments(t *testing.T, c providerCase) []notify.Attachment {
	t.Helper()

	if len(c.Attachments) == 0 {
		return nil
	}

	attachments, err := notify.LoadAttachments(c.Attachments)
	if err != nil {
		t.Fatalf("load attachments for %s: %v", c.Name, err)
	}

	return attachments
}
