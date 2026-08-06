package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const discordWebhookBase = "https://discord.com/api/webhooks"

// Discord accepts at most this many files in one message.
const discordMaxAttachments = 10

type DiscordTarget struct {
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

	// :key=value pairs are substituted into the template.
	templateTokens := map[string]string{}
	for key, value := range target.QueryPayload {
		templateTokens[key] = value
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
		// Batching only affects how attachments are grouped, which this port
		// does not send; it is accepted so the URL round-trips.
		batch:          parseBoolWithDefault(target.Query["batch"], true),
		templatePath:   strings.TrimSpace(target.Query["template"]),
		templateTokens: templateTokens,
	}, nil
}

func (d *DiscordTarget) buildPayload(body, title string, notifyType NotifyType) (map[string]any, error) {
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

	// A template defines the whole message, so the embed and content the
	// plugin would otherwise build are skipped entirely.
	if d.templatePath != "" {
		rendered, err := renderNotifyTemplate(d.templatePath, d.templateTokens, body, title, notifyType, "256x256")
		if err != nil {
			return nil, err
		}
		for key, value := range rendered {
			payload[key] = value
		}
	} else if d.format == "markdown" {
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

func (d *DiscordTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	return d.buildRequest(body, title, notifyType, nil)
}

func (d *DiscordTarget) buildRequest(body, title string, notifyType NotifyType, attachments []Attachment) (RequestSpec, error) {
	payload, err := d.buildPayload(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}

	requestBody := ""
	contentType := "application/json; charset=utf-8"
	if len(attachments) > 0 {
		// With files present the payload moves into a multipart field, and
		// the generated boundary decides the content type.
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

	return d.specFor(requestBody, contentType)
}

func (d *DiscordTarget) specFor(requestBody, contentType string) (RequestSpec, error) {
	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": contentType,
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
		Body:    requestBody,
	}, nil
}

func (d *DiscordTarget) Send(body, title string, notifyType NotifyType) error {
	return d.SendWithAttachments(body, title, notifyType, nil)
}

// SendWithAttachments posts the message, then the files in a second request.
// Discord does not carry them alongside the text: the attachment post reuses
// the payload with the message content stripped out, so the body is not
// repeated under each file.
func (d *DiscordTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	// Upstream keeps going after a failed target; see sendOutcome.
	var outcome sendOutcome
	spec, err := d.buildRequest(body, title, notifyType, nil)
	if err != nil {
		return err
	}
	outcome.record(SendRequest(spec))

	if len(attachments) == 0 {
		return outcome.err()
	}

	payload, err := d.buildPayload(body, title, notifyType)
	if err != nil {
		return err
	}
	payload["tts"] = false
	// Wait for the upload to post before continuing.
	payload["wait"] = true
	delete(payload, "content")
	delete(payload, "embeds")
	delete(payload, "allow_mentions")

	// With batching off each file is its own message, which is the legacy
	// one-per-message behavior.
	perRequest := discordMaxAttachments
	if !d.batch {
		perRequest = 1
	}

	for start := 0; start < len(attachments); start += perRequest {
		end := min(start+perRequest, len(attachments))
		requestBody, contentType, err := discordStyleAttachmentBody(payload, attachments[start:end])
		if err != nil {
			return err
		}
		spec, err := d.specFor(requestBody, contentType)
		if err != nil {
			return err
		}
		outcome.record(SendRequest(spec))
	}

	return outcome.err()
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
