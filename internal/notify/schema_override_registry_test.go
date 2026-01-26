package notify

import "testing"

func TestSchemaOverrideRegistryApplies(t *testing.T) {
	values := map[string]SchemaValue{}
	RegisterSchemaOverride("testschema", func(_ *ParsedURL, values map[string]SchemaValue) {
		values["foo"] = schemaValueString("bar")
	})

	ApplySchemaOverrides("testschema", &ParsedURL{}, values)
	value, ok := values["foo"]
	if !ok {
		t.Fatalf("missing override value")
	}
	if value.Value != "bar" {
		t.Fatalf("override value mismatch: %v", value.Value)
	}
}
