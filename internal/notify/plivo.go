package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const plivoURL = "https://api.plivo.com/v1/Account/{auth_id}/Message/"
const plivoBatchSize = 20

type PlivoTarget struct {
	authID  string
	token   string
	source  string
	targets []string
	batch   bool
}

func NewPlivoTarget(target *ParsedURL) (*PlivoTarget, error) {
	authID := strings.TrimSpace(target.User)
	if raw := strings.TrimSpace(target.Query["id"]); raw != "" {
		authID = raw
	}

	targets := splitPath(target.Path)

	token := strings.TrimSpace(target.Host)
	if raw := strings.TrimSpace(target.Query["token"]); raw != "" {
		token = raw
		if host := strings.TrimSpace(target.Host); host != "" {
			targets = append([]string{host}, targets...)
		}
	}

	if authID == "" || token == "" {
		return nil, fmt.Errorf("missing credentials")
	}

	sourceRaw := strings.TrimSpace(target.Query["from"])
	if sourceRaw == "" {
		if len(targets) > 0 {
			sourceRaw = targets[0]
			targets = targets[1:]
		}
	}

	if sourceRaw == "" {
		return nil, fmt.Errorf("missing source")
	}
	sourceDigits, ok := normalizePhone(sourceRaw)
	if !ok {
		return nil, fmt.Errorf("invalid source")
	}
	source := "+" + sourceDigits

	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		for _, entry := range parseDelimitedList(toValue) {
			targets = append(targets, entry)
		}
	}

	normalizedTargets := []string{}
	for _, entry := range targets {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if normalized, ok := normalizePhone(entry); ok {
			normalizedTargets = append(normalizedTargets, "+"+normalized)
		}
	}

	if len(normalizedTargets) == 0 {
		normalizedTargets = []string{source}
	}

	batch := parseBoolWithDefault(target.Query["batch"], false)

	return &PlivoTarget{
		authID:  authID,
		token:   token,
		source:  source,
		targets: normalizedTargets,
		batch:   batch,
	}, nil
}

func (p *PlivoTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(p.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	batchSize := 1
	if p.batch {
		batchSize = plivoBatchSize
	}

	recipients := strings.Join(p.targets[:minInt(len(p.targets), batchSize)], ",")
	payload := map[string]any{
		"src":        p.source,
		"dst":        nil,
		"text":       message,
		"recipients": recipients,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	requestURL := plivoURL

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    requestURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
			"Authorization": basicAuthHeader(
				p.authID,
				p.token,
			),
		},
		Body: string(data),
	}, nil
}

func (p *PlivoTarget) Send(body, title string, notifyType NotifyType) error {
	if len(p.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	batchSize := 1
	if p.batch {
		batchSize = plivoBatchSize
	}

	requestURL := plivoURL

	for index := 0; index < len(p.targets); index += batchSize {
		end := index + batchSize
		if end > len(p.targets) {
			end = len(p.targets)
		}

		payload := map[string]any{
			"src":        p.source,
			"dst":        nil,
			"text":       message,
			"recipients": strings.Join(p.targets[index:end], ","),
		}

		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		spec := RequestSpec{
			Method: "POST",
			URL:    requestURL,
			Headers: map[string]string{
				"User-Agent":   "Apprise",
				"Accept":       "*/*",
				"Content-Type": "application/json",
				"Authorization": basicAuthHeader(
					p.authID,
					p.token,
				),
			},
			Body: string(data),
		}

		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = notifyType

	return nil
}

func init() {
	RegisterSchemaEntryOrdered(99, SchemaEntry{
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
				"from": map[string]any{
					"alias_of": "source",
				},
				"id": map[string]any{
					"alias_of": "auth_id",
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
			"templates": []string{"{schema}://{auth_id}@{token}/{source}", "{schema}://{auth_id}@{token}/{source}/{targets}"},
			"tokens": map[string]any{
				"auth_id": map[string]any{
					"map_to":   "auth_id",
					"name":     "Auth ID",
					"private":  false,
					"regex":    []string{"^[a-z0-9]{20,30}$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "plivo",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"plivo"},
				},
				"source": map[string]any{
					"map_to":   "source",
					"name":     "Source Phone No",
					"prefix":   "+",
					"private":  false,
					"regex":    []string{"^[0-9\\s)(+-]+$", "i"},
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
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Auth Token",
					"private":  false,
					"regex":    []string{"^[a-z0-9]{30,50}$", "i"},
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
		"secure_protocols": []string{"plivo"},
		"service_name":     "Plivo",
		"service_url":      "https://plivo.com",
		"setup_url":        "https://appriseit.com/services/plivo/",
	})
}
