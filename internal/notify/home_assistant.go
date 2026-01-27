package notify

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const homeAssistantDefaultPort = 8123

type HomeAssistantTarget struct {
	host        string
	port        int
	secure      bool
	user        string
	password    string
	accessToken string
	nid         string
	fullpath    string
}

func NewHomeAssistantTarget(target *ParsedURL) (*HomeAssistantTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	secure := strings.EqualFold(target.Scheme, "hassios")
	port := target.Port
	if port == 0 && !secure {
		port = homeAssistantDefaultPort
	}

	accessToken := strings.TrimSpace(target.Query["accesstoken"])
	fullpath := ""
	if accessToken == "" {
		parts := splitPath(target.Path)
		if len(parts) == 0 {
			return nil, fmt.Errorf("missing access token")
		}
		accessToken = parts[len(parts)-1]
		parts = parts[:len(parts)-1]
		if len(parts) > 0 {
			fullpath = "/" + strings.Join(parts, "/")
		}
	}
	if accessToken == "" {
		return nil, fmt.Errorf("missing access token")
	}

	nid := strings.TrimSpace(target.Query["nid"])

	return &HomeAssistantTarget{
		host:        host,
		port:        port,
		secure:      secure,
		user:        strings.TrimSpace(target.User),
		password:    target.Password,
		accessToken: accessToken,
		nid:         nid,
		fullpath:    fullpath,
	}, nil
}

func (h *HomeAssistantTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload := map[string]any{
		"title":           title,
		"message":         body,
		"notification_id": h.notificationID(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + h.accessToken,
	}
	if h.user != "" {
		password := h.password
		if password == "" {
			password = "None"
		}
		headers["Authorization"] = basicAuthHeader(h.user, password)
	}

	_ = notifyType

	return RequestSpec{
		Method:  "POST",
		URL:     h.buildURL(),
		Headers: headers,
		Body:    string(data),
	}, nil
}

func (h *HomeAssistantTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := h.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (h *HomeAssistantTarget) buildURL() string {
	scheme := "http"
	if h.secure {
		scheme = "https"
	}

	base := fmt.Sprintf("%s://%s", scheme, h.host)
	if h.port > 0 {
		base += fmt.Sprintf(":%d", h.port)
	}

	path := strings.TrimRight(h.fullpath, "/")
	return base + path + "/api/services/persistent_notification/create"
}

func (h *HomeAssistantTarget) notificationID() string {
	if h.nid != "" {
		return h.nid
	}
	return newUUIDv4()
}

func newUUIDv4() string {
	if strings.TrimSpace(os.Getenv("APPRISE_FIXED_TIME")) != "" {
		return "00000000-0000-4000-8000-000000000000"
	}

	buf := make([]byte, 16)
	_, err := rand.Read(buf)
	if err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(buf[0:4]),
		hex.EncodeToString(buf[4:6]),
		hex.EncodeToString(buf[6:8]),
		hex.EncodeToString(buf[8:10]),
		hex.EncodeToString(buf[10:16]),
	)
}

func init() {
	RegisterSchemaEntryOrdered(124, SchemaEntry{
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
				"nid": map[string]any{
					"map_to":   "nid",
					"name":     "Notification ID",
					"private":  false,
					"regex":    []string{"^[a-z0-9_-]+$", "i"},
					"required": false,
					"type":     "string",
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
			"templates": []string{"{schema}://{host}/{accesstoken}", "{schema}://{host}:{port}/{accesstoken}", "{schema}://{user}@{host}/{accesstoken}", "{schema}://{user}@{host}:{port}/{accesstoken}", "{schema}://{user}:{password}@{host}/{accesstoken}", "{schema}://{user}:{password}@{host}:{port}/{accesstoken}"},
			"tokens": map[string]any{
				"accesstoken": map[string]any{
					"map_to":   "accesstoken",
					"name":     "Long-Lived Access Token",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": true,
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
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"hassio", "hassios"},
				},
				"user": map[string]any{
					"map_to":   "user",
					"name":     "Username",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"hassio"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"hassios"},
		"service_name":     "HomeAssistant",
		"service_url":      "https://www.home-assistant.io/",
		"setup_url":        "https://appriseit.com/services/homeassistant/",
	})
}
