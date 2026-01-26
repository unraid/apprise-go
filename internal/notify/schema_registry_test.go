package notify

import "testing"

func TestSchemaRegistryLookup(t *testing.T) {
	entry, ok := SchemaEntryForSchema("apprise")
	if !ok {
		t.Fatalf("schema not found")
	}
	if entry["service_name"] != "Apprise API" {
		t.Fatalf("service_name mismatch: %v", entry["service_name"])
	}
}

func TestSchemaRegistryDetailsAsset(t *testing.T) {
	details := SchemaDetails()
	asset, ok := details["asset"].(map[string]any)
	if !ok {
		t.Fatalf("asset missing")
	}
	if asset["app_id"] != "Apprise" {
		t.Fatalf("asset app_id mismatch: %v", asset["app_id"])
	}
}
