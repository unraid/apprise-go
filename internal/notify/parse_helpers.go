package notify

import (
	"regexp"
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
	if strings.HasPrefix(format, "notifyformat.") {
		format = strings.TrimPrefix(format, "notifyformat.")
	}
	switch format {
	case "md":
		return "markdown"
	default:
		return format
	}
}
