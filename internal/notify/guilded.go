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
	webhookID    string
	webhookToken string
	username     string
	tts          bool
	avatar       bool
	avatarURL    string
	threadID     string
	flags        int
	format       string
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
	}, nil
}

func (g *GuildedTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
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

	if g.format == "markdown" {
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

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/json; charset=utf-8",
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
		Body:    string(data),
	}, nil
}

func (g *GuildedTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := g.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
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
			"kwargs":    map[string]any{},
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
