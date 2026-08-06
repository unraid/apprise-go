// Command schema_dump prints the Go schema registry details as JSON, keyed by
// schema, so it can be diffed against the upstream Python details output.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/unraid/apprise-go/internal/notify"
)

func main() {
	out := map[string]any{}
	for _, entry := range notify.SchemaEntries() {
		details, ok := entry["details"]
		if !ok {
			continue
		}
		for _, key := range []string{"protocols", "secure_protocols"} {
			values, ok := entry[key].([]string)
			if !ok {
				continue
			}
			for _, schema := range values {
				out[schema] = details
			}
		}
	}

	data, err := json.Marshal(out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}
