package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const webexWebhookURL = "https://api.ciscospark.com/v1/webhooks/incoming//"

// Bot mode posts messages to rooms through the API rather than a webhook.
const webexBotURL = "https://webexapis.com/v1/messages"

// A webhook token is 80-160 alphanumeric characters; anything else is a bot
// access token.
var webexTokenPattern = regexp.MustCompile(`(?i)^[a-z0-9]{80,160}$`)

type WebexTeamsTarget struct {
	token       string
	accessToken string
	mode        string
	rooms       []string
	format      string
}

func NewWebexTeamsTarget(target *ParsedURL) (*WebexTeamsTarget, error) {
	token := strings.TrimSpace(target.Host)
	if rawToken, ok := target.Query["token"]; ok && strings.TrimSpace(rawToken) != "" {
		token = strings.TrimSpace(rawToken)
	}
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	rooms := splitPath(target.Path)
	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		rooms = append(rooms, splitPath(to)...)
	}

	mode := ""
	if raw := strings.TrimSpace(strings.ToLower(target.Query["mode"])); raw != "" {
		for _, candidate := range []string{"webhook", "bot"} {
			if strings.HasPrefix(candidate, raw) {
				mode = candidate
				break
			}
		}
		if mode == "" {
			return nil, fmt.Errorf("invalid mode: %s", target.Query["mode"])
		}
	}

	// With no mode given, room IDs in the path mean bot mode; otherwise the
	// token's shape decides.
	if mode == "" {
		if len(rooms) > 0 || !webexTokenPattern.MatchString(token) {
			mode = "bot"
		} else {
			mode = "webhook"
		}
	}

	format := normalizeNotifyFormat(target.Query["format"])
	if format == "" {
		format = "markdown"
	}

	if mode == "webhook" {
		if !webexTokenPattern.MatchString(token) {
			return nil, fmt.Errorf("invalid token")
		}

		return &WebexTeamsTarget{token: token, mode: mode, format: format}, nil
	}

	if len(rooms) == 0 {
		return nil, fmt.Errorf("missing room ids")
	}

	return &WebexTeamsTarget{accessToken: token, mode: mode, rooms: rooms, format: format}, nil
}

func (w *WebexTeamsTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	specs, err := w.buildRequests(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}

	return specs[0], nil
}

func (w *WebexTeamsTarget) buildRequests(body, title string, notifyType NotifyType) ([]RequestSpec, error) {
	_ = notifyType

	// Webex has no title field, so the title is folded into the message.
	message := body
	if title != "" {
		message = title + "\r\n" + body
	}

	// Only markdown gets its own key; anything else is plain text.
	messageKey := "text"
	if w.format == "markdown" {
		messageKey = "markdown"
	}

	if w.mode == "webhook" {
		data, err := json.Marshal(map[string]string{messageKey: message})
		if err != nil {
			return nil, err
		}

		return []RequestSpec{{
			Method: "POST",
			URL:    webexWebhookURL + w.token,
			Headers: map[string]string{
				"User-Agent":   "Apprise",
				"Accept":       "*/*",
				"Content-Type": "application/json",
			},
			Body: string(data),
		}}, nil
	}

	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Accept":        "*/*",
		"Authorization": "Bearer " + w.accessToken,
		"Content-Type":  "application/json",
	}

	specs := make([]RequestSpec, 0, len(w.rooms))
	for _, room := range w.rooms {
		data, err := json.Marshal(map[string]string{
			"roomId":   room,
			messageKey: message,
		})
		if err != nil {
			return nil, err
		}

		specs = append(specs, RequestSpec{
			Method:  "POST",
			URL:     webexBotURL,
			Headers: headers,
			Body:    string(data),
		})
	}

	return specs, nil
}

func (w *WebexTeamsTarget) Send(body, title string, notifyType NotifyType) error {
	return w.SendWithAttachments(body, title, notifyType, nil)
}

// SendWithAttachments posts one request per file in bot mode. Only the first
// carries the message text, so a notification with three files does not
// arrive as three copies of the message.
func (w *WebexTeamsTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	// Upstream keeps going after a failed target; see sendOutcome.
	var outcome sendOutcome
	if len(attachments) == 0 || w.mode != "bot" {
		specs, err := w.buildRequests(body, title, notifyType)
		if err != nil {
			return err
		}

		for _, spec := range specs {
			outcome.record(SendRequest(spec))
		}

		return outcome.err()
	}

	message := body
	if title != "" {
		message = title + "\r\n" + body
	}

	messageKey := "text"
	if w.format == "markdown" {
		messageKey = "markdown"
	}

	for _, room := range w.rooms {
		for index, attachment := range attachments {
			values := formFields{}
			values.Set("roomId", room)
			if index == 0 {
				values.Set(messageKey, message)
			}

			requestBody, contentType, err := singleFileAttachmentBody(
				values, "files", attachment, true)
			if err != nil {
				return err
			}

			if err := SendRequest(RequestSpec{
				Method: "POST",
				URL:    webexBotURL,
				Headers: map[string]string{
					"User-Agent":    "Apprise",
					"Accept":        "*/*",
					"Authorization": "Bearer " + w.accessToken,
					"Content-Type":  contentType,
				},
				Body: requestBody,
			}); err != nil {
				return err
			}
		}
	}

	return outcome.err()
}

func init() {
	RegisterSchemaEntryOrdered(70, SchemaEntry{
		"attachment_support": true,
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
				"mode": map[string]any{
					"map_to":   "mode",
					"name":     "Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"webhook", "bot"},
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
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{token}", "{schema}://{access_token}/{targets}"},
			"tokens": map[string]any{
				"access_token": map[string]any{
					"map_to":   "access_token",
					"name":     "Bot Access Token",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"webex", "wxteams"},
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []any{},
					"map_to":   "targets",
					"name":     "Room IDs",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Webhook Token",
					"private":  true,
					"regex":    []string{"^[a-z0-9]{80,160}$", "i"},
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
		"secure_protocols": []string{"wxteams", "webex"},
		"service_name":     "Cisco Webex Teams",
		"service_url":      "https://webex.teams.com/",
		"setup_url":        "https://appriseit.com/services/wxteams/",
	})
}
