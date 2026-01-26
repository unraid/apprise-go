package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const misskeyDefaultVisibility = "public"

var misskeyVisibilities = []string{
	"public",
	"home",
	"followers",
	"specified",
}

type MisskeyTarget struct {
	host       string
	port       int
	secure     bool
	token      string
	visibility string
}

func NewMisskeyTarget(target *ParsedURL) (*MisskeyTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	token := strings.TrimSpace(target.Query["token"])
	if token == "" {
		token = strings.TrimSpace(target.Password)
	}
	if token == "" && strings.TrimSpace(target.Password) == "" {
		token = strings.TrimSpace(target.User)
	}
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	visibility := misskeyDefaultVisibility
	if raw := strings.TrimSpace(target.Query["visibility"]); raw != "" {
		normalized, ok := normalizeMisskeyVisibility(raw)
		if !ok {
			return nil, fmt.Errorf("invalid visibility")
		}
		visibility = normalized
	}

	return &MisskeyTarget{
		host:       host,
		port:       target.Port,
		secure:     strings.EqualFold(target.Scheme, "misskeys"),
		token:      token,
		visibility: visibility,
	}, nil
}

func (m *MisskeyTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	message := mergeTitleBody(title, body)
	payload := map[string]string{
		"i":          m.token,
		"text":       message,
		"visibility": m.visibility,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    m.baseURL() + "/api/notes/create",
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/json",
		},
		Body: string(data),
	}, nil
}

func (m *MisskeyTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := m.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}
	return SendRequest(spec)
}

func (m *MisskeyTarget) baseURL() string {
	scheme := "http"
	if m.secure {
		scheme = "https"
	}

	base := fmt.Sprintf("%s://%s", scheme, m.host)
	if m.port > 0 {
		base += fmt.Sprintf(":%d", m.port)
	}
	return base
}

func normalizeMisskeyVisibility(raw string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return misskeyDefaultVisibility, true
	}

	for _, visibility := range misskeyVisibilities {
		if strings.HasPrefix(visibility, value) {
			return visibility, true
		}
	}

	return "", false
}

func init() {
	RegisterSchemaEntryOrdered(98, SchemaEntry{
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
				"visibility": map[string]any{
					"default":  "public",
					"map_to":   "visibility",
					"name":     "Visibility",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"public", "home", "followers", "specified"},
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{token}@{host}", "{schema}://{token}@{host}:{port}"},
			"tokens": map[string]any{
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": true,
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
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"misskey", "misskeys"},
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Access Token",
					"private":  false,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"misskey"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"misskeys"},
		"service_name":     "Misskey",
		"service_url":      "https://misskey-hub.net/",
		"setup_url":        "https://appriseit.com/services/misskey/",
	})
}
