package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const mastodonStatusPath = "/api/v1/statuses"
const mastodonDefaultVisibility = "default"
const tootDefaultVisibility = "public"

var mastodonUserPattern = regexp.MustCompile(`^[A-Za-z0-9_]+(@[A-Za-z0-9_.-]+)?$`)
var mastodonMentionPattern = regexp.MustCompile(`(?i)@[A-Z0-9_]+(?:@[A-Z0-9_.-]+)?`)

type MastodonTarget struct {
	host              string
	port              int
	secure            bool
	token             string
	targets           []string
	visibility        string
	visibilityDefault string
	sensitive         bool
	spoiler           string
	language          string
	idempotencyKey    string
}

func NewMastodonTarget(target *ParsedURL) (*MastodonTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	token := strings.TrimSpace(target.Query["token"])
	if token == "" && strings.TrimSpace(target.Password) == "" && strings.TrimSpace(target.User) != "" {
		token = strings.TrimSpace(target.User)
	}
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	targets := []string{}
	for _, entry := range splitPath(target.Path) {
		if normalized, ok := normalizeMastodonTarget(entry); ok {
			targets = append(targets, normalized)
		}
	}

	visibility := strings.ToLower(strings.TrimSpace(target.Query["visibility"]))
	visibilityDefault := mastodonDefaultVisibility
	if strings.HasPrefix(strings.ToLower(target.Scheme), "toot") {
		visibilityDefault = tootDefaultVisibility
	}
	if visibility == "" {
		visibility = visibilityDefault
	}

	sensitive := parseBoolValue(target.Query["sensitive"], false)

	return &MastodonTarget{
		host:              host,
		port:              target.Port,
		secure:            strings.EqualFold(target.Scheme, "mastodons") || strings.EqualFold(target.Scheme, "toots"),
		token:             token,
		targets:           targets,
		visibility:        visibility,
		visibilityDefault: visibilityDefault,
		sensitive:         sensitive,
		spoiler:           strings.TrimSpace(target.Query["spoiler"]),
		language:          strings.TrimSpace(target.Query["language"]),
		idempotencyKey:    strings.TrimSpace(target.Query["key"]),
	}, nil
}

func (m *MastodonTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	message := mergeTitleBody(title, body)
	status := message
	mentions := extractMastodonMentions(message)
	if len(mentions) > 0 {
		seen := map[string]struct{}{}
		targetSet := map[string]struct{}{}
		for _, entry := range m.targets {
			targetSet[entry] = struct{}{}
		}
		filtered := []string{}
		for _, mention := range mentions {
			if _, ok := seen[mention]; ok {
				continue
			}
			seen[mention] = struct{}{}
			if _, ok := targetSet[mention]; ok {
				continue
			}
			filtered = append(filtered, mention)
		}
		if len(filtered) > 0 {
			status = strings.Join(filtered, " ") + " " + message
		}
	}

	payload := map[string]any{
		"status":    status,
		"sensitive": m.sensitive,
	}
	if m.visibility != "" && m.visibility != mastodonDefaultVisibility {
		payload["visibility"] = m.visibility
	}
	if m.spoiler != "" {
		payload["spoiler_text"] = m.spoiler
	}
	if m.language != "" {
		payload["language"] = m.language
	}
	if m.idempotencyKey != "" {
		payload["Idempotency-Key"] = m.idempotencyKey
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    m.baseURL() + mastodonStatusPath,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Authorization": "Bearer " + m.token,
			"Content-Type":  "application/json",
		},
		Body: string(data),
	}, nil
}

func extractMastodonMentions(message string) []string {
	if message == "" {
		return nil
	}
	indices := mastodonMentionPattern.FindAllStringIndex(message, -1)
	if len(indices) == 0 {
		return nil
	}
	mentions := make([]string, 0, len(indices))
	for _, index := range indices {
		start, end := index[0], index[1]
		if start < 0 || end <= start || end > len(message) {
			continue
		}
		if end < len(message) {
			r, _ := utf8.DecodeRuneInString(message[end:])
			if !isMentionDelimiter(r) {
				continue
			}
		}
		mentions = append(mentions, message[start:end])
	}
	return mentions
}

