package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const guildedWebhookBase = "https://media.guilded.gg/webhooks"

type GuildedTarget struct {
	webhookID      string
	webhookToken   string
	username       string
	tts            bool
	avatar         bool
	avatarURL      string
	threadID       string
	flags          int
	format         string
	batch          bool
	templatePath   string
	templateTokens map[string]string
}

func NewGuildedTarget(target *ParsedURL) (*GuildedTarget, error) {
	webhookID := target.Host
	segments := splitPath(target.Path)
	if webhookID == "" || len(segments) == 0 {
		return nil, fmt.Errorf("missing webhook credentials")
	}
	webhookToken := segments[0]

	tts := parseBoolWithDefault(target.Query["tts"], false)
	avatar := true
	if rawAvatar, ok := target.Query["avatar"]; ok {
		avatar = parseBoolWithDefault(rawAvatar, true)
	}
	avatarURL := ""
	if rawAvatarURL, ok := target.Query["avatar_url"]; ok && rawAvatarURL != "" {
		avatarURL = rawAvatarURL
	}

	threadID := strings.TrimSpace(target.Query["thread"])
	format := normalizeDiscordFormat(target.Query["format"])
	if format == "" {
		if threadID != "" {
			format = "markdown"
		} else {
			format = "text"
		}
	}

	flags := 0
	if rawFlags := strings.TrimSpace(target.Query["flags"]); rawFlags != "" {
		value, err := strconv.Atoi(rawFlags)
		if err != nil || value < 0 {
			return nil, fmt.Errorf("invalid flags")
		}
		flags = value
	}

	// :key=value pairs are substituted into the template.
	templateTokens := map[string]string{}
	for key, value := range target.QueryPayload {
		templateTokens[key] = value
	}

	return &GuildedTarget{
		webhookID:    webhookID,
		webhookToken: webhookToken,
		username:     target.User,
		tts:          tts,
		avatar:       avatar,
		avatarURL:    avatarURL,
		threadID:     threadID,
		flags:        flags,
		format:       format,
		// Batching only affects how attachments are grouped, which this port
		// does not send; it is accepted so the URL round-trips.
		batch:          parseBoolWithDefault(target.Query["batch"], true),
		templatePath:   strings.TrimSpace(target.Query["template"]),
		templateTokens: templateTokens,
	}, nil
}

func (g *GuildedTarget) buildPayload(body, title string, notifyType NotifyType) (map[string]any, error) {
	payload := map[string]any{
		"tts":  g.tts,
		"wait": !g.tts,
	}

	if g.flags > 0 {
		payload["flags"] = g.flags
	}

	if g.avatar {
		if g.avatarURL != "" {
			payload["avatar_url"] = g.avatarURL
		} else {
			payload["avatar_url"] = defaultImageURL(notifyType)
		}
	}

	if g.username != "" {
		payload["username"] = g.username
	}

	// A template defines the whole message, so the embed and content the
	// plugin would otherwise build are skipped entirely.
	if g.templatePath != "" {
		rendered, err := renderNotifyTemplate(g.templatePath, g.templateTokens, body, title, notifyType, "256x256")
		if err != nil {
			return nil, err
		}
		for key, value := range rendered {
			payload[key] = value
		}
	} else if g.format == "markdown" {
		embed := map[string]any{
			"author": map[string]any{
				"name": "Apprise",
				"url":  appriseAppURL,
			},
			"title":       title,
			"description": body,
			"color":       appriseColorInt(notifyType),
		}
		payload["embeds"] = []any{embed}
	} else if body != "" {
		if title == "" {
			payload["content"] = body
		} else {
			payload["content"] = title + "\r\n" + body
		}
	}

	return payload, nil
}

func (g *GuildedTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	return g.buildRequest(body, title, notifyType, nil)
}

func (g *GuildedTarget) buildRequest(body, title string, notifyType NotifyType, attachments []Attachment) (RequestSpec, error) {
	payload, err := g.buildPayload(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}

	requestBody := ""
	contentType := "application/json; charset=utf-8"
	if len(attachments) > 0 {
		// Files move the payload into a multipart field, and the generated
		// boundary decides the content type.
		requestBody, contentType, err = discordStyleAttachmentBody(payload, attachments)
		if err != nil {
			return RequestSpec{}, err
		}
	} else {
		data, err := json.Marshal(payload)
		if err != nil {
			return RequestSpec{}, err
		}
		requestBody = string(data)
	}

	return g.specFor(requestBody, contentType)
}

