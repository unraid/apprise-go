package parity

import (
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
)

func TestProviderManifestsCoverSupportedSchemas(t *testing.T) {
	defs := loadProviderDefinitions(t)

	schemaToProvider := map[string]string{}
	for name, def := range defs {
		for _, schema := range def.Schemas {
			normalized := strings.ToLower(strings.TrimSpace(schema))
			if normalized == "" {
				t.Fatalf("provider %s has empty schema", name)
			}
			if existing, ok := schemaToProvider[normalized]; ok {
				t.Fatalf("schema %s defined in both %s and %s", normalized, existing, name)
			}
			schemaToProvider[normalized] = name
		}
	}

	missing := []string{}
	for _, schema := range notify.SupportedSchemas() {
		if isNonHTTPSchema(schema) {
			continue
		}
		if _, ok := schemaToProvider[schema]; !ok {
			missing = append(missing, schema)
		}
	}

	extra := []string{}
	for schema := range schemaToProvider {
		if !notify.SupportsSchema(schema) {
			extra = append(extra, schema)
		}
	}

	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("schema coverage mismatch: missing=%s extra=%s", formatList(missing), formatList(extra))
	}
}

func TestProviderBuildersMatchManifests(t *testing.T) {
	defs := loadProviderDefinitions(t)

	missingBuilders := []string{}
	for name := range defs {
		if _, ok := providerBuilders[name]; !ok {
			missingBuilders = append(missingBuilders, name)
		}
	}

	extraBuilders := []string{}
	for name := range providerBuilders {
		if _, ok := defs[name]; !ok {
			extraBuilders = append(extraBuilders, name)
		}
	}

	if len(missingBuilders) > 0 || len(extraBuilders) > 0 {
		t.Fatalf("provider builder mismatch: missing=%s extra=%s", formatList(missingBuilders), formatList(extraBuilders))
	}
}
