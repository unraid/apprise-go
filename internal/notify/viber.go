package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	viberURL = "https://chatapi.viber.com/pa/send_message"

	// Viber truncates a longer sender name.
	viberSenderNameLimit = 28
)

type ViberTarget struct {
	token   string
	targets []string
	source  string
	avatar  string
}

func NewViberTarget(target *ParsedURL) (*ViberTarget, error) {
	entries := append([]string{strings.TrimSpace(target.Host)}, splitPath(target.Path)...)

	token := strings.TrimSpace(target.Query["token"])
	if token == "" {
		// Without ?token= the first entry is the token and the rest are
		// receiver IDs.
		if len(entries) == 0 || entries[0] == "" {
			return nil, fmt.Errorf("missing token")
		}
		token = entries[0]
		entries = entries[1:]
	}

	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		entries = append(entries, parseDelimitedList(to)...)
	}

	targets := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry = strings.TrimSpace(entry); entry != "" {
			targets = append(targets, entry)
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("missing receiver ids")
	}

	return &ViberTarget{
		token:   token,
		targets: targets,
		source:  strings.TrimSpace(target.Query["from"]),
		avatar:  strings.TrimSpace(target.Query["avatar"]),
	}, nil
}

func (v *ViberTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	specs, err := v.buildRequests(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}

	return specs[0], nil
}

func (v *ViberTarget) Send(body, title string, notifyType NotifyType) error {
	specs, err := v.buildRequests(body, title, notifyType)
	if err != nil {
		return err
	}

	for _, spec := range specs {
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

// buildRequests returns one request per receiver, since Viber addresses a
// single recipient per call.
func (v *ViberTarget) buildRequests(body, title string, notifyType NotifyType) ([]RequestSpec, error) {
	_ = notifyType

	sender := v.source
	if sender == "" {
		sender = pushoverDefaultAppDesc
	}
	if len(sender) > viberSenderNameLimit {
		sender = sender[:viberSenderNameLimit]
	}

	senderBlock := map[string]any{"name": sender}
	if v.avatar != "" {
		senderBlock["avatar"] = v.avatar
	}

	headers := map[string]string{
		"User-Agent":         "Apprise",
		"Accept":             "*/*",
		"Content-Type":       "application/json",
		"X-Viber-Auth-Token": v.token,
	}

	specs := make([]RequestSpec, 0, len(v.targets))
	for _, receiver := range v.targets {
		payload := map[string]any{
			"type":     "text",
			"text":     mergeTitleBody(title, body),
			"sender":   senderBlock,
			"receiver": receiver,
		}

		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		specs = append(specs, RequestSpec{
			Method:  "POST",
			URL:     viberURL,
			Headers: headers,
			Body:    string(data),
		})
	}

	return specs, nil
}

func init() {
	RegisterSchemaEntryOrdered(145, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"avatar": map[string]any{
					"map_to":   "avatar",
					"name":     "Bot Avatar URL",
					"private":  false,
					"required": false,
					"type":     "string",
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
				"from": map[string]any{
					"map_to":   "source",
					"name":     "Bot Name",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"optional": map[string]any{
					"default":  false,
					"map_to":   "optional",
					"name":     "Optional Service",
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
				"redirect": map[string]any{
					"default":  true,
					"map_to":   "redirect",
					"name":     "Follow Redirects",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"retry": map[string]any{
					"default":  0,
					"map_to":   "retry",
					"max":      10,
					"min":      0,
					"name":     "Service Retry",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"rto": map[string]any{
					"default":  4.0,
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
				"wait": map[string]any{
					"default":  0.0,
					"map_to":   "wait",
					"max":      20.0,
					"min":      0.0,
					"name":     "Inter-Retry Wait",
					"private":  false,
					"required": false,
					"type":     "float",
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{token}/{targets}"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "viber",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"viber"},
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []any{},
					"map_to":   "targets",
					"name":     "Receiver IDs",
					"private":  false,
					"required": true,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Authentication Token",
					"private":  true,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"viber"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"service_name": "Viber",
		"service_url":  "https://www.viber.com/",
		"setup_url":    "https://appriseit.com/services/viber/",
	})
}
