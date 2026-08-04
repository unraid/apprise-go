package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const mastodonStatusPath = "/api/v1/statuses"

// Media is uploaded first and the status then references the ids it gets back.
const mastodonMediaPath = "/api/v1/media"
const mastodonDefaultVisibility = "default"
const tootDefaultVisibility = "public"

var mastodonUserPattern = regexp.MustCompile(`^[A-Za-z0-9_]+(@[A-Za-z0-9_.-]+)?$`)
var mastodonMentionPattern = regexp.MustCompile(`(?i)@[A-Z0-9_]+(?:@[A-Z0-9_.-]+)?`)

// A hashtag may not be all digits once underscores are removed, which is what
// keeps "#123" from being treated as one.
var mastodonHashtagPattern = regexp.MustCompile(`^[^\W_][\w]*$`)
var mastodonHashtagDigits = regexp.MustCompile(`^[0-9]+$`)

// Go's regexp has no lookaround, so the boundary conditions upstream writes as
// (?<![#%\w]) and (?![#%\w]) are handled by capturing the neighbours.
var mastodonHashtagDetectPattern = regexp.MustCompile(`(?:^|[^#%\w])(#[^\W_][\w]*)(?:$|[^#%\w])`)

type MastodonTarget struct {
	host              string
	port              int
	secure            bool
	token             string
	targets           []string
	hashtags          []string
	ping              []string
	visibility        string
	visibilityDefault string
	sensitive         bool
	spoiler           string
	language          string
	idempotencyKey    string
	format            string
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

	// A path entry is either a user to mention or a hashtag to append; they
	// are carried separately because only mentions prefix a direct message.
	targets := []string{}
	hashtags := []string{}
	hashtagSeen := map[string]struct{}{}
	for _, entry := range sortedUniqueTargets(splitPath(target.Path)) {
		if normalized, ok := normalizeMastodonTarget(entry); ok {
			targets = append(targets, normalized)
			continue
		}
		if normalized, ok := normalizeMastodonHashtag(entry); ok {
			key := strings.ToLower(normalized)
			if _, seen := hashtagSeen[key]; !seen {
				hashtagSeen[key] = struct{}{}
				hashtags = append(hashtags, normalized)
			}
		}
	}

	// ?ping= names mentions and hashtags to append to every status.
	ping := []string{}
	pingSeen := map[string]struct{}{}
	// Upstream reads this through parse_list, which sorts as well as
	// deduplicates, so a hashtag sorts ahead of a mention regardless of the
	// order they were written in.
	for _, entry := range sortedUniqueTargets(parseDelimitedList(target.Query["ping"])) {
		normalized, ok := normalizeMastodonPingToken(entry)
		if !ok {
			continue
		}
		key := strings.ToLower(normalized)
		if _, seen := pingSeen[key]; seen {
			continue
		}
		pingSeen[key] = struct{}{}
		ping = append(ping, normalized)
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
		hashtags:          hashtags,
		ping:              ping,
		visibility:        visibility,
		visibilityDefault: visibilityDefault,
		sensitive:         sensitive,
		spoiler:           strings.TrimSpace(target.Query["spoiler"]),
		language:          strings.TrimSpace(target.Query["language"]),
		idempotencyKey:    strings.TrimSpace(target.Query["key"]),
		format:            normalizeNotifyFormat(target.Query["format"]),
	}, nil
}

func (m *MastodonTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	return m.buildRequest(body, title, notifyType, nil)
}

