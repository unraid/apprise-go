package parity

var nonHTTPSchemas = map[string]struct{}{
	"mailto":  {},
	"mailtos": {},
}

func isNonHTTPSchema(schema string) bool {
	_, ok := nonHTTPSchemas[schema]
	return ok
}
