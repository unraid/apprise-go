package notify

import (
	"fmt"
	"strings"
)

const signalgridURL = "https://api.signalgrid.co/v1/push"

var signalgridTypeNames = map[NotifyType]string{
	NotifyInfo:    "INFO",
	NotifySuccess: "SUCCESS",
	NotifyWarning: "WARN",
	NotifyFailure: "CRIT",
}

type SignalgridTarget struct {
	clientKey string
	targets   []string
	critical  bool
}

func NewSignalgridTarget(target *ParsedURL) (*SignalgridTarget, error) {
	clientKey := strings.TrimSpace(target.Query["client_key"])
	if clientKey == "" {
		clientKey = strings.TrimSpace(target.Host)
	}
	if clientKey == "" {
		return nil, fmt.Errorf("invalid Signalgrid client key")
	}

	targets := splitPath(target.Path)
	if rawTargets := strings.TrimSpace(target.Query["to"]); rawTargets != "" {
		targets = append(targets, parseDelimitedList(rawTargets)...)
	}

	return &SignalgridTarget{
		clientKey: clientKey,
		targets:   targets,
		critical:  parseBoolWithDefault(target.Query["critical"], false),
	}, nil
}

func (s *SignalgridTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(s.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("no Signalgrid channels")
	}

	return s.buildRequest(body, title, notifyType, s.targets[0]), nil
}

func (s *SignalgridTarget) buildRequest(body, title string, notifyType NotifyType, channel string) RequestSpec {
	values := formFields{}
	values.Set("client_key", s.clientKey)
	values.Set("channel", channel)
	values.Set("title", title)
	values.Set("body", body)
	values.Set("type", signalgridTypeName(notifyType))
	if s.critical {
		values.Set("critical", "true")
	} else {
		values.Set("critical", "false")
	}

	return RequestSpec{
		Method: "POST",
		URL:    signalgridURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/x-www-form-urlencoded",
		},
		Body: values.Encode(),
	}
}

func signalgridTypeName(notifyType NotifyType) string {
	if name, ok := signalgridTypeNames[notifyType]; ok {
		return name
	}
	return "INFO"
}

func (s *SignalgridTarget) Send(body, title string, notifyType NotifyType) error {
	if len(s.targets) == 0 {
		return fmt.Errorf("no Signalgrid channels")
	}

	var outcome sendOutcome
	for _, channel := range s.targets {
		outcome.record(SendRequest(s.buildRequest(body, title, notifyType, channel)))
	}

	return outcome.err()
}

// Signalgrid has no attachment API. Upstream logs and drops attachments, so
// the attachment-aware entry point preserves that behavior while still using
// the ordinary per-channel send loop.
func (s *SignalgridTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	_ = attachments
	return s.Send(body, title, notifyType)
}

func init() {
	RegisterSchemaEntryOrdered(168, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"client_key": map[string]any{
					"alias_of": "client_key",
				},
				"critical": map[string]any{
					"default":  false,
					"map_to":   "critical",
					"name":     "Critical Notification",
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
			"templates": []string{"{schema}://{client_key}/{targets}"},
			"tokens": map[string]any{
				"client_key": map[string]any{
					"map_to":   "client_key",
					"name":     "Client Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "signalgrid",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"signalgrid"},
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{},
					"map_to":   "targets",
					"name":     "Channels",
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
		"secure_protocols": []string{"signalgrid"},
		"service_name":     "Signalgrid",
		"service_url":      "https://signalgrid.co/",
		"setup_url":        "https://docs.signalgrid.co/integrations/apprise/",
	})
}