func (m *MastodonTarget) buildRequest(body, title string, notifyType NotifyType, mediaIDs []string) (RequestSpec, error) {
	message := mergeTitleBody(title, body)

	// Only a direct message prefixes its recipients, and a mention already
	// written into the body is not repeated.
	prefixed := []string{}
	if m.visibility == "direct" {
		inBody := map[string]struct{}{}
		for _, mention := range extractMastodonMentions(message) {
			inBody[mention] = struct{}{}
		}
		for _, entry := range m.targets {
			if _, ok := inBody[entry]; !ok {
				prefixed = append(prefixed, entry)
			}
		}
	}

	status := message
	if len(prefixed) > 0 {
		status = strings.Join(prefixed, " ") + " " + message
	}
	status += mastodonPingPayload(m.pingTokens(message, prefixed))

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
	if len(mediaIDs) > 0 {
		payload["media_ids"] = mediaIDs
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

// mastodonMediaPattern is what Mastodon will transcode; anything else is
// ignored rather than rejected, so it would silently never appear.
var mastodonMediaPattern = regexp.MustCompile(`(?i)^(image|video|audio)/.*`)

func (m *MastodonTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	mediaIDs := []string{}
	for index, attachment := range attachments {
		if !mastodonMediaPattern.MatchString(attachment.MimeType) {
			continue
		}

		id, err := m.uploadMedia(attachment, index)
		if err != nil {
			return err
		}
		mediaIDs = append(mediaIDs, id)
	}

	spec, err := m.buildRequest(body, title, notifyType, mediaIDs)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

// uploadMedia posts one file and returns the id the status references.
func (m *MastodonTarget) uploadMedia(attachment Attachment, index int) (string, error) {
	name := attachment.FileName(index, ".dat")

	// The filename doubles as the media description, which is what a client
	// reads out as alt text.
	fields := formFields{}
	fields.Set("description", name)

	// Mastodon is handed a filename and a handle without a type, so the part
	// is labelled application/octet-stream rather than the file's own type.
	requestBody, contentType, err := singleFileAttachmentBody(
		fields, "file",
		Attachment{
			Name: name,
			Data: attachment.Data,
		}, true)
	if err != nil {
		return "", err
	}

	var response struct {
		ID json.Number `json:"id"`
	}
	if err := doJSONRequest(RequestSpec{
		Method: "POST",
		URL:    m.baseURL() + mastodonMediaPath,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Authorization": "Bearer " + m.token,
			"Content-Type":  contentType,
		},
		Body: requestBody,
	}, &response); err != nil {
		return "", err
	}
	if response.ID.String() == "" {
		return "", fmt.Errorf("mastodon media upload returned no id")
	}

	return response.ID.String(), nil
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
					"default":  4.0,
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
					"default":  4.0,
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
				"ping": map[string]any{
					"delim":    []string{",", " "},
					"group":    []any{},
					"map_to":   "ping",
					"name":     "Ping Users/Tags",
					"private":  false,
					"required": false,
					"type":     "list:string",
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

// pingTokens returns the mention and hashtag tokens to append to a status.
// Anything already prefixed onto the status, and — in markdown, where the
// body is scanned — anything already visible in it, is left out so a mention
// is never doubled up.
func (m *MastodonTarget) pingTokens(message string, prefixed []string) []string {
	seen := map[string]struct{}{}
	for _, entry := range prefixed {
		seen[strings.ToLower(entry)] = struct{}{}
	}

	tokens := []string{}
	if m.format == "markdown" {
		// Markdown can carry visible tokens, so the body contributes its own.
		tokens = append(tokens, mastodonScanTokens(message, seen)...)
	} else {
		// Other formats are not scanned, but tokens already written into the
		// body still suppress a duplicate copy at the end.
		mastodonScanTokens(message, seen)
	}

	configured := append(append(append([]string{}, m.targets...), m.hashtags...), m.ping...)
	for _, entry := range configured {
		normalized, ok := normalizeMastodonPingToken(entry)
		if !ok {
			continue
		}
		key := strings.ToLower(normalized)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		tokens = append(tokens, normalized)
	}

	return tokens
}

// mastodonScanTokens finds the mentions and hashtags written into a message,
// recording each in seen so a caller can suppress duplicates.
func mastodonScanTokens(message string, seen map[string]struct{}) []string {
	tokens := []string{}
	for _, mention := range extractMastodonMentions(message) {
		key := strings.ToLower(mention)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		tokens = append(tokens, mention)
	}
	for _, hashtag := range extractMastodonHashtags(message) {
		key := strings.ToLower(hashtag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		tokens = append(tokens, hashtag)
	}

	return tokens
}

func mastodonPingPayload(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}

	return " " + strings.Join(tokens, " ")
}

// normalizeMastodonPingToken accepts an @mention or a #hashtag and rejects
// anything else, matching upstream's normalize_ping_token.
func normalizeMastodonPingToken(token string) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}

	if strings.HasPrefix(token, "@") {
		if normalized, ok := normalizeMastodonTarget(token); ok {
			return normalized, true
		}
		return "", false
	}

	if strings.HasPrefix(token, "#") {
		return normalizeMastodonHashtag(token)
	}

	return "", false
}

func normalizeMastodonHashtag(token string) (string, bool) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, "#") {
		return "", false
	}

	value := token[1:]
	if !mastodonHashtagPattern.MatchString(value) {
		return "", false
	}
	// A tag that is only digits once underscores are dropped is not one.
	if mastodonHashtagDigits.MatchString(strings.ReplaceAll(value, "_", "")) {
		return "", false
	}

	return token, true
}

func extractMastodonHashtags(message string) []string {
	matches := mastodonHashtagDetectPattern.FindAllStringSubmatch(message, -1)
	tags := make([]string, 0, len(matches))
	for _, match := range matches {
		if normalized, ok := normalizeMastodonHashtag(match[1]); ok {
			tags = append(tags, normalized)
		}
	}

	return tags
}
