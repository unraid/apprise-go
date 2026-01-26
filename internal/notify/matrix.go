package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const matrixT2BotWebhookURL = "https://webhooks.t2bot.io/api/v1/matrix/hook/"

type MatrixTarget struct {
	token       string
	displayName string
}

func NewMatrixTarget(target *ParsedURL) (*MatrixTarget, error) {
	mode := strings.ToLower(strings.TrimSpace(target.Query["mode"]))
	if mode != "" && mode != "t2bot" {
		return nil, fmt.Errorf("unsupported matrix mode")
	}

	if len(splitPath(target.Path)) > 0 && mode != "t2bot" {
		return nil, fmt.Errorf("matrix rooms not supported")
	}

	token := strings.TrimSpace(target.Query["token"])
	if token == "" {
		token = strings.TrimSpace(target.Password)
	}
	if token == "" {
		token = strings.TrimSpace(target.Host)
	}
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	displayName := strings.TrimSpace(target.User)
	if displayName == "" {
		displayName = "Apprise"
	}

	return &MatrixTarget{
		token:       token,
		displayName: displayName,
	}, nil
}

func (m *MatrixTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	message := mergeTitleBody(title, body)
	payload := map[string]string{
		"displayName": m.displayName,
		"format":      "plain",
		"text":        message,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    matrixT2BotWebhookURL + m.token,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/json",
		},
		Body: string(data),
	}, nil
}

func (m *MatrixTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := m.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}
	return SendRequest(spec)
}

func init() {
	RegisterSchemaEntryOrdered(2, SchemaEntry{
		"attachment_support": true,
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
				"discovery": map[string]any{
					"default":  true,
					"map_to":   "discovery",
					"name":     "Server Discovery",
					"private":  false,
					"required": false,
					"type":     "bool",
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
					"default":  false,
					"map_to":   "include_image",
					"name":     "Include Image",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"mode": map[string]any{
					"default":  "off",
					"map_to":   "mode",
					"name":     "Webhook Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"off", "matrix", "slack", "t2bot"},
				},
				"msgtype": map[string]any{
					"default":  "text",
					"map_to":   "msgtype",
					"name":     "Message Type",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"text", "notice"},
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
				"version": map[string]any{
					"default":  "3",
					"map_to":   "version",
					"name":     "Matrix API Verion",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"2", "3"},
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{token}", "{schema}://{user}@{token}", "{schema}://{user}:{password}@{host}/{targets}", "{schema}://{user}:{password}@{host}:{port}/{targets}", "{schema}://{token}@{host}/{targets}", "{schema}://{token}@{host}:{port}/{targets}", "{schema}://{user}:{token}@{host}/{targets}", "{schema}://{user}:{token}@{host}:{port}/{targets}"},
			"tokens": map[string]any{
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
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"matrix", "matrixs"},
				},
				"target_room_alias": map[string]any{
					"map_to":   "targets",
					"name":     "Target Room Alias",
					"prefix":   "#",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_room_id": map[string]any{
					"map_to":   "targets",
					"name":     "Target Room ID",
					"prefix":   "!",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_user": map[string]any{
					"map_to":   "targets",
					"name":     "Target User",
					"prefix":   "@",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_room_alias", "target_room_id", "target_user"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "password",
					"name":     "Access Token",
					"private":  true,
					"required": false,
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
		"protocols": []string{"matrix"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"matrixs"},
		"service_name":     "Matrix",
		"service_url":      "https://matrix.org/",
		"setup_url":        "https://appriseit.com/services/matrix/",
	})
}
