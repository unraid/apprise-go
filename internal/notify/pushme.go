package notify

import (
	"fmt"
	"net/url"
	"strings"
)

const pushMeURL = "https://push.i-i.me/"

type PushMeTarget struct {
	token  string
	status bool
}

func NewPushMeTarget(target *ParsedURL) (*PushMeTarget, error) {
	token := target.Host
	if rawToken, ok := target.Query["token"]; ok && rawToken != "" {
		token = rawToken
	} else if rawToken, ok := target.Query["push_key"]; ok && rawToken != "" {
		token = rawToken
	}
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	status := parseBoolWithDefault(target.Query["status"], true)

	return &PushMeTarget{
		token:  token,
		status: status,
	}, nil
}

func (p *PushMeTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	resolvedTitle := title
	if p.status {
		resolvedTitle = fmt.Sprintf("%s %s", notifyTypeASCII(notifyType), title)
	}

	query := buildQuery([]queryPair{
		{"push_key", p.token},
		{"title", resolvedTitle},
		{"content", body},
		{"type", "text"},
	})

	u, err := url.Parse(pushMeURL)
	if err != nil {
		return RequestSpec{}, err
	}
	u.RawQuery = query

	headers := map[string]string{
		"User-Agent": "Apprise",
		"Accept":     "*/*",
	}

	return RequestSpec{
		Method:  "POST",
		URL:     u.String(),
		Headers: headers,
		Body:    "",
	}, nil
}

func (p *PushMeTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := p.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func notifyTypeASCII(notifyType NotifyType) string {
	switch notifyType {
	case NotifyInfo:
		return "[i]"
	case NotifySuccess:
		return "[+]"
	case NotifyFailure:
		return "[!]"
	case NotifyWarning:
		return "[~]"
	default:
		return "[?]"
	}
}

func parseBoolWithDefault(raw string, fallback bool) bool {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "1", "true", "yes", "on", "y":
		return true
	case "0", "false", "no", "off", "n":
		return false
	case "":
		return fallback
	default:
		return fallback
	}
}

type queryPair struct {
	key   string
	value string
}

func buildQuery(pairs []queryPair) string {
	parts := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		parts = append(parts, url.QueryEscape(pair.key)+"="+url.QueryEscape(pair.value))
	}
	return strings.Join(parts, "&")
}

func init() {
	RegisterSchemaEntryOrdered(82, SchemaEntry{
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
				"overflow": map[string]any{
					"default":  "upstream",
					"map_to":   "overflow",
					"name":     "Overflow Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"split", "truncate", "upstream"},
				},
				"push_key": map[string]any{
					"alias_of": "token",
				},
				"rto": map[string]any{
					"default":  4,
					"map_to":   "rto",
					"name":     "Socket Read Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"status": map[string]any{
					"default":  true,
					"map_to":   "status",
					"name":     "Show Status",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"store": map[string]any{
					"default":  true,
					"map_to":   "store",
					"name":     "Persistent Storage",
					"private":  false,
					"required": false,
					"type":     "bool",
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
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{token}"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "pushme",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"pushme"},
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Token",
					"private":  true,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"pushme"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": nil,
		"service_name":     "PushMe",
		"service_url":      "https://push.i-i.me/",
		"setup_url":        "https://appriseit.com/services/pushme/",
	})
}
