package notify

import (
	"strings"
	"sync"
)

type secureSchemeRegistry struct {
	once    sync.Once
	schemes map[string]struct{}
}

var secureSchemesState secureSchemeRegistry

func isSecureScheme(scheme string) bool {
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if scheme == "" {
		return false
	}

	secureSchemesState.once.Do(func() {
		secureSchemesState.schemes = buildSecureSchemeRegistry()
	})

	_, ok := secureSchemesState.schemes[scheme]
	return ok
}

func buildSecureSchemeRegistry() map[string]struct{} {
	registry := map[string]struct{}{}
	for _, entry := range SchemaEntries() {
		for _, scheme := range extractSchemaList(entry, "secure_protocols") {
			scheme = strings.ToLower(strings.TrimSpace(scheme))
			if scheme == "" {
				continue
			}
			registry[scheme] = struct{}{}
		}
	}
	return registry
}

func secureSchemesSnapshot() map[string]struct{} {
	secureSchemesState.once.Do(func() {
		secureSchemesState.schemes = buildSecureSchemeRegistry()
	})

	out := make(map[string]struct{}, len(secureSchemesState.schemes))
	for scheme := range secureSchemesState.schemes {
		out[scheme] = struct{}{}
	}
	return out
}