func isMentionDelimiter(r rune) bool {
	if r <= 0x20 {
		return true
	}
	switch r {
	case ',', '.', '&', '(', ')', '[', ']':
		return true
	default:
		return false
	}
}

func (m *MastodonTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := m.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (m *MastodonTarget) baseURL() string {
	scheme := "http"
	if m.secure {
		scheme = "https"
	}

	base := fmt.Sprintf("%s://%s", scheme, m.host)
	if m.port > 0 {
		base += fmt.Sprintf(":%d", m.port)
	}

	return base
}

func normalizeMastodonTarget(raw string) (string, bool) {
	entry := strings.TrimSpace(raw)
	if entry == "" {
		return "", false
	}
	entry = strings.TrimPrefix(entry, "@")
	if !mastodonUserPattern.MatchString(entry) {
		return "", false
	}
	return "@" + entry, true
}

func init() {
	RegisterSchemaEntryOrdered(30, SchemaEntry{
		"attachment_support": true,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"batch": map[string]any{
					"default":  true,
					"map_to":   "batch",
					"name":     "Batch Mode",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"cache": map[string]any{
					"default":  true,
					"map_to":   "cache",
					"name":     "Cache Results",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"cto": map[string]any{
					"default":  4,
					"map_to":   "cto",
					"name":     "Socket Connect Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"emojis": map[string]any{
					"default":  false,
					"map_to":   "emojis",
					"name":     "Interpret Emojis",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"format": map[string]any{
					"default":  "text",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"key": map[string]any{
					"map_to":   "key",
					"name":     "Idempotency-Key",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"language": map[string]any{
					"map_to":   "language",
					"name":     "Language Code",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"overflow": map[string]any{
					"default":  "upstream",
					"map_to":   "overflow",
					"name":     "Overflow Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"split", "truncate", "upstream"},
				},
				"rto": map[string]any{
					"default":  4,
					"map_to":   "rto",
					"name":     "Socket Read Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"sensitive": map[string]any{
					"default":  false,
					"map_to":   "sensitive",
					"name":     "Sensitive Attachments",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"spoiler": map[string]any{
					"map_to":   "spoiler",
					"name":     "Spoiler Text",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"store": map[string]any{
					"default":  true,
					"map_to":   "store",
					"name":     "Persistent Storage",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"to": map[string]any{
					"alias_of": "targets",
					"delim":    []string{",", " "},
				},
				"token": map[string]any{
					"alias_of": "token",
				},
				"tz": map[string]any{
					"default":  nil,
					"map_to":   "tz",
					"name":     "Timezone",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"verify": map[string]any{
					"default":  true,
					"map_to":   "verify",
					"name":     "Verify SSL",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"visibility": map[string]any{
					"default":  "default",
					"map_to":   "visibility",
					"name":     "Visibility",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"default", "direct", "private", "unlisted", "public"},
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{token}@{host}", "{schema}://{token}@{host}:{port}", "{schema}://{token}@{host}/{targets}", "{schema}://{token}@{host}:{port}/{targets}"},
			"tokens": map[string]any{
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"port": map[string]any{
					"map_to":   "port",
					"max":      65535,
					"min":      1,
					"name":     "Port",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"mastodon", "mastodons", "toot", "toots"},
				},
				"target_user": map[string]any{
					"map_to":   "targets",
					"name":     "Target User",
					"prefix":   "@",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_user"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Access Token",
					"private":  false,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"mastodon", "toot"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"mastodons", "toots"},
		"service_name":     "Mastodon",
		"service_url":      "https://joinmastodon.org",
		"setup_url":        "https://appriseit.com/services/mastodon/",
	})
}
