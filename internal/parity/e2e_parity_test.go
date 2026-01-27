//go:build e2e

package parity

import (
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

func schemaProviderMap(defs map[string]providerDefinition) map[string]string {
	out := map[string]string{}
	for name, def := range defs {
		for _, schema := range def.Schemas {
			normalized := strings.ToLower(strings.TrimSpace(schema))
			if normalized == "" {
				continue
			}
			out[normalized] = name
		}
	}
	return out
}

func caseScheme(t *testing.T, url string) string {
	t.Helper()

	parsed, err := notify.ParseURL(url)
	if err != nil {
		t.Fatalf("parse case url: %v", err)
	}
	return parsed.Scheme
}

func rewriteCaseScheme(t *testing.T, c providerCase, schema string) providerCase {
	t.Helper()

	parts := strings.SplitN(c.URL, "://", 2)
	if len(parts) != 2 {
		t.Fatalf("case url missing scheme: %s", c.URL)
	}
	c.URL = schema + "://" + parts[1]
	return c
}

func caseForSchema(t *testing.T, schema string, def providerDefinition) providerCase {
	t.Helper()

	for _, c := range def.Cases {
		if caseScheme(t, c.URL) == schema {
			return c
		}
	}

	if len(def.Cases) == 0 {
		t.Fatalf("provider %s has no cases", def.Name)
	}

	// Use a stable fallback case and rewrite the scheme to exercise every schema alias.
	return rewriteCaseScheme(t, def.Cases[0], schema)
}

func TestE2ERequestParityAllSchemas(t *testing.T) {
	defs := loadProviderDefinitions(t)
	providerBySchema := schemaProviderMap(defs)

	schemas := notify.SupportedSchemas()
	if len(schemas) == 0 {
		t.Fatalf("no supported schemas discovered")
	}

	for _, schema := range schemas {
		schema := strings.ToLower(strings.TrimSpace(schema))
		t.Run(schema, func(t *testing.T) {
			provider, ok := providerBySchema[schema]
			if !ok {
				t.Fatalf("no provider manifest for schema %s", schema)
			}
			builder, ok := providerBuilders[provider]
			if !ok {
				t.Fatalf("missing provider builder for %s (schema %s)", provider, schema)
			}

			def := defs[provider]
			c := caseForSchema(t, schema, def)
			notifyType := notify.NotifyInfo
			if strings.TrimSpace(c.Type) != "" {
				parsed, ok := notify.ParseNotifyType(c.Type)
				if !ok {
					t.Fatalf("invalid notify type %s for %s", c.Type, c.Name)
				}
				notifyType = parsed
			}

			logProgress(t, "python-vs-go "+schema)
			pythonSpecs := testutil.CapturePythonRequestsWithType(t, c.URL, c.Body, c.Title, notifyType)

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

			assertRequestSpecSequenceMatches(t, pythonSpecs, goSpecs)
		})
	}
}
