package notify

import (
	"sort"
	"strings"
)

var coreSchemas = map[string]struct{}{
	"apprise":  {},
	"apprises": {},
	"gotify":   {},
	"gotifys":  {},
	"ifttt":    {},
	"json":     {},
	"jsons":    {},
	"ntfy":     {},
	"ntfys":    {},
	"form":     {},
	"forms":    {},
	"xml":      {},
	"xmls":     {},
}

var supportedSchemas = initSupportedSchemas()

func initSupportedSchemas() map[string]struct{} {
	merged := map[string]struct{}{}
	addSchemas(merged, coreSchemas)
	addSchemas(merged, chatSchemas)
	addSchemas(merged, emailSchemas)
	addSchemas(merged, pushSchemas)
	addSchemas(merged, smsSchemas)
	addSchemas(merged, systemSchemas)
	return merged
}

func addSchemas(dst, src map[string]struct{}) {
	for schema := range src {
		dst[schema] = struct{}{}
	}
}

func SupportedSchemas() []string {
	schemas := make([]string, 0, len(supportedSchemas))
	for schema := range supportedSchemas {
		schemas = append(schemas, schema)
	}
	sort.Strings(schemas)
	return schemas
}

func SupportsSchema(schema string) bool {
	_, ok := supportedSchemas[strings.ToLower(schema)]
	return ok
}
