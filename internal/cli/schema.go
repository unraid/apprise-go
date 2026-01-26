package cli

import (
	"encoding/json"

	"github.com/unraid/apprise-go/internal/notify"
)

func SchemaJSON() ([]byte, error) {
	return notify.SchemaJSON()
}

func LoadSchema() (map[string]any, error) {
	data, err := SchemaJSON()
	if err != nil {
		return nil, err
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, err
	}
	return schema, nil
}
