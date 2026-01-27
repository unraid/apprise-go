package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

type MattermostTarget struct {
	host         string
	port         int
	secure       bool
	fullPath     string
	token        string
	username     string
	includeImage bool
	channels     []string
}

func NewMattermostTarget(target *ParsedURL) (*MattermostTarget, error) {
	if strings.TrimSpace(target.Host) == "" {
		return nil, fmt.Errorf("missing host")
	}

	segments := splitPath(target.Path)
	if len(segments) == 0 {
		return nil, fmt.Errorf("missing token")
	}
	token := segments[len(segments)-1]
	fullPath := ""
	if len(segments) > 1 {
		fullPath = "/" + strings.Join(segments[:len(segments)-1], "/")
	}

	channels := []string{}
	if channelValue, ok := target.Query["channels"]; ok && strings.TrimSpace(channelValue) != "" {
		channels = append(channels, parseDelimitedList(channelValue)...)
	}
	if channelValue, ok := target.Query["channel"]; ok && strings.TrimSpace(channelValue) != "" {
		channels = append(channels, parseDelimitedList(channelValue)...)
	}
	if channelValue, ok := target.Query["to"]; ok && strings.TrimSpace(channelValue) != "" {
		channels = append(channels, parseDelimitedList(channelValue)...)
	}

	return &MattermostTarget{
		host:         target.Host,
		port:         target.Port,
		secure:       target.Scheme == "mmosts",
		fullPath:     fullPath,
		token:        token,
		username:     strings.TrimSpace(target.User),
		includeImage: parseBool(target.Query["image"], true),
		channels:     channels,
	}, nil
}

func (m *MattermostTarget) Send(body, title string, notifyType NotifyType) error {
	message := mergeTitleBody(title, body)
	if len(m.channels) == 0 {
		spec, err := m.buildSpec(message, notifyType, "")
		if err != nil {
			return err
		}
		return SendRequest(spec)
	}

	for _, channel := range m.channels {
		spec, err := m.buildSpec(message, notifyType, channel)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func (m *MattermostTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	message := mergeTitleBody(title, body)
	channel := ""
	if len(m.channels) > 0 {
		channel = m.channels[0]
	}
	return m.buildSpec(message, notifyType, channel)
}

func (m *MattermostTarget) buildSpec(message string, notifyType NotifyType, channel string) (RequestSpec, error) {
	payload := map[string]any{
		"text":     message,
		"icon_url": nil,
	}

	if m.includeImage {
		payload["icon_url"] = appriseImageURL(notifyType, "72x72")
	}

	username := m.username
	if username == "" {
		username = "Apprise"
	}
	payload["username"] = username

	if channel != "" {
		payload["channel"] = strings.TrimPrefix(channel, "#")
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	scheme := "http"
	if m.secure {
		scheme = "https"
	}
	host := m.host
	if m.port != 0 {
		host = fmt.Sprintf("%s:%d", host, m.port)
	}

	path := strings.TrimRight(m.fullPath, "/")
	url := fmt.Sprintf("%s://%s%s/hooks/%s", scheme, host, path, m.token)

	return RequestSpec{
		Method: "POST",
		URL:    url,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
		},
		Body: string(data),
	}, nil
}

func init() {
	RegisterSchemaEntryOrdered(43, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"channel": map[string]any{
					"alias_of": "channels",
				},
				"channels": map[string]any{
					"delim":    []string{",", " "},
					"group":    []any{},
					"map_to":   "channels",
					"name":     "Channels",
					"private":  false,
					"required": false,
					"type":     "list:string",
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
				"image": map[string]any{
					"default":  true,
					"map_to":   "include_image",
					"name":     "Include Image",
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
					"alias_of": "channels",
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
			"templates": []string{"{schema}://{host}/{token}", "{schema}://{host}:{port}/{token}", "{schema}://{host}/{fullpath}/{token}", "{schema}://{host}:{port}/{fullpath}/{token}", "{schema}://{botname}@{host}/{token}", "{schema}://{botname}@{host}:{port}/{token}", "{schema}://{botname}@{host}/{fullpath}/{token}", "{schema}://{botname}@{host}:{port}/{fullpath}/{token}"},
			"tokens": map[string]any{
				"botname": map[string]any{
					"map_to":   "user",
					"name":     "Bot Name",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"fullpath": map[string]any{
					"map_to":   "fullpath",
					"name":     "Path",
					"private":  false,
					"required": false,
					"type":     "string",
				},
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
					"values":   []string{"mmost", "mmosts"},
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Webhook Token",
					"private":  true,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"mmost"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"mmosts"},
		"service_name":     "Mattermost",
		"service_url":      "https://mattermost.com/",
		"setup_url":        "https://appriseit.com/services/mattermost/",
	})
}
