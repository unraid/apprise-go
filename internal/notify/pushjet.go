package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type PushjetTarget struct {
	scheme    string
	host      string
	port      int
	secretKey string
	user      string
	password  string
}

func NewPushjetTarget(target *ParsedURL) (*PushjetTarget, error) {
	segments := splitPath(target.Path)
	secretKey := ""
	if len(segments) > 0 {
		secretKey = segments[0]
	}
	if rawSecret, ok := target.Query["secret"]; ok && rawSecret != "" {
		secretKey = rawSecret
	}
	if secretKey == "" {
		return nil, fmt.Errorf("missing secret key")
	}

	scheme := "http"
	if strings.ToLower(target.Scheme) == "pjets" {
		scheme = "https"
	}

	return &PushjetTarget{
		scheme:    scheme,
		host:      target.Host,
		port:      target.Port,
		secretKey: secretKey,
		user:      target.User,
		password:  target.Password,
	}, nil
}

func (p *PushjetTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	messageJSON, err := json.Marshal(body)
	if err != nil {
		return RequestSpec{}, err
	}
	titleJSON, err := json.Marshal(title)
	if err != nil {
		return RequestSpec{}, err
	}
	payload := fmt.Sprintf(
		`{"message": %s, "title": %s, "link": null, "level": null}`,
		string(messageJSON),
		string(titleJSON),
	)

	u := url.URL{
		Scheme: p.scheme,
		Host:   p.host,
		Path:   "/message/",
	}
	if p.port != 0 {
		u.Host = fmt.Sprintf("%s:%d", p.host, p.port)
	}
	q := url.Values{}
	q.Set("secret", p.secretKey)
	u.RawQuery = q.Encode()

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/x-www-form-urlencoded; charset=utf-8",
	}
	if p.user != "" {
		headers["Authorization"] = basicAuthHeader(p.user, p.password)
	}

	_ = notifyType

	return RequestSpec{
		Method:  "POST",
		URL:     u.String(),
		Headers: headers,
		Body:    payload,
	}, nil
}

func (p *PushjetTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := p.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func init() {
	RegisterSchemaEntryOrdered(105, SchemaEntry{
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
				"secret": map[string]any{
					"alias_of": "secret_key",
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
			"templates": []string{"{schema}://{host}:{port}/{secret_key}", "{schema}://{host}/{secret_key}", "{schema}://{user}:{password}@{host}:{port}/{secret_key}", "{schema}://{user}:{password}@{host}/{secret_key}"},
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
					"values":   []string{"pjet", "pjets"},
				},
				"secret_key": map[string]any{
					"map_to":   "secret_key",
					"name":     "Secret Key",
					"private":  true,
					"required": true,
					"type":     "string",
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
		"protocols": []string{"pjet"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"pjets"},
		"service_name":     "Pushjet",
		"service_url":      nil,
		"setup_url":        "https://appriseit.com/services/pushjet/",
	})
}
