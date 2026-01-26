package notify

import (
	"strings"
	"sync"
)

type SchemaOverride func(target *ParsedURL, values map[string]SchemaValue)

type schemaOverrideRegistry struct {
	mu        sync.RWMutex
	overrides map[string][]SchemaOverride
}

var schemaOverrides = &schemaOverrideRegistry{
	overrides: map[string][]SchemaOverride{},
}

func RegisterSchemaOverride(schema string, override SchemaOverride) {
	schemaOverrides.register(schema, override)
}

func ApplySchemaOverrides(schema string, target *ParsedURL, values map[string]SchemaValue) {
	schemaOverrides.apply(schema, target, values)
}

func (r *schemaOverrideRegistry) register(schema string, override SchemaOverride) {
	if override == nil {
		return
	}

	key := strings.ToLower(strings.TrimSpace(schema))
	if key == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.overrides[key] = append(r.overrides[key], override)
}

func (r *schemaOverrideRegistry) apply(schema string, target *ParsedURL, values map[string]SchemaValue) {
	key := strings.ToLower(strings.TrimSpace(schema))
	if key == "" {
		return
	}

	r.mu.RLock()
	overrides := append([]SchemaOverride(nil), r.overrides[key]...)
	r.mu.RUnlock()

	for _, override := range overrides {
		override(target, values)
	}
}
