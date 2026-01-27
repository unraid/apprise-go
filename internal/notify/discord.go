package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const discordWebhookBase = "https://discord.com/api/webhooks"

type DiscordTarget struct {
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

func NewDiscordTarget(target *ParsedURL) (*DiscordTarget, error) {
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

	return &DiscordTarget{
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

func (d *DiscordTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload := map[string]any{
		"tts":  d.tts,
		"wait": !d.tts,
	}

	if d.flags > 0 {
		payload["flags"] = d.flags
	}

	if d.avatar {
		if d.avatarURL != "" {
			payload["avatar_url"] = d.avatarURL
		} else {
			payload["avatar_url"] = defaultImageURL(notifyType)
		}
	}

	if d.username != "" {
		payload["username"] = d.username
	}

	if d.format == "markdown" {
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

	targetURL := fmt.Sprintf("%s/%s/%s", discordWebhookBase, d.webhookID, d.webhookToken)
	if d.threadID != "" {
		parsed, err := url.Parse(targetURL)
		if err != nil {
			return RequestSpec{}, err
		}
		query := parsed.Query()
		query.Set("thread_id", d.threadID)
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

func (d *DiscordTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := d.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func defaultImageURL(notifyType NotifyType) string {
	if strings.TrimSpace(string(notifyType)) == "" {
		notifyType = NotifyInfo
	}

	return appriseImageURL(notifyType, "256x256")
}

func normalizeDiscordFormat(raw string) string {
	format := strings.ToLower(strings.TrimSpace(raw))
	switch format {
	case "":
		return ""
	case "markdown", "md", "notifyformat.markdown":
		return "markdown"
	case "html", "notifyformat.html":
		return "html"
	case "text", "notifyformat.text":
		return "text"
	default:
		return ""
	}
}
