package notify

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const telegramAuthorityHost = "tgram.local"

type ParsedURL struct {
	Raw          string
	Scheme       string
	Host         string
	Port         int
	HasPort      bool
	User         string
	HasUser      bool
	Password     string
	HasPassword  bool
	Path         string
	Query        map[string]string
	QueryAdd     map[string]string
	QueryDel     map[string]string
	QueryPayload map[string]string
}

func ParseURL(raw string) (*ParsedURL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty url")
	}

	schemeCandidate := ""
	if parts := strings.SplitN(raw, "://", 2); len(parts) == 2 {
		schemeCandidate = parts[0]
	}
	if schemeCandidate == "" {
		return nil, fmt.Errorf("missing scheme")
	}

	sanitized := sanitizeFragment(raw)
	if strings.EqualFold(schemeCandidate, "tgram") {
		sanitized = sanitizeTelegramAuthority(sanitized)
	}

	u, err := url.Parse(sanitized)
	if err != nil {
		if schemeCandidate[0] < '0' || schemeCandidate[0] > '9' {
			return nil, err
		}

		parts := strings.SplitN(sanitized, "://", 2)
		if len(parts) != 2 || parts[1] == "" {
			return nil, err
		}
		parsed, parseErr := url.Parse("scheme://" + parts[1])
		if parseErr != nil {
			return nil, err
		}
		u = parsed
		u.Scheme = schemeCandidate
	}

	if u.Scheme == "" {
		return nil, fmt.Errorf("missing scheme")
	}

	host := u.Hostname()

	port := 0
	hasPort := false
	if portRaw := u.Port(); portRaw != "" {
		hasPort = true
		value, err := strconv.Atoi(portRaw)
		if err != nil {
			if strings.EqualFold(u.Scheme, "tgram") {
				host = u.Host
				hasPort = false
			} else {
				return nil, fmt.Errorf("invalid port: %s", portRaw)
			}
		} else {
			port = value
		}
	} else if strings.EqualFold(u.Scheme, "tgram") && strings.Contains(u.Host, ":") {
		host = u.Host
	}

	user := ""
	password := ""
	hasUser := false
	hasPassword := false
	if u.User != nil {
		hasUser = true
		user = u.User.Username()
		if pw, ok := u.User.Password(); ok {
			password = pw
			hasPassword = true
		}
	}
	if strings.EqualFold(u.Scheme, "tgram") && strings.EqualFold(u.Hostname(), telegramAuthorityHost) && user != "" {
		host = user
		if password != "" {
			host += ":" + password
		}
		user = ""
		password = ""
		hasUser = false
		hasPassword = false
	}

	parsedPath := u.EscapedPath()
	if parsedPath == "." {
		parsedPath = ""
	}

	qsd := parseQSD(u.RawQuery, false, true)

	return &ParsedURL{
		Raw:          raw,
		Scheme:       strings.ToLower(u.Scheme),
		Host:         host,
		Port:         port,
		HasPort:      hasPort,
		User:         user,
		HasUser:      hasUser,
		Password:     password,
		HasPassword:  hasPassword,
		Path:         parsedPath,
		Query:        qsd.qsd,
		QueryAdd:     qsd.add,
		QueryDel:     qsd.del,
		QueryPayload: qsd.payload,
	}, nil
}

type qsdResult struct {
	qsd     map[string]string
	add     map[string]string
	del     map[string]string
	payload map[string]string
}

func parseQSD(raw string, plusToSpace bool, sanitize bool) qsdResult {
	result := qsdResult{
		qsd:     map[string]string{},
		add:     map[string]string{},
		del:     map[string]string{},
		payload: map[string]string{},
	}

	if raw == "" {
		return result
	}

	pairs := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '&' || r == ';'
	})

	for _, pair := range pairs {
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, "=", 2)
		key := parts[0]
		val := ""
		if len(parts) == 2 {
			val = parts[1]
		}

		key = normalizeKey(key)
		key = decodeQueryValue(key)
		key = strings.TrimSpace(key)

		if plusToSpace {
			val = strings.ReplaceAll(val, "+", " ")
		}
		val = decodeQueryValue(val)
		val = strings.TrimSpace(val)

		storeKey := key
		if sanitize {
			storeKey = strings.ToLower(strings.TrimSpace(key))
		}
		result.qsd[storeKey] = val

		if strings.HasPrefix(key, "+") && len(key) > 1 {
			result.add[key[1:]] = val
		}
		if strings.HasPrefix(key, "-") && len(key) > 1 {
			result.del[key[1:]] = val
		}
		if strings.HasPrefix(key, ":") && len(key) > 1 {
			result.payload[key[1:]] = val
		}
	}

	return result
}

func normalizeKey(raw string) string {
	if raw == "" {
		return ""
	}

	first := raw[:1]
	rest := ""
	if len(raw) > 1 {
		rest = strings.ReplaceAll(raw[1:], "+", " ")
	}
	return first + rest
}

func decodeQueryValue(value string) string {
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}

func sanitizeFragment(raw string) string {
	if !strings.Contains(raw, "#") {
		return raw
	}
	return strings.ReplaceAll(raw, "#", "%23")
}

func sanitizeTelegramAuthority(raw string) string {
	parts := strings.SplitN(raw, "://", 2)
	if len(parts) != 2 {
		return raw
	}

	scheme := parts[0]
	authority := parts[1]
	suffix := ""
	if idx := strings.IndexAny(authority, "/?#"); idx != -1 {
		suffix = authority[idx:]
		authority = authority[:idx]
	}

	if strings.Contains(authority, "@") {
		return raw
	}

	decoded, err := url.PathUnescape(authority)
	if err != nil {
		decoded = authority
	}

	tokenParts := strings.SplitN(decoded, ":", 2)
	if len(tokenParts) != 2 || tokenParts[0] == "" || tokenParts[1] == "" {
		return raw
	}

	return scheme + "://" + tokenParts[0] + ":" + tokenParts[1] + "@" + telegramAuthorityHost + suffix
}
