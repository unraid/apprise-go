package notify

import "testing"

func TestTargetBuildersCoverSupportedSchemas(t *testing.T) {
	for _, schema := range SupportedSchemas() {
		if _, ok := targetBuilders[schema]; !ok {
			t.Fatalf("missing target builder for supported schema %s", schema)
		}
	}
}
