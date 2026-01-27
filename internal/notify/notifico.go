package notify

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	notificoURL             = "https://n.tkte.ch/h/%s/%s"
	notificoAppID           = "Apprise"
	notificoColorTeal       = "\x0310"
	notificoColorOrange     = "\x0307"
	notificoColorRed        = "\x0304"
	notificoColorLightGreen = "\x0309"
	notificoColorReset      = "\x03"
	notificoFormatBold      = "\x02"
	notificoFormatReset     = "\x0f"
)

type NotificoTarget struct {
	projectID string
	msgHook   string
	color     bool
	prefix    bool
}

func NewNotificoTarget(target *ParsedURL) (*NotificoTarget, error) {
	projectID := target.Host
	segments := splitPath(target.Path)
	if projectID == "" || len(segments) == 0 {
		return nil, fmt.Errorf("missing project or hook")
	}

	color := parseBoolWithDefault(target.Query["color"], true)
	prefix := parseBoolWithDefault(target.Query["prefix"], true)

	return &NotificoTarget{
		projectID: projectID,
		msgHook:   segments[0],
		color:     color,
		prefix:    prefix,
	}, nil
}

func (n *NotificoTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	message := mergeTitleBody(title, body)
	payload := n.formatPayload(message, notifyType)

	values := url.Values{}
	values.Set("payload", payload)

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/x-www-form-urlencoded; charset=utf-8",
	}

	return RequestSpec{
		Method:  "GET",
		URL:     fmt.Sprintf(notificoURL, n.projectID, n.msgHook) + "?" + values.Encode(),
		Headers: headers,
		Body:    "",
	}, nil
}

func (n *NotificoTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := n.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (n *NotificoTarget) formatPayload(body string, notifyType NotifyType) string {
	color := ""
	token := "i"

	switch notifyType {
	case NotifyInfo:
		color = notificoColorTeal
		token = "i"
	case NotifySuccess:
		color = notificoColorLightGreen
		token = "✔"
	case NotifyWarning:
		color = notificoColorOrange
		token = "!"
	case NotifyFailure:
		color = notificoColorRed
		token = "✗"
	}

	if !n.color {
		color = ""
	}

	if !n.prefix {
		return body
	}

	var b strings.Builder
	if n.color {
		b.WriteString(color)
	}
	b.WriteString("[")
	b.WriteString(token)
	b.WriteString("]")
	if n.color {
		b.WriteString(notificoColorReset)
	}
	b.WriteString(" ")
	if n.color {
		b.WriteString(notificoFormatBold)
	}
	b.WriteString(notificoAppID)
	if n.color {
		b.WriteString(notificoFormatReset)
	}
	b.WriteString(": ")
	b.WriteString(body)
	if n.color {
		b.WriteString(notificoFormatReset)
	}
	return b.String()
}

func init() {
	RegisterSchemaEntryOrdered(42, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"color": map[string]any{
					"default":  true,
					"map_to":   "color",
					"name":     "IRC Colors",
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
				"prefix": map[string]any{
					"default":  true,
					"map_to":   "prefix",
					"name":     "Prefix",
					"private":  false,
					"required": false,
					"type":     "bool",
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
			"templates": []string{"{schema}://{project_id}/{msghook}"},
			"tokens": map[string]any{
				"msghook": map[string]any{
					"map_to":   "msghook",
					"name":     "Message Hook",
					"private":  true,
					"regex":    []string{"^[a-z0-9]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"project_id": map[string]any{
					"map_to":   "project_id",
					"name":     "Project ID",
					"private":  true,
					"regex":    []string{"^[0-9]+$", ""},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "notifico",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"notifico"},
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"notifico"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"notifico"},
		"service_name":     "Notifico",
		"service_url":      "https://n.tkte.ch",
		"setup_url":        "https://appriseit.com/services/notifico/",
	})
}
