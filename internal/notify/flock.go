package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	flockWebhookURL = "https://api.flock.com/hooks/sendMessage"
	flockAPIURL     = "https://api.flock.co/v1/chat.sendMessage"
)

var flockTokenRe = regexp.MustCompile(`(?i)^[a-z0-9-]+$`)
var flockUserRe = regexp.MustCompile(`(?i)^(?:@|u:)?([A-Z0-9_]+)$`)
var flockChannelRe = regexp.MustCompile(`(?i)^(?:#|g:)([A-Z0-9_]+)$`)

type FlockTarget struct {
	token        string
	botname      string
	includeImage bool
	targets      []string
}

func NewFlockTarget(target *ParsedURL) (*FlockTarget, error) {
	token := strings.TrimSpace(target.Host)
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}
	if !flockTokenRe.MatchString(token) {
		return nil, fmt.Errorf("invalid token")
	}

	includeImage := parseBoolWithDefault(target.Query["image"], true)
	botname := strings.TrimSpace(target.User)

	entries := []string{}
	entries = append(entries, splitPath(target.Path)...)
	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		entries = append(entries, parseDelimitedList(toValue)...)
	}

	normalizedEntries := normalizeFlockEntries(entries)
	targets := []string{}
	for _, entry := range normalizedEntries {
		if user := flockUserRe.FindStringSubmatch(entry); user != nil {
			targets = append(targets, "u:"+user[1])
			continue
		}
		if channel := flockChannelRe.FindStringSubmatch(entry); channel != nil {
			targets = append(targets, "g:"+channel[1])
			continue
		}
	}

	if len(entries) > 0 && len(targets) == 0 {
		return nil, fmt.Errorf("invalid targets")
	}

	return &FlockTarget{
		token:        token,
		botname:      botname,
		includeImage: includeImage,
		targets:      targets,
	}, nil
}

func (f *FlockTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(f.targets) == 0 {
		return f.buildRequest(body, title, notifyType, "")
	}
	return f.buildRequest(body, title, notifyType, f.targets[0])
}

func (f *FlockTarget) Send(body, title string, notifyType NotifyType) error {
	if len(f.targets) == 0 {
		spec, err := f.buildRequest(body, title, notifyType, "")
		if err != nil {
			return err
		}
		return SendRequest(spec)
	}

	for _, target := range f.targets {
		spec, err := f.buildRequest(body, title, notifyType, target)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func (f *FlockTarget) buildRequest(body, title string, notifyType NotifyType, target string) (RequestSpec, error) {
	payload := map[string]any{
		"token":   f.token,
		"flockml": f.buildFlockML(title, body),
		"sendAs": map[string]any{
			"name":         f.displayName(),
			"profileImage": f.profileImage(notifyType),
		},
	}

	url := flockWebhookURL + "/" + f.token
	if target != "" {
		payload["to"] = target
		url = flockAPIURL
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    url,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/json",
		},
		Body: string(data),
	}, nil
}

func (f *FlockTarget) displayName() string {
	if f.botname != "" {
		return f.botname
	}
	return "Apprise"
}

func (f *FlockTarget) profileImage(notifyType NotifyType) any {
	if !f.includeImage {
		return nil
	}
	return appriseImageURL(notifyType, "72x72")
}

func (f *FlockTarget) buildFlockML(title, body string) string {
	escapedTitle := flockEscapeHTML(title)
	escapedBody := flockEscapeHTML(body)
	if escapedTitle != "" {
		escapedTitle = "<b>" + escapedTitle + "</b><br/>"
	}
	return "<flockml>" + escapedTitle + escapedBody + "</flockml>"
}

func flockEscapeHTML(value string) string {
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}

func normalizeFlockEntries(entries []string) []string {
	if len(entries) == 0 {
		return nil
	}
	unique := map[string]struct{}{}
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		unique[entry] = struct{}{}
	}
	if len(unique) == 0 {
		return nil
	}
	sorted := make([]string, 0, len(unique))
	for entry := range unique {
		sorted = append(sorted, entry)
	}
	sort.Strings(sorted)
	return sorted
}

func init() {
	RegisterSchemaEntryOrdered(77, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
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
				"image": map[string]any{
					"default":  true,
					"map_to":   "include_image",
					"name":     "Include Image",
					"private":  false,
					"required": false,
					"type":     "bool",
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
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{token}", "{schema}://{botname}@{token}", "{schema}://{botname}@{token}/{targets}", "{schema}://{token}/{targets}"},
			"tokens": map[string]any{
				"botname": map[string]any{
					"map_to":   "user",
					"name":     "Bot Name",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "flock",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"flock"},
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"to_channel", "to_user"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"to_channel": map[string]any{
					"map_to":   "targets",
					"name":     "To Channel ID",
					"prefix":   "#",
					"private":  false,
					"regex":    []string{"^[A-Z0-9_]+$", "i"},
					"required": false,
					"type":     "string",
				},
				"to_user": map[string]any{
					"map_to":   "targets",
					"name":     "To User ID",
					"prefix":   "@",
					"private":  false,
					"regex":    []string{"^[A-Z0-9_]+$", "i"},
					"required": false,
					"type":     "string",
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Access Key",
					"private":  true,
					"regex":    []string{"^[a-z0-9-]+$", "i"},
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": nil,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"flock"},
		"service_name":     "Flock",
		"service_url":      "https://flock.com/",
		"setup_url":        "https://appriseit.com/services/flock/",
	})
}
