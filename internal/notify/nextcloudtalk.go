package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const nextcloudTalkDefaultMessage = "Apprise"

type NextcloudTalkTarget struct {
	host      string
	port      int
	secure    bool
	user      string
	password  string
	targets   []string
	urlPrefix string
	headers   map[string]string
}

func NewNextcloudTalkTarget(target *ParsedURL) (*NextcloudTalkTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	user := strings.TrimSpace(target.User)
	if user == "" || strings.TrimSpace(target.Password) == "" {
		return nil, fmt.Errorf("missing credentials")
	}

	targets := splitPath(target.Path)

	headers := cloneMap(target.QueryAdd)

	urlPrefix := strings.Trim(target.Query["url_prefix"], "/")

	return &NextcloudTalkTarget{
		host:      host,
		port:      target.Port,
		secure:    strings.EqualFold(target.Scheme, "nctalks"),
		user:      user,
		password:  target.Password,
		targets:   targets,
		urlPrefix: urlPrefix,
		headers:   headers,
	}, nil
}

func (n *NextcloudTalkTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(n.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	payload := map[string]string{
		"message": n.message(body, title),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	headers := n.buildHeaders()

	_ = notifyType

	return RequestSpec{
		Method:  "POST",
		URL:     n.buildURL(n.targets[0]),
		Headers: headers,
		Body:    string(data),
	}, nil
}

func (n *NextcloudTalkTarget) Send(body, title string, notifyType NotifyType) error {
	if len(n.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	payload := map[string]string{
		"message": n.message(body, title),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	headers := n.buildHeaders()

	for _, target := range n.targets {
		spec := RequestSpec{
			Method:  "POST",
			URL:     n.buildURL(target),
			Headers: headers,
			Body:    string(data),
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = notifyType

	return nil
}

func (n *NextcloudTalkTarget) buildHeaders() map[string]string {
	headers := map[string]string{
		"User-Agent":     "Apprise",
		"OCS-APIRequest": "true",
		"Accept":         "application/json",
		"Content-Type":   "application/json",
	}
	for key, value := range n.headers {
		headers[key] = value
	}
	headers["Authorization"] = basicAuthHeader(n.user, n.password)
	return headers
}

func (n *NextcloudTalkTarget) message(body, title string) string {
	if strings.TrimSpace(body) == "" {
		if strings.TrimSpace(title) == "" {
			return nextcloudTalkDefaultMessage
		}
		return title
	}
	if strings.TrimSpace(title) == "" {
		return nextcloudTalkDefaultMessage + "\r\n" + body
	}
	return title + "\r\n" + body
}

func (n *NextcloudTalkTarget) buildURL(target string) string {
	scheme := "http"
	if n.secure {
		scheme = "https"
	}

	base := fmt.Sprintf("%s://%s", scheme, n.host)
	if n.port > 0 {
		base += fmt.Sprintf(":%d", n.port)
	}

	path := "/" + n.urlPrefix + "/ocs/v2.php/apps/spreed/api/v1/chat/" + target
	return base + path
}

func init() {
	RegisterSchemaEntryOrdered(27, SchemaEntry{
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
				"tz": map[string]any{
					"default":  nil,
					"map_to":   "tz",
					"name":     "Timezone",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"url_prefix": map[string]any{
					"map_to":   "url_prefix",
					"name":     "URL Prefix",
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
			"kwargs": map[string]any{
				"headers": map[string]any{
					"map_to":   "headers",
					"name":     "HTTP Header",
					"prefix":   "+",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{user}:{password}@{host}/{targets}", "{schema}://{user}:{password}@{host}:{port}/{targets}"},
			"tokens": map[string]any{
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
					"values":   []string{"nctalk", "nctalks"},
				},
				"target_room_id": map[string]any{
					"map_to":   "targets",
					"name":     "Room ID",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_room_id"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": true,
					"type":     "list:string",
				},
				"user": map[string]any{
					"map_to":   "user",
					"name":     "Username",
					"private":  false,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"nctalk"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"nctalks"},
		"service_name":     "Nextcloud Talk",
		"service_url":      "https://nextcloud.com/talk",
		"setup_url":        "https://appriseit.com/services/nextcloudtalk/",
	})
}