func (g *GuildedTarget) specFor(requestBody, contentType string) (RequestSpec, error) {
	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": contentType,
	}

	targetURL := fmt.Sprintf("%s/%s/%s", guildedWebhookBase, g.webhookID, g.webhookToken)
	if g.threadID != "" {
		parsed, err := url.Parse(targetURL)
		if err != nil {
			return RequestSpec{}, err
		}
		query := parsed.Query()
		query.Set("thread_id", g.threadID)
		parsed.RawQuery = query.Encode()
		targetURL = parsed.String()
	}

	return RequestSpec{
		Method:  "POST",
		URL:     targetURL,
		Headers: headers,
		Body:    requestBody,
	}, nil
}

func (g *GuildedTarget) Send(body, title string, notifyType NotifyType) error {
	return g.SendWithAttachments(body, title, notifyType, nil)
}

// SendWithAttachments posts the message, then the files separately, the way
// Discord does — Guilded is modelled on it.
func (g *GuildedTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	spec, err := g.buildRequest(body, title, notifyType, nil)
	if err != nil {
		return err
	}
	if err := SendRequest(spec); err != nil {
		return err
	}

	if len(attachments) == 0 {
		return nil
	}

	payload, err := g.buildPayload(body, title, notifyType)
	if err != nil {
		return err
	}
	payload["tts"] = false
	payload["wait"] = true
	delete(payload, "content")
	delete(payload, "embeds")
	delete(payload, "allow_mentions")

	perRequest := discordMaxAttachments
	if !g.batch {
		perRequest = 1
	}

	for start := 0; start < len(attachments); start += perRequest {
		end := min(start+perRequest, len(attachments))
		requestBody, contentType, err := discordStyleAttachmentBody(payload, attachments[start:end])
		if err != nil {
			return err
		}
		spec, err := g.specFor(requestBody, contentType)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func init() {
	RegisterSchemaEntryOrdered(4, SchemaEntry{
		"attachment_support": true,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"avatar": map[string]any{
					"default":  true,
					"map_to":   "avatar",
					"name":     "Avatar Image",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"avatar_url": map[string]any{
					"map_to":   "avatar_url",
					"name":     "Avatar URL",
					"private":  false,
					"required": false,
					"type":     "string",
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
				"fields": map[string]any{
					"default":  true,
					"map_to":   "fields",
					"name":     "Use Fields",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"flags": map[string]any{
					"map_to":   "flags",
					"min":      0,
					"name":     "Discord Flags",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"footer": map[string]any{
					"default":  false,
					"map_to":   "footer",
					"name":     "Display Footer",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"footer_logo": map[string]any{
					"default":  true,
					"map_to":   "footer_logo",
					"name":     "Footer Logo",
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
				"batch": map[string]any{
					"default":  true,
					"map_to":   "batch",
					"name":     "Batch Attachments",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"template": map[string]any{
					"map_to":   "template",
					"name":     "Template Path",
					"private":  true,
					"required": false,
					"type":     "string",
				},
				"href": map[string]any{
					"map_to":   "href",
					"name":     "URL",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"image": map[string]any{
					"default":  false,
					"map_to":   "include_image",
					"name":     "Include Image",
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
				"ping": map[string]any{
					"delim":    []string{",", " "},
					"group":    []any{},
					"map_to":   "ping",
					"name":     "Ping Users/Roles",
					"private":  false,
					"required": false,
					"type":     "list:string",
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
				"thread": map[string]any{
					"map_to":   "thread",
					"name":     "Thread ID",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"tts": map[string]any{
					"default":  false,
					"map_to":   "tts",
					"name":     "Text To Speech",
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
				"url": map[string]any{
					"alias_of": "href",
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
			"kwargs": map[string]any{
				"tokens": map[string]any{
					"map_to":   "tokens",
					"name":     "Template Tokens",
					"prefix":   ":",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{webhook_id}/{webhook_token}", "{schema}://{botname}@{webhook_id}/{webhook_token}"},
			"tokens": map[string]any{
				"botname": map[string]any{
					"map_to":   "user",
					"name":     "Bot Name",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "guilded",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"guilded"},
				},
				"webhook_id": map[string]any{
					"map_to":   "webhook_id",
					"name":     "Webhook ID",
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
			},
		},
		"enabled":   true,
		"protocols": nil,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"guilded"},
		"service_name":     "Guilded",
		"service_url":      "https://guilded.gg/",
		"setup_url":        "https://appriseit.com/services/guilded/",
	})
}
