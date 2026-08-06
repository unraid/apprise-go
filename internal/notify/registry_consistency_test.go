package notify

import (
	"sort"
	"strings"
	"testing"
)

// A working provider needs three separate registrations to agree: a schema
// entry describing it, membership in a category registry so the schema is
// supported, and a dispatcher so a URL resolves to a target. Nothing checked
// that they stayed in sync, and twice a provider shipped with its entry,
// dispatcher, fixtures and passing provider tests while the URL was
// unsupported because the registry membership was missing. Every provider
// test still passed, because they construct the target directly.

// schemaEntrySchemas returns every schema named by a registered schema entry.
func schemaEntrySchemas() []string {
	seen := map[string]struct{}{}
	for _, entry := range SchemaEntries() {
		for _, key := range []string{"protocols", "secure_protocols"} {
			values, ok := entry[key].([]string)
			if !ok {
				continue
			}
			for _, schema := range values {
				seen[strings.ToLower(schema)] = struct{}{}
			}
		}
	}

	schemas := make([]string, 0, len(seen))
	for schema := range seen {
		schemas = append(schemas, schema)
	}
	sort.Strings(schemas)

	return schemas
}

func TestSchemaEntriesAreSupported(t *testing.T) {
	var missing []string
	for _, schema := range schemaEntrySchemas() {
		if !SupportsSchema(schema) {
			missing = append(missing, schema)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("schemas have an entry but are not supported, so the URL will "+
			"not resolve; add them to a category registry: %s",
			strings.Join(missing, ", "))
	}
}

func TestSchemaEntriesHaveDispatchers(t *testing.T) {
	var missing []string
	for _, schema := range schemaEntrySchemas() {
		if _, ok := targetBuilders[schema]; !ok {
			missing = append(missing, schema)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("schemas have an entry but no dispatcher in targetBuilders: %s",
			strings.Join(missing, ", "))
	}
}

func TestSupportedSchemasHaveDispatchers(t *testing.T) {
	var missing []string
	for _, schema := range SupportedSchemas() {
		if _, ok := targetBuilders[strings.ToLower(schema)]; !ok {
			missing = append(missing, schema)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("schemas are supported but have no dispatcher, so a URL "+
			"reports support and then fails to build: %s",
			strings.Join(missing, ", "))
	}
}
