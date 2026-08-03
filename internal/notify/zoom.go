package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const zoomURLTemplate = "https://inbots.zoom.us/incoming/hook/%s"

const (
	// Simple posts plain text; full wraps a head and body so a title can be
	// carried separately.
	zoomModeSimple = "simple"
	zoomModeFull   = "full"
)

type ZoomTarget struct {
	webhookID string
	token     string
	mode      string
}

func NewZoomTarget(target *ParsedURL) (*ZoomTarget, error) {
	webhookID := strings.TrimSpace(target.Host)
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

	// Full is the default upstream, so a title survives unless simple is asked for.
	mode := zoomModeFull
	if candidate := strings.ToLower(strings.TrimSpace(target.Query["mode"])); candidate == zoomModeSimple {
		mode = zoomModeSimple
	}

	return &ZoomTarget{webhookID: webhookID, token: token, mode: mode}, nil
}

func (z *ZoomTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	_ = notifyType

	base := fmt.Sprintf(zoomURLTemplate, url.PathEscape(z.webhookID))
	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Accept":        "*/*",
		"Authorization": z.token,
	}

	if z.mode == zoomModeFull {
		content := map[string]any{
			"body": []any{map[string]any{"type": "message", "text": body}},
		}
		if title != "" {
			content["head"] = map[string]any{"text": title}
		}

		data, err := json.Marshal(map[string]any{"content": content})
		if err != nil {
			return RequestSpec{}, err
		}

		headers["Content-Type"] = "application/json"
		return RequestSpec{
			Method:  "POST",
			URL:     base + "?format=full",
			Headers: headers,
			Body:    string(data),
		}, nil
	}

	// Simple mode sends the text unwrapped, with the title prefixed.
	text := body
	if title != "" {
		text = title + ": " + body
	}

	return RequestSpec{
		Method:  "POST",
		URL:     base,
		Headers: headers,
		Body:    text,
	}, nil
}

func (z *ZoomTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := z.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func init() {
	RegisterSchemaEntryOrdered(146, SchemaEntry{
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
					"default":  "text",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"mode": map[string]any{
					"default":  "full",
					"map_to":   "mode",
					"name":     "Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"simple", "full"},
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
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{webhook_id}/{token}"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "zoom",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"zoom"},
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Verification Token",
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
		"protocols": []string{"zoom"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"service_name": "Zoom",
		"service_url":  "https://zoom.us/",
		"setup_url":    "https://appriseit.com/services/zoom/",
	})
}
