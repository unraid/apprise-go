package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	ryverModeSlack = "slack"
	ryverModeRyver = "ryver"
)

var ryverSlackNewline = regexp.MustCompile(`\r\*\n`)

type RyverTarget struct {
	organization string
	token        string
	botname      string
	mode         string
	includeImage bool
}

func NewRyverTarget(target *ParsedURL) (*RyverTarget, error) {
	organization := strings.TrimSpace(target.Host)
	if organization == "" {
		return nil, fmt.Errorf("missing organization")
	}

	segments := splitPath(target.Path)
	if len(segments) == 0 {
		return nil, fmt.Errorf("missing token")
	}
	token := segments[0]

	mode := strings.ToLower(strings.TrimSpace(target.Query["mode"]))
	if mode == "" {
		mode = ryverModeRyver
	}
	if mode != ryverModeRyver && mode != ryverModeSlack {
		return nil, fmt.Errorf("invalid mode: %s", mode)
	}

	includeImage := parseBoolWithDefault(target.Query["image"], true)

	return &RyverTarget{
		organization: organization,
		token:        token,
		botname:      strings.TrimSpace(target.User),
		mode:         mode,
		includeImage: includeImage,
	}, nil
}

func (r *RyverTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := r.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (r *RyverTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	messageTitle := title
	messageBody := body
	if r.mode == ryverModeSlack {
		messageTitle = ryverSlackFormat(messageTitle)
		messageBody = ryverSlackFormat(messageBody)
	}

	message := messageBody
	if strings.TrimSpace(messageTitle) != "" {
		message = fmt.Sprintf("**%s**\r\n%s", messageTitle, messageBody)
	}

	var displayName any
	if r.botname != "" {
		displayName = r.botname
	}

	var avatar any
	if r.includeImage {
		avatar = appriseImageURL(notifyType, "72x72")
	}

	payload := map[string]any{
		"body": message,
		"createSource": map[string]any{
			"displayName": displayName,
			"avatar":      avatar,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	url := fmt.Sprintf("https://%s.ryver.com/application/webhook/%s", r.organization, r.token)

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

func ryverSlackFormat(input string) string {
	if input == "" {
		return input
	}

	formatted := ryverSlackNewline.ReplaceAllString(input, "\\n")
	formatted = strings.ReplaceAll(formatted, "&", "&amp;")
	formatted = strings.ReplaceAll(formatted, "<", "&lt;")
	formatted = strings.ReplaceAll(formatted, ">", "&gt;")
	return formatted
}

func init() {
	RegisterSchemaEntryOrdered(29, SchemaEntry{
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
				"image": map[string]any{
					"default":  true,
					"map_to":   "include_image",
					"name":     "Include Image",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"mode": map[string]any{
					"default":  "ryver",
					"map_to":   "mode",
					"name":     "Webhook Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"slack", "ryver"},
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
			"templates": []string{"{schema}://{organization}/{token}", "{schema}://{botname}@{organization}/{token}"},
			"tokens": map[string]any{
				"botname": map[string]any{
					"map_to":   "user",
					"name":     "Bot Name",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"organization": map[string]any{
					"map_to":   "organization",
					"name":     "Organization",
					"private":  false,
					"regex":    []string{"^[A-Z0-9_-]{3,32}$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "ryver",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"ryver"},
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Token",
					"private":  true,
					"regex":    []string{"^[A-Z0-9]{15}$", "i"},
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
		"secure_protocols": []string{"ryver"},
		"service_name":     "Ryver",
		"service_url":      "https://ryver.com/",
		"setup_url":        "https://appriseit.com/services/ryver/",
	})
}
