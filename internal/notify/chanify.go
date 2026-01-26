package notify

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const chanifyURL = "https://api.chanify.net/v1/sender/%s/"

var chanifyTokenPattern = regexp.MustCompile(`(?i)^[A-Z0-9._-]+$`)

type ChanifyTarget struct {
	token string
}

func NewChanifyTarget(target *ParsedURL) (*ChanifyTarget, error) {
	token := target.Host
	if rawToken, ok := target.Query["token"]; ok && rawToken != "" {
		token = rawToken
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}
	if !chanifyTokenPattern.MatchString(token) {
		return nil, fmt.Errorf("invalid token")
	}

	return &ChanifyTarget{token: token}, nil
}

func (c *ChanifyTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	text := body
	if title != "" {
		text = title + "\r\n" + body
	}

	values := url.Values{}
	values.Set("text", text)

	_ = title
	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    fmt.Sprintf(chanifyURL, c.token),
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/x-www-form-urlencoded",
		},
		Body: values.Encode(),
	}, nil
}

func (c *ChanifyTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := c.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func init() {
	RegisterSchemaEntryOrdered(73, SchemaEntry{
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
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{token}"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "chanify",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"chanify"},
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Token",
					"private":  true,
					"regex":    []string{"^[A-Z0-9._-]+$", "i"},
					"required": true,
					"type":     "string",
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
		"secure_protocols": []string{"chanify"},
		"service_name":     "Chanify",
		"service_url":      "https://chanify.net/",
		"setup_url":        "https://appriseit.com/services/chanify/",
	})
}
