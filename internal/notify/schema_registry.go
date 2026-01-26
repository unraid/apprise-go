package notify

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/unraid/apprise-go/internal/version"
)

type SchemaEntry map[string]any

type schemaEntryRecord struct {
	order int
	entry SchemaEntry
}

type schemaRegistry struct {
	mu       sync.RWMutex
	entries  []schemaEntryRecord
	asset    map[string]any
	bySchema map[string]SchemaEntry
}

var schemaRegistryState = &schemaRegistry{
	entries:  []schemaEntryRecord{},
	asset:    map[string]any{},
	bySchema: map[string]SchemaEntry{},
}

func RegisterSchemaEntryOrdered(order int, entry SchemaEntry) {
	schemaRegistryState.register(order, entry)
}

func RegisterSchemaAsset(asset map[string]any) {
	schemaRegistryState.registerAsset(asset)
}

func SchemaEntries() []SchemaEntry {
	return schemaRegistryState.entriesOrdered()
}

func SchemaEntryForSchema(schema string) (SchemaEntry, bool) {
	return schemaRegistryState.entryForSchema(schema)
}

func SchemaDetails() map[string]any {
	return schemaRegistryState.details()
}

func SchemaJSON() ([]byte, error) {
	details := SchemaDetails()
	data, err := json.Marshal(details)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (r *schemaRegistry) register(order int, entry SchemaEntry) {
	if entry == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.entries = append(r.entries, schemaEntryRecord{order: order, entry: entry})

	for _, schema := range extractSchemaList(entry, "protocols") {
		r.bySchema[strings.ToLower(schema)] = entry
	}
	for _, schema := range extractSchemaList(entry, "secure_protocols") {
		r.bySchema[strings.ToLower(schema)] = entry
	}
}

func (r *schemaRegistry) registerAsset(asset map[string]any) {
	if asset == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.asset = map[string]any{}
	for key, value := range asset {
		r.asset[key] = value
	}
}

func (r *schemaRegistry) entriesOrdered() []SchemaEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ordered := make([]schemaEntryRecord, len(r.entries))
	copy(ordered, r.entries)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].order < ordered[j].order
	})

	entries := make([]SchemaEntry, 0, len(ordered))
	for _, record := range ordered {
		entries = append(entries, record.entry)
	}
	return entries
}

func (r *schemaRegistry) entryForSchema(schema string) (SchemaEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.bySchema[strings.ToLower(strings.TrimSpace(schema))]
	return entry, ok
}

func (r *schemaRegistry) details() map[string]any {
	entries := r.entriesOrdered()

	asset := map[string]any{}
	r.mu.RLock()
	for key, value := range r.asset {
		asset[key] = value
	}
	r.mu.RUnlock()
	if mask := resolveImagePathMask(); mask != "" {
		asset["image_path_mask"] = mask
	}

	return map[string]any{
		"version": version.UpstreamVersion,
		"schemas": entries,
		"asset":   asset,
	}
}

func extractSchemaList(entry SchemaEntry, key string) []string {
	raw, ok := entry[key]
	if !ok || raw == nil {
		return nil
	}

	switch value := raw.(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		items := make([]string, 0, len(value))
		for _, item := range value {
			if item == nil {
				continue
			}
			items = append(items, fmt.Sprint(item))
		}
		return items
	default:
		return []string{fmt.Sprint(value)}
	}
}
