package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const googleChatURL = "https://chat.googleapis.com/v1/spaces/%s/messages"

const googleChatReplyOption = "REPLY_MESSAGE_FALLBACK_TO_NEW_THREAD"

type GoogleChatTarget struct {
	workspace string
	key       string
	token     string
	thread    string
}

func NewGoogleChatTarget(target *ParsedURL) (*GoogleChatTarget, error) {
	segments := splitPathSegments(target.Path)

	workspace := strings.TrimSpace(target.Host)
	if raw := strings.TrimSpace(target.Query["workspace"]); raw != "" {
		workspace = raw
	}

	key := ""
	if len(segments) > 0 {
		key = segments[0]
	}
	if raw := strings.TrimSpace(target.Query["key"]); raw != "" {
		key = raw
	}

	token := ""
	if len(segments) > 1 {
		token = segments[1]
	}
	if raw := strings.TrimSpace(target.Query["token"]); raw != "" {
		token = raw
	}

	thread := ""
	if len(segments) > 2 {
		thread = segments[2]
	}
	if raw := strings.TrimSpace(target.Query["thread"]); raw != "" {
		thread = raw
	} else if raw := strings.TrimSpace(target.Query["threadkey"]); raw != "" {
		thread = raw
	}

	if workspace == "" || key == "" || token == "" {
		return nil, fmt.Errorf("missing workspace, key, or token")
	}

	return &GoogleChatTarget{
		workspace: workspace,
		key:       key,
		token:     token,
		thread:    thread,
	}, nil
}

func (g *GoogleChatTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	message := body
	if title != "" {
		message = title + "\r\n" + body
	}

	payload := map[string]any{
		"text": message,
	}

	params := url.Values{}
	params.Set("token", g.token)
	params.Set("key", g.key)

	if g.thread != "" {
		params.Set("messageReplyOption", googleChatReplyOption)
		payload["thread"] = map[string]string{
			"thread_key": g.thread,
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	requestURL := fmt.Sprintf(googleChatURL, g.workspace)
	if encoded := params.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}

	return RequestSpec{
		Method: "POST",
		URL:    requestURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json; charset=utf-8",
		},
		Body: string(data),
	}, nil
}

func (g *GoogleChatTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := g.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func splitPathSegments(rawPath string) []string {
	path := strings.Trim(rawPath, "/")
	if path == "" {
		return nil
	}
	parts := strings.Split(path, "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		decoded, err := url.PathUnescape(part)
		if err != nil {
			decoded = part
		}
		decoded = strings.TrimSpace(decoded)
		if decoded != "" {
			segments = append(segments, decoded)
		}
	}
	return segments
}

func init() {
	RegisterSchemaEntryOrdered(66, SchemaEntry{
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
					"default":  "markdown",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"key": map[string]any{
					"alias_of": "webhook_key",
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
				"thread": map[string]any{
					"alias_of": "thread_key",
				},
				"token": map[string]any{
					"alias_of": "webhook_token",
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
				"workspace": map[string]any{
					"alias_of": "workspace",
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{workspace}/{webhook_key}/{webhook_token}", "{schema}://{workspace}/{webhook_key}/{webhook_token}/{thread_key}"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "gchat",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"gchat"},
				},
				"thread_key": map[string]any{
					"map_to":   "thread_key",
					"name":     "Thread Key",
					"private":  true,
					"required": false,
					"type":     "string",
				},
				"webhook_key": map[string]any{
					"map_to":   "webhook_key",
					"name":     "Webhook Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"webhook_token": map[string]any{
					"map_to":   "webhook_token",
					"name":     "Webhook Token",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"workspace": map[string]any{
					"map_to":   "workspace",
					"name":     "Workspace",
					"private":  true,
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
		"secure_protocols": []string{"gchat"},
		"service_name":     "Google Chat",
		"service_url":      "https://chat.google.com/",
		"setup_url":        "https://appriseit.com/services/googlechat/",
	})
}
