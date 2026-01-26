package notify

import (
	"strings"
	"testing"
)

func TestSecureSchemesMatchesSchema(t *testing.T) {
	expected := map[string]struct{}{}
	insecure := map[string]struct{}{}

	for _, entry := range SchemaEntries() {
		for _, scheme := range extractSchemaList(entry, "secure_protocols") {
			scheme = strings.ToLower(strings.TrimSpace(scheme))
			if scheme == "" {
				continue
			}
			expected[scheme] = struct{}{}
		}
		for _, scheme := range extractSchemaList(entry, "protocols") {
			scheme = strings.ToLower(strings.TrimSpace(scheme))
			if scheme == "" {
				continue
			}
			if _, ok := expected[scheme]; ok {
				continue
			}
			insecure[scheme] = struct{}{}
		}
	}

	actual := secureSchemesSnapshot()
	for scheme := range expected {
		if !isSecureScheme(scheme) {
			t.Fatalf("secure scheme missing: %s", scheme)
		}
	}
	for scheme := range actual {
		if _, ok := expected[scheme]; !ok {
			t.Fatalf("unexpected secure scheme: %s", scheme)
		}
	}
	for scheme := range insecure {
		if isSecureScheme(scheme) {
			t.Fatalf("unexpected secure protocol match: %s", scheme)
		}
	}
}
