package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const chimeURLTemplate = "https://hooks.chime.aws/incomingwebhooks/%s?token=%s"

type ChimeTarget struct {
	webhookID string
	token     string
}

func NewChimeTarget(target *ParsedURL) (*ChimeTarget, error) {
	webhookID := strings.TrimSpace(target.Query["webhook_id"])
	if webhookID == "" {
		webhookID = strings.TrimSpace(target.Host)
	}
	if webhookID == "" {
		return nil, fmt.Errorf("missing webhook id")
	}

	token := strings.TrimSpace(target.Query["token"])
	if token == "" {
		if parts := splitPath(target.Path); len(parts) > 0 {
			token = parts[0]
		}
	}
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	return &ChimeTarget{webhookID: webhookID, token: token}, nil
}

func (c *ChimeTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	_ = notifyType

	// Chime has no title field; the framework folds it into the body, which
	// the webhook renders as markdown.
	payload := map[string]any{"Content": mergeTitleBody(title, body)}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		// The token carries base64 padding, so it has to be escaped.
		URL: fmt.Sprintf(chimeURLTemplate, c.webhookID, url.QueryEscape(c.token)),
		Headers: map[string]string{
			"User-Agent": "Apprise",
			"Accept":     "*/*",
			// Chime rejects a bare application/json content type.
			"Content-Type": "application/json; charset=utf-8",
		},
		Body: string(data),
	}, nil
}

func (c *ChimeTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := c.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func init() {
	RegisterSchemaEntryOrdered(144, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
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
					"default":  "markdown",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"optional": map[string]any{
					"default":  false,
					"map_to":   "optional",
					"name":     "Optional Service",
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
				"redirect": map[string]any{
					"default":  true,
					"map_to":   "redirect",
					"name":     "Follow Redirects",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"retry": map[string]any{
					"default":  0,
					"map_to":   "retry",
					"max":      10,
					"min":      0,
					"name":     "Service Retry",
					"private":  false,
					"required": false,
					"type":     "int",
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
				"wait": map[string]any{
					"default":  0.0,
					"map_to":   "wait",
					"max":      20.0,
					"min":      0.0,
					"name":     "Inter-Retry Wait",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"webhook_id": map[string]any{
					"alias_of": "webhook_id",
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{webhook_id}/{token}"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "chime",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"chime"},
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Webhook Token",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"webhook_id": map[string]any{
					"map_to":   "webhook_id",
					"name":     "Webhook ID",
					"private":  true,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"chime"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"service_name": "Amazon Chime",
		"service_url":  "https://aws.amazon.com/chime/",
		"setup_url":    "https://appriseit.com/services/chime/",
	})
}
