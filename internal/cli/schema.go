package cli

import (
	"bytes"
	_ "embed"
	"encoding/json"
)

//go:embed schema.json
var schemaJSON []byte

func SchemaJSON() []byte {
	return bytes.TrimSpace(schemaJSON)
}

func LoadSchema() (map[string]any, error) {
	var schema map[string]any
	if err := json.Unmarshal(SchemaJSON(), &schema); err != nil {
		return nil, err
	}
	return schema, nil
}
