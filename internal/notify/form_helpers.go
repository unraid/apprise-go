package notify

import (
	"net/url"
	"strings"
)

type formPair struct {
	key   string
	value string
}

func encodeFormPairs(pairs []formPair) string {
	var b strings.Builder
	for i, pair := range pairs {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(percentEncode(pair.key))
		b.WriteByte('=')
		b.WriteString(percentEncode(pair.value))
	}
	return b.String()
}

// percentEncode matches upstream's urlencode, which quotes a space as %20
// rather than +. AWS SigV4 signs the raw payload string, so the difference
// changes the signature even though both forms decode identically.
func percentEncode(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}
