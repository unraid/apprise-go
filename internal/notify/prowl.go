package notify

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	prowlURL          = "https://api.prowlapp.com/publicapi/add"
	prowlAppID        = "Apprise"
	prowlDefaultLevel = 0
)

var prowlPriorityOrder = []struct {
	prefix string
	value  int
}{
	{"-2", -2},
	{"-1", -1},
	{"0", 0},
	{"1", 1},
	{"2", 2},
	{"l", -2},
	{"m", -1},
	{"n", 0},
	{"h", 1},
	{"e", 2},
}

type ProwlTarget struct {
	apiKey      string
	providerKey string
	priority    int
}

func NewProwlTarget(target *ParsedURL) (*ProwlTarget, error) {
	apiKey := target.Host
	if apiKey == "" {
		return nil, fmt.Errorf("missing api key")
	}

	providerKey := ""
	segments := splitPath(target.Path)
	if len(segments) > 0 {
		providerKey = segments[0]
	}

	priority := prowlDefaultLevel
	if rawPriority, ok := target.Query["priority"]; ok && rawPriority != "" {
		priority = parseProwlPriority(rawPriority, priority)
	}

	return &ProwlTarget{
		apiKey:      apiKey,
		providerKey: providerKey,
		priority:    priority,
	}, nil
}

func (p *ProwlTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	values := url.Values{}
	values.Set("apikey", p.apiKey)
	values.Set("application", prowlAppID)
	values.Set("event", title)
	values.Set("description", body)
	values.Set("priority", fmt.Sprintf("%d", p.priority))
	if p.providerKey != "" {
		values.Set("providerkey", p.providerKey)
	}

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-type": "application/x-www-form-urlencoded",
	}

	_ = notifyType

	return RequestSpec{
		Method:  "POST",
		URL:     prowlURL,
		Headers: headers,
		Body:    values.Encode(),
	}, nil
}

func (p *ProwlTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := p.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func parseProwlPriority(raw string, fallback int) int {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	for _, entry := range prowlPriorityOrder {
		if strings.HasPrefix(normalized, entry.prefix) {
			return entry.value
		}
	}

	return fallback
}

func init() {
	RegisterSchemaEntryOrdered(8, SchemaEntry{
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
				"priority": map[string]any{
					"default":  0,
					"map_to":   "priority",
					"name":     "Priority",
					"private":  false,
					"required": false,
					"type":     "choice:int",
					"values":   []any{-2, -1, 0, 1, 2},
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
			"templates": []string{"{schema}://{apikey}", "{schema}://{apikey}/{providerkey}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"regex":    []string{"^[A-Za-z0-9]{40}$", "i"},
					"required": true,
					"type":     "string",
				},
				"providerkey": map[string]any{
					"map_to":   "providerkey",
					"name":     "Provider Key",
					"private":  true,
					"regex":    []string{"^[A-Za-z0-9]{40}$", "i"},
					"required": false,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "prowl",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"prowl"},
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
		"secure_protocols": []string{"prowl"},
		"service_name":     "Prowl",
		"service_url":      "https://www.prowlapp.com/",
		"setup_url":        "https://appriseit.com/services/prowl/",
	})
}
