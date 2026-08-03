package notify

import (
	"fmt"
	"net/url"
	"regexp"
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
	host      string
	port      int
	secure    bool
	user      string
	password  string
	color     bool
	prefix    bool
}

func NewNotificoTarget(target *ParsedURL) (*NotificoTarget, error) {
	segments := splitPath(target.Path)
	host := strings.TrimSpace(target.Host)

	var projectID, msgHook string
	var selfHosted bool

	// A numeric host is the project ID and means the official endpoint;
	// anything else is the hostname of a self-hosted instance.
	if notificoProjectID.MatchString(host) {
		projectID = host
		if len(segments) > 0 {
			msgHook = segments[0]
		}
	} else {
		selfHosted = true
		if len(segments) > 0 {
			projectID = segments[0]
		}
		if len(segments) > 1 {
			msgHook = segments[1]
		}
	}

	if raw := strings.TrimSpace(target.Query["project"]); raw != "" {
		projectID = raw
	}
	if raw := strings.TrimSpace(target.Query["token"]); raw != "" {
		msgHook = raw
	}

	if projectID == "" || msgHook == "" {
		return nil, fmt.Errorf("missing project or hook")
	}

	result := &NotificoTarget{
		projectID: projectID,
		msgHook:   msgHook,
		color:     parseBoolWithDefault(target.Query["color"], true),
		prefix:    parseBoolWithDefault(target.Query["prefix"], true),
	}

	if selfHosted {
		result.host = host
		result.port = target.Port
		result.secure = strings.EqualFold(target.Scheme, "notificos")
		result.user = strings.TrimSpace(target.User)
		result.password = target.Password
	}

	return result, nil
}

// notificoProjectID matches the numeric project ID that identifies the
// official endpoint; any other host is a self-hosted instance.
var notificoProjectID = regexp.MustCompile(`^[0-9]+$`)

func (n *NotificoTarget) notifyURL() string {
	if n.host == "" {
		return fmt.Sprintf(notificoURL, n.projectID, n.msgHook)
	}

	scheme := "http"
	if n.secure {
		scheme = "https"
	}

	base := fmt.Sprintf("%s://%s", scheme, n.host)
	if n.port > 0 {
		base += fmt.Sprintf(":%d", n.port)
	}

	return fmt.Sprintf("%s/h/%s/%s", base, n.projectID, n.msgHook)
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
	// Self-hosted instances may sit behind basic auth.
	if n.user != "" {
		headers["Authorization"] = basicAuthHeader(n.user, n.password)
	}

	return RequestSpec{
		Method:  "GET",
		URL:     n.notifyURL() + "?" + values.Encode(),
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
				"project": map[string]any{
					"alias_of": "project_id",
				},
				"token": map[string]any{
					"alias_of": "msghook",
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
			"templates": []string{"{schema}://{project_id}/{msghook}", "{schema}://{host}/{project_id}/{msghook}", "{schema}://{host}:{port}/{project_id}/{msghook}", "{schema}://{user}@{host}/{project_id}/{msghook}", "{schema}://{user}@{host}:{port}/{project_id}/{msghook}", "{schema}://{user}:{password}@{host}/{project_id}/{msghook}", "{schema}://{user}:{password}@{host}:{port}/{project_id}/{msghook}"},
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
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"password": map[string]any{
					"map_to":   "password",
					"name":     "Password",
					"private":  true,
					"required": false,
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
				"user": map[string]any{
					"map_to":   "user",
					"name":     "Username",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"notifico", "notificos"},
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
		"secure_protocols": []string{"notificos"},
		"service_name":     "Notifico",
		"service_url":      "https://n.tkte.ch",
		"setup_url":        "https://appriseit.com/services/notifico/",
	})
}
