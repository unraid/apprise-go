package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const groupmeURL = "https://api.groupme.com/v3/bots/post"

type GroupMeTarget struct {
	botID string
	token string
}

// groupmeBotIDRe mirrors upstream's inline bot_id check.
var groupmeBotIDRe = regexp.MustCompile(`(?i)^[a-z0-9]+$`)

func NewGroupMeTarget(target *ParsedURL) (*GroupMeTarget, error) {
	botID := strings.TrimSpace(target.Query["bot_id"])
	if botID == "" {
		botID = strings.TrimSpace(target.Host)
	}
	if botID == "" {
		return nil, fmt.Errorf("missing bot id")
	}
	// Upstream validates the shape inline rather than declaring a regex on the
	// token, which is why the shared credential table does not cover it.
	if !groupmeBotIDRe.MatchString(botID) {
		return nil, fmt.Errorf("invalid groupme bot id: %q", botID)
	}

	// The access token is only needed for attachment uploads, so it stays
	// optional here.
	token := strings.TrimSpace(target.Query["token"])
	if token == "" {
		if parts := splitPath(target.Path); len(parts) > 0 {
			token = parts[0]
		}
	}

	return &GroupMeTarget{botID: botID, token: token}, nil
}

// GroupMe's image service takes the raw bytes and answers with a URL the
// message then references.
const groupmeImageURL = "https://image.groupme.com/pictures"

func (g *GroupMeTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	return g.buildRequest(body, title, notifyType, nil)
}

func (g *GroupMeTarget) buildRequest(body, title string, notifyType NotifyType, images []any) (RequestSpec, error) {
	_ = notifyType

	payload := map[string]any{
		"bot_id": g.botID,
		"text":   mergeTitleBody(title, body),
	}
	if len(images) > 0 {
		payload["attachments"] = images
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    groupmeURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json; charset=utf-8",
		},
		Body: string(data),
	}, nil
}

func (g *GroupMeTarget) Send(body, title string, notifyType NotifyType) error {
	return g.SendWithAttachments(body, title, notifyType, nil)
}

// SendWithAttachments uploads each image first and references the returned
// URLs in the message. Only images are accepted; anything else is skipped, so
// a PDF would otherwise be silently lost.
func (g *GroupMeTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	images := []any{}
	// Uploading needs an access token. Without one the text still goes out,
	// which is what upstream does rather than failing the notification.
	for _, attachment := range attachments {
		if g.token == "" {
			break
		}
		if !strings.HasPrefix(strings.ToLower(attachment.MIMEType), "image/") {
			continue
		}

		imageURL, err := g.uploadImage(attachment)
		if err != nil {
			return err
		}
		images = append(images, map[string]any{"type": "image", "url": imageURL})
	}

	spec, err := g.buildRequest(body, title, notifyType, images)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

// uploadImage posts the raw bytes and returns the URL GroupMe answers with,
// which it nests under payload.url.
func (g *GroupMeTarget) uploadImage(attachment Attachment) (string, error) {
	var response struct {
		Payload struct {
			URL string `json:"url"`
		} `json:"payload"`
	}

	if err := doJSONRequest(RequestSpec{
		Method: "POST",
		URL:    groupmeImageURL,
		Headers: map[string]string{
			"User-Agent":     "Apprise",
			"Content-Type":   attachment.MIMEType,
			"X-Access-Token": g.token,
		},
		Body: string(attachment.Data),
	}, &response); err != nil {
		return "", err
	}
	if response.Payload.URL == "" {
		return "", fmt.Errorf("groupme image service returned no url")
	}

	return response.Payload.URL, nil
}

func init() {
	RegisterSchemaEntryOrdered(147, SchemaEntry{
		"attachment_support": true,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"bot_id": map[string]any{
					"alias_of": "bot_id",
				},
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
			"templates": []string{"{schema}://{bot_id}", "{schema}://{bot_id}/{token}"},
			"tokens": map[string]any{
				"bot_id": map[string]any{
					"map_to":   "bot_id",
					"name":     "Bot ID",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "groupme",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"groupme"},
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Access Token",
					"private":  true,
					"required": false,
					"type":     "string",
				},
			},
		},
		"enabled": true,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"protocols":        nil,
		"secure_protocols": []string{"groupme"},
		"service_name":     "GroupMe",
		"service_url":      "https://groupme.com/",
		"setup_url":        "https://appriseit.com/services/groupme/",
	})
}
