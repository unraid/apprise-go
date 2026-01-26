package notify

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

const popcornURL = "https://popcornnotify.com/notify"

var popcornKeyPattern = regexp.MustCompile(`(?i)^[a-z0-9]+$`)

var popcornListDelimiters = regexp.MustCompile(`[\[\];,\s]+`)

type PopcornTarget struct {
	apiKey  string
	batch   bool
	targets []string
}

func NewPopcornTarget(target *ParsedURL) (*PopcornTarget, error) {
	apiKey := strings.TrimSpace(target.Host)
	if apiKey == "" {
		return nil, fmt.Errorf("missing apikey")
	}
	if !popcornKeyPattern.MatchString(apiKey) {
		return nil, fmt.Errorf("invalid apikey")
	}

	targets := splitPath(target.Path)
	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		targets = append(targets, parsePopcornList(toValue)...)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	batch := parseBool(target.Query["batch"], false)

	return &PopcornTarget{
		apiKey:  apiKey,
		batch:   batch,
		targets: targets,
	}, nil
}

func (p *PopcornTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(p.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	batchSize := 1
	if p.batch {
		batchSize = 10
	}
	if batchSize > len(p.targets) {
		batchSize = len(p.targets)
	}

	recipients := strings.Join(p.targets[:batchSize], ",")

	values := url.Values{}
	values.Set("message", body)
	values.Set("subject", title)
	values.Set("recipients", recipients)

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    popcornURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Accept":        "*/*",
			"Content-Type":  "application/x-www-form-urlencoded",
			"Authorization": basicAuthHeader(p.apiKey, "None"),
		},
		Body: values.Encode(),
	}, nil
}

func (p *PopcornTarget) Send(body, title string, notifyType NotifyType) error {
	if len(p.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	batchSize := 1
	if p.batch {
		batchSize = 10
	}

	for start := 0; start < len(p.targets); start += batchSize {
		end := start + batchSize
		if end > len(p.targets) {
			end = len(p.targets)
		}

		values := url.Values{}
		values.Set("message", body)
		values.Set("subject", title)
		values.Set("recipients", strings.Join(p.targets[start:end], ","))

		spec := RequestSpec{
			Method: "POST",
			URL:    popcornURL,
			Headers: map[string]string{
				"User-Agent":    "Apprise",
				"Accept":        "*/*",
				"Content-Type":  "application/x-www-form-urlencoded",
				"Authorization": basicAuthHeader(p.apiKey, "None"),
			},
			Body: values.Encode(),
		}

		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func parsePopcornList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := popcornListDelimiters.Split(raw, -1)
	values := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values[part] = struct{}{}
	}

	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func init() {
	RegisterSchemaEntryOrdered(68, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"batch": map[string]any{
					"default":  false,
					"map_to":   "batch",
					"name":     "Batch Mode",
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
			"templates": []string{"{schema}://{apikey}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  false,
					"regex":    []string{"^[a-z0-9]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "popcorn",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"popcorn"},
				},
				"target_email": map[string]any{
					"map_to":   "targets",
					"name":     "Target Email",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_phone": map[string]any{
					"map_to":   "targets",
					"name":     "Target Phone No",
					"prefix":   "+",
					"private":  false,
					"regex":    []string{"^[0-9\\s)(+-]+$", "i"},
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_email", "target_phone"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": true,
					"type":     "list:string",
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
		"secure_protocols": []string{"popcorn"},
		"service_name":     "PopcornNotify",
		"service_url":      "https://popcornnotify.com/",
		"setup_url":        "https://appriseit.com/services/popcornnotify/",
	})
}
