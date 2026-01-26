package notify

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const vonageURL = "https://rest.nexmo.com/sms/json"
const vonageDefaultTTL = 900000
const vonageMinTTL = 20000
const vonageMaxTTL = 604800000

type VonageTarget struct {
	apiKey  string
	secret  string
	source  string
	ttl     int
	targets []string
}

func NewVonageTarget(target *ParsedURL) (*VonageTarget, error) {
	apiKey := strings.TrimSpace(target.User)
	secret := target.Password
	if raw := strings.TrimSpace(target.Query["key"]); raw != "" {
		apiKey = raw
	}
	if raw := strings.TrimSpace(target.Query["secret"]); raw != "" {
		secret = raw
	}
	if apiKey == "" || secret == "" {
		return nil, fmt.Errorf("missing credentials")
	}

	ttl := vonageDefaultTTL
	if raw := strings.TrimSpace(target.Query["ttl"]); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			ttl = value
		}
	}
	if ttl < vonageMinTTL || ttl > vonageMaxTTL {
		return nil, fmt.Errorf("invalid ttl")
	}

	sourceRaw := strings.TrimSpace(target.Host)
	if raw := strings.TrimSpace(target.Query["from"]); raw != "" {
		sourceRaw = raw
	} else if raw := strings.TrimSpace(target.Query["source"]); raw != "" {
		sourceRaw = raw
	}
	if sourceRaw == "" {
		return nil, fmt.Errorf("missing source")
	}
	source, ok := normalizePhone(sourceRaw)
	if !ok {
		return nil, fmt.Errorf("invalid source")
	}

	targets := []string{}
	appendTarget := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if normalized, ok := normalizePhone(raw); ok {
			targets = append(targets, normalized)
		}
	}

	for _, entry := range splitPath(target.Path) {
		appendTarget(entry)
	}
	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		for _, entry := range parseDelimitedList(toValue) {
			appendTarget(entry)
		}
	}

	return &VonageTarget{
		apiKey:  apiKey,
		secret:  secret,
		source:  source,
		ttl:     ttl,
		targets: targets,
	}, nil
}

func (v *VonageTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	targets := v.targets
	if len(targets) == 0 {
		targets = []string{v.source}
	}
	if len(targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)

	values := url.Values{}
	values.Set("api_key", v.apiKey)
	values.Set("api_secret", v.secret)
	values.Set("ttl", strconv.Itoa(v.ttl))
	values.Set("from", v.source)
	values.Set("text", message)
	values.Set("to", targets[0])

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    vonageURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/x-www-form-urlencoded",
		},
		Body: values.Encode(),
	}, nil
}

func (v *VonageTarget) Send(body, title string, notifyType NotifyType) error {
	targets := v.targets
	if len(targets) == 0 {
		targets = []string{v.source}
	}
	if len(targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)

	for _, target := range targets {
		values := url.Values{}
		values.Set("api_key", v.apiKey)
		values.Set("api_secret", v.secret)
		values.Set("ttl", strconv.Itoa(v.ttl))
		values.Set("from", v.source)
		values.Set("text", message)
		values.Set("to", target)

		spec := RequestSpec{
			Method: "POST",
			URL:    vonageURL,
			Headers: map[string]string{
				"User-Agent":   "Apprise",
				"Accept":       "*/*",
				"Content-Type": "application/x-www-form-urlencoded",
			},
			Body: values.Encode(),
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = notifyType

	return nil
}

func init() {
	RegisterSchemaEntryOrdered(53, SchemaEntry{
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
				"from": map[string]any{
					"alias_of": "from_phone",
				},
				"key": map[string]any{
					"alias_of": "apikey",
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
				"secret": map[string]any{
					"alias_of": "secret",
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
				"ttl": map[string]any{
					"default":  900000,
					"map_to":   "ttl",
					"max":      604800000,
					"min":      20000,
					"name":     "ttl",
					"private":  false,
					"required": false,
					"type":     "int",
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
			"templates": []string{"{schema}://{apikey}:{secret}@{from_phone}", "{schema}://{apikey}:{secret}@{from_phone}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"regex":    []string{"^[a-z0-9]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"from_phone": map[string]any{
					"map_to":   "source",
					"name":     "From Phone No",
					"private":  false,
					"regex":    []string{"^\\+?[0-9\\s)(+-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"nexmo", "vonage"},
				},
				"secret": map[string]any{
					"map_to":   "secret",
					"name":     "API Secret",
					"private":  true,
					"regex":    []string{"^[a-z0-9]+$", "i"},
					"required": true,
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
					"group":    []string{"target_phone"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
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
		"secure_protocols": []string{"vonage", "nexmo"},
		"service_name":     "Vonage",
		"service_url":      "https://dashboard.nexmo.com/",
		"setup_url":        "https://appriseit.com/services/vonage/",
	})
}
