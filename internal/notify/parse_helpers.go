package notify

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var parseListDelimiters = regexp.MustCompile(`[\[\];,\s]+`)
var phoneAllowed = regexp.MustCompile(`^\+?[0-9\s)(+-]+\s*$`)

func parseDelimitedList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := parseListDelimiters.Split(raw, -1)
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values = append(values, part)
	}

	return values
}

// sortedUniqueTargets applies the deduplication and sorting upstream's
// parse_list performs, so the order a provider sends its targets in follows
// from the set rather than from how the URL happened to be written.
func sortedUniqueTargets(entries []string) []string {
	unique := map[string]struct{}{}
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		unique[trimmed] = struct{}{}
	}

	normalized := make([]string, 0, len(unique))
	for entry := range unique {
		normalized = append(normalized, entry)
	}
	sort.Strings(normalized)

	return normalized
}

func normalizePhone(raw string) (string, bool) {
	return normalizePhoneWithBounds(raw, 10, 14)
}

func normalizePhoneWithBounds(raw string, minLen, maxLen int) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	if !phoneAllowed.MatchString(raw) {
		return "", false
	}

	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}

	normalized := b.String()
	if normalized == "" {
		return "", false
	}
	if minLen > 0 && len(normalized) < minLen {
		return "", false
	}
	if maxLen > 0 && len(normalized) > maxLen {
		return "", false
	}
	return normalized, true
}

func normalizePhoneWithPlus(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	hasPlus := strings.HasPrefix(trimmed, "+")
	normalized, ok := normalizePhone(trimmed)
	if !ok {
		return "", false
	}
	if hasPlus {
		return "+" + normalized, true
	}
	return normalized, true
}

func mergeTitleBody(title, body string) string {
	if title == "" {
		return body
	}
	return title + "\r\n" + body
}

func normalizeNotifyFormat(raw string) string {
	format := strings.TrimSpace(strings.ToLower(raw))
	if format == "" {
		return ""
	}
	format = strings.TrimPrefix(format, "notifyformat.")
	switch format {
	case "md":
		return "markdown"
	default:
		return format
	}
}

// pythonQuotePath percent-encodes a URL path the way upstream's
// quote(..., safe=URL_PATH_SAFE_CHARS) does: RFC 3986 pchar sub-delims stay
// literal, minus comma and semicolon, which Apprise reserves as its own list
// delimiters. Go's net/url escaping keeps a different set (it leaves , and ;
// alone and escapes ! ' ( ) *), so the request path has to be built here.
func pythonQuotePath(path string) string {
	const safe = "/:@!$&'()*+="
	var out strings.Builder
	for i := 0; i < len(path); i++ {
		c := path[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' || strings.IndexByte(safe, c) >= 0 {
			out.WriteByte(c)
			continue
		}
		fmt.Fprintf(&out, "%%%02X", c)
	}
	return out.String()
}
