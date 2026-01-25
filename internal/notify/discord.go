package notify

import (
	"encoding/json"
	"fmt"
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

	return &DiscordTarget{
		webhookID:    webhookID,
		webhookToken: webhookToken,
		username:     target.User,
		tts:          tts,
		avatar:       avatar,
		avatarURL:    avatarURL,
	}, nil
}

func (d *DiscordTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload := map[string]any{
		"tts":  d.tts,
		"wait": !d.tts,
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

	if body != "" {
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

	url := fmt.Sprintf("%s/%s/%s", discordWebhookBase, d.webhookID, d.webhookToken)

	return RequestSpec{
		Method:  "POST",
		URL:     url,
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
