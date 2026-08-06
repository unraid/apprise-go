package notify

import (
	"fmt"
	"net/url"
	"regexp"
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

	// The prefixed query maps carry no order, but the URL they came from
	// did, and upstream emits these fields in the order they were written.
	// A map cannot be walked in that order, so the keys are kept alongside.
	QueryAddOrder     []string
	QueryDelOrder     []string
	QueryPayloadOrder []string
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
	if strings.EqualFold(schemeCandidate, "rocket") || strings.EqualFold(schemeCandidate, "rockets") {
		sanitized = sanitizeRocketAuthority(sanitized)
	}

	authority := urlAuthority(sanitized)
	useFirstAt := strings.Count(authority, "@") > 1
	if useFirstAt {
		parsed, parseErr := parseLenientURL(sanitized, schemeCandidate, true)
		if parseErr == nil {
			return parsed, nil
		}
	}

	u, err := url.Parse(sanitized)
	if err != nil {
		// Go's parser rejects a malformed percent-escape outright; Python's
		// urlsplit does not decode at all, so upstream carries the raw bytes
		// through and unquotes later with errors ignored. A URL like
		// tgram://token/%$/ or an encoded @ in a host reaches upstream intact
		// and is rejected -- or accepted -- on its merits rather than by the
		// parser, so the manual split has to handle these too.
		if strings.Contains(err.Error(), "invalid port") ||
			strings.Contains(err.Error(), "invalid URL escape") {
			parsed, parseErr := parseLenientURL(sanitized, schemeCandidate, useFirstAt)
			if parseErr == nil {
				return parsed, nil
			}
		}
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
			host = u.Host
			hasPort = false
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

	result := &ParsedURL{
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

		QueryAddOrder:     qsd.addOrder,
		QueryDelOrder:     qsd.delOrder,
		QueryPayloadOrder: qsd.payloadOrder,
	}
	applyUserPassOverrides(result)
	return result, nil
}

// rocketAuthorityRe matches a Rocket.Chat webhook embedded in the authority,
// the form rocket://webhook_a/webhook_b@host or
// rocket://user:webhook_a/webhook_b@host. The slash makes it illegal as
// userinfo, so no standard parser keeps it in one piece.
var rocketAuthorityRe = regexp.MustCompile(`(?is)^(?P<schema>[^:]+://)((?P<user>[^:/]+):)?(?P<webhook>[a-z0-9]+(?:/|%2F)[a-z0-9]+)@(?P<url>.+)$`)

// sanitizeRocketAuthority lifts an embedded webhook out of the authority and
// leaves it as ?webhook=, so what follows is an ordinary URL.
//
// Upstream does this rewrite in the plugin's parse_url before handing the
// result to the base parser. Doing it here instead means every later step --
// including the shared host and credential checks -- sees the same URL the
// plugin will, rather than an authority of "user:webhook_a" that is not a
// hostname and never claimed to be.
func sanitizeRocketAuthority(raw string) string {
	m := rocketAuthorityRe.FindStringSubmatch(raw)
	if m == nil {
		return raw
	}

	webhook := m[rocketAuthorityRe.SubexpIndex("webhook")]
	rest := m[rocketAuthorityRe.SubexpIndex("url")]
	if strings.Contains(rest, "webhook=") {
		// An explicit ?webhook= wins; leave the URL alone rather than
		// producing two of them.
		return raw
	}

	rebuilt := m[rocketAuthorityRe.SubexpIndex("schema")]
	if user := m[rocketAuthorityRe.SubexpIndex("user")]; user != "" {
		rebuilt += user + "@"
	}
	rebuilt += rest

	separator := "?"
	if strings.Contains(rebuilt, "?") {
		separator = "&"
	}
	return rebuilt + separator + "webhook=" + url.QueryEscape(webhook)
}

// applyUserPassOverrides applies the ?user= and ?pass= query arguments.
//
// Upstream does this in NotifyBase.parse_url, so it holds for every schema
// rather than being a per-plugin convenience: a URL can carry its credentials
// entirely in the query string and no plugin has to know about it. They
// override the userinfo outright when both are present.
func applyUserPassOverrides(parsed *ParsedURL) {
	if parsed == nil {
		return
	}
	if value, ok := parsed.Query["user"]; ok {
		parsed.User = value
		parsed.HasUser = true
	}
	if value, ok := parsed.Query["pass"]; ok {
		parsed.Password = value
		parsed.HasPassword = true
	}
}

func parseLenientURL(raw string, scheme string, splitFirstAt bool) (*ParsedURL, error) {
	parts := strings.SplitN(raw, "://", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid url")
	}
	rest := parts[1]
	authority := rest
	path := ""
	if idx := strings.Index(rest, "/"); idx != -1 {
		authority = rest[:idx]
		path = rest[idx:]
	}
	query := ""
	if idx := strings.Index(authority, "?"); idx != -1 {
		query = authority[idx+1:]
		authority = authority[:idx]
	} else if idx := strings.Index(path, "?"); idx != -1 {
		query = path[idx+1:]
		path = path[:idx]
	}

	user := ""
	password := ""
	hasUser := false
	hasPassword := false
	host := authority
	splitIdx := strings.LastIndex(authority, "@")
	if splitFirstAt {
		splitIdx = strings.Index(authority, "@")
	}
	if splitIdx != -1 {
		userinfo := authority[:splitIdx]
		host = strings.TrimLeft(authority[splitIdx+1:], "@")
		hasUser = true
		if parts := strings.SplitN(userinfo, ":", 2); len(parts) == 2 {
			user = parts[0]
			password = parts[1]
			hasPassword = true
		} else {
			user = userinfo
		}
	}

	qsd := parseQSD(query, false, true)

	// url.Parse decodes these components, so the manual split has to as well
	// or the same URL means different things depending on which path parsed
	// it -- and a host of "%20%20" would reach a provider as two literal
	// escapes instead of the whitespace upstream sees and rejects.
	loweredScheme := strings.ToLower(scheme)

	// Telegram carries its bot token as "id:secret" in the authority, which
	// looks exactly like userinfo. url.Parse's path applies this fix-up after
	// sanitizeTelegramAuthority; the manual split has to do the same or the
	// token arrives as a user and a password and no longer matches its own
	// format.
	if loweredScheme == "tgram" && strings.EqualFold(host, telegramAuthorityHost) && user != "" {
		host = user
		if password != "" {
			host += ":" + password
		}
		user, password = "", ""
		hasUser, hasPassword = false, false
	}

	result := &ParsedURL{
		Raw:          raw,
		Scheme:       loweredScheme,
		Host:         lenientUnquote(host),
		Port:         0,
		HasPort:      false,
		User:         lenientUnquote(user),
		HasUser:      hasUser,
		Password:     lenientUnquote(password),
		HasPassword:  hasPassword,
		Path:         lenientUnquote(path),
		Query:        qsd.qsd,
		QueryAdd:     qsd.add,
		QueryDel:     qsd.del,
		QueryPayload: qsd.payload,

		QueryAddOrder:     qsd.addOrder,
		QueryDelOrder:     qsd.delOrder,
		QueryPayloadOrder: qsd.payloadOrder,
	}
	applyUserPassOverrides(result)
	return result, nil
}

func urlAuthority(raw string) string {
	parts := strings.SplitN(raw, "://", 2)
	if len(parts) != 2 {
		return ""
	}
	rest := parts[1]
	for i, ch := range rest {
		if ch == '/' || ch == '?' || ch == '#' {
			return rest[:i]
		}
	}
	return rest
}

type qsdResult struct {
	qsd     map[string]string
	add     map[string]string
	del     map[string]string
	payload map[string]string

	addOrder     []string
	delOrder     []string
	payloadOrder []string
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

		// A repeated key keeps its first position, matching the maps, where
		// the later value simply overwrites the earlier one.
		if strings.HasPrefix(key, "+") && len(key) > 1 {
			if _, seen := result.add[key[1:]]; !seen {
				result.addOrder = append(result.addOrder, key[1:])
			}
			result.add[key[1:]] = val
		}
		if strings.HasPrefix(key, "-") && len(key) > 1 {
			if _, seen := result.del[key[1:]]; !seen {
				result.delOrder = append(result.delOrder, key[1:])
			}
			result.del[key[1:]] = val
		}
		if strings.HasPrefix(key, ":") && len(key) > 1 {
			if _, seen := result.payload[key[1:]]; !seen {
				result.payloadOrder = append(result.payloadOrder, key[1:])
			}
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

// lenientUnquote decodes percent-escapes the way Python's urllib.parse.unquote
// does: a well-formed %XX becomes its byte, and anything else is left exactly
// as written rather than failing the parse. Go's url.Parse rejects a malformed
// escape outright, but upstream never sees that error -- urlsplit does not
// decode at all -- so a URL carrying a stray % reaches the provider intact and
// is judged on its contents.
func lenientUnquote(value string) string {
	if !strings.Contains(value, "%") {
		return value
	}

	var out strings.Builder
	out.Grow(len(value))
	for i := 0; i < len(value); {
		if value[i] == '%' && i+2 < len(value) {
			hi, hiOK := unhex(value[i+1])
			lo, loOK := unhex(value[i+2])
			if hiOK && loOK {
				out.WriteByte(hi<<4 | lo)
				i += 3
				continue
			}
		}
		out.WriteByte(value[i])
		i++
	}
	return out.String()
}

func unhex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
