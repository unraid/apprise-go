package notify

import (
	"fmt"
	"net/url"
)

const serverChanURL = "https://sctapi.ftqq.com/%s.send"

type ServerChanTarget struct {
	token string
}

func NewServerChanTarget(target *ParsedURL) (*ServerChanTarget, error) {
	token := target.Host
	if token == "" {
		segments := splitPath(target.Path)
		if len(segments) > 0 {
			token = segments[0]
		}
	}
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	return &ServerChanTarget{token: token}, nil
}

func (s *ServerChanTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	values := url.Values{}
	values.Set("title", title)
	values.Set("desp", body)

	_ = notifyType

	headers := map[string]string{
		"Accept":       "*/*",
		"Content-Type": "application/x-www-form-urlencoded",
	}

	return RequestSpec{
		Method:  "POST",
		URL:     fmt.Sprintf(serverChanURL, s.token),
		Headers: headers,
		Body:    values.Encode(),
	}, nil
}

func (s *ServerChanTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := s.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func init() {
	RegisterSchemaEntryOrdered(60, SchemaEntry{
		"service_name":       "ServerChan",
		"service_url":        "https://sct.ftqq.com/",
		"setup_url":          "https://appriseit.com/services/serverchan/",
		"attachment_support": false,
		"category":           "native",
		"enabled":            true,
		"protocols":          []string(nil),
		"secure_protocols":   []string{"schan"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []string{},
			"packages_required":    []string{},
		},
		"details": map[string]any{
			"args": map[string]any{
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
			"kwargs": map[string]any{},
			"templates": []string{
				"{schema}://{token}",
			},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "schan",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"schan"},
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Token",
					"private":  true,
					"regex":    []string{"^[a-z0-9-]+$", "i"},
					"required": true,
					"type":     "string",
				},
			},
		},
	})
}
