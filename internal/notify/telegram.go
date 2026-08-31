package notify

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
)

const telegramAPIBase = "https://api.telegram.org/bot"

type telegramRecipient struct {
	chatID       string
	chatIDInt    int64
	isNumeric    bool
	messageTopic int
}

type TelegramTarget struct {
	botToken     string
	content      string
	targets      []telegramRecipient
	notifyFormat string
	markdownMode string
	silent       bool
	preview      bool
	detect       bool
	includeImage bool

	// templatePath switches the send to Telegram's Rich Message endpoint;
	// the template defines the whole message and body/title only feed its
	// substitution tokens.
	templatePath   string
	templateTokens map[string]string
}

func NewTelegramTarget(target *ParsedURL) (*TelegramTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing bot token")
	}

	segments := splitPath(target.Path)
	decodedHost, err := url.PathUnescape(host)
	if err != nil {
		decodedHost = host
	}

	botToken := ""
	rawTargets := []string{}
	if strings.Contains(decodedHost, ":") {
		botToken = decodedHost
		rawTargets = append(rawTargets, segments...)
	} else {
		if len(segments) == 0 {
			return nil, fmt.Errorf("missing bot token")
		}
		botToken = decodedHost + ":" + segments[0]
		rawTargets = append(rawTargets, segments[1:]...)
	}
	if len(botToken) >= 3 && strings.EqualFold(botToken[:3], "bot") {
		botToken = botToken[3:]
	}

	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		rawTargets = append(rawTargets, parseDelimitedList(toValue)...)
	}

	defaultTopic := parseOptionalIntValue(target.Query["topic"])
	if defaultTopic == nil {
		defaultTopic = parseOptionalIntValue(target.Query["thread"])
	}

	targets := make([]telegramRecipient, 0, len(rawTargets))
	for _, entry := range rawTargets {
		if recipient, ok := parseTelegramRecipient(entry, defaultTopic); ok {
			targets = append(targets, recipient)
		}
	}

	detect := parseBoolValue(target.Query["detect"], len(targets) == 0)

	// An unrecognized ?format= falls back to the plugin default rather than
	// failing: upstream's base class only maps the value it knows and leaves
	// notify_format at the default otherwise, so a typo changes the rendering
	// but never rejects the URL.
	format := normalizeNotifyFormat(target.Query["format"])
	switch format {
	case "html", "markdown", "text":
	default:
		format = "html"
	}

	templateTokens := map[string]string{}
	for key, value := range target.QueryPayload {
		templateTokens[key] = value
	}

	return &TelegramTarget{
		templatePath:   strings.TrimSpace(target.Query["template"]),
		templateTokens: templateTokens,
		botToken:       botToken,
		content:        telegramContentPlacement(target.Query["content"]),
		targets:        targets,
		notifyFormat:   format,
		markdownMode:   telegramMarkdownMode(target.Query["mdv"]),
		silent:         parseBoolValue(target.Query["silent"], false),
		preview:        parseBoolValue(target.Query["preview"], false),
		detect:         detect,
		includeImage:   parseBoolValue(target.Query["image"], false),
	}, nil
}

func (t *TelegramTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(t.targets) == 0 {
		if t.detect {
			return t.buildDetectSpec(), nil
		}
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := formatTelegramMessage(title, body, t.notifyFormat, t.markdownMode)
	spec, err := t.buildSpec(message, t.targets[0])
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return spec, nil
}

func (t *TelegramTarget) Send(body, title string, notifyType NotifyType) error {
	return t.SendWithAttachments(body, title, notifyType, nil)
}

// telegramCaptionMaxLen is the longest message Telegram will carry as a
// caption on a media item.
const telegramCaptionMaxLen = 1024

func telegramContentPlacement(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), "after") {
		return "after"
	}

	return "before"
}

func (t *TelegramTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	// Upstream keeps going after a failed target; see sendOutcome.
	var outcome sendOutcome
	if len(t.targets) == 0 {
		if t.detect {
			return SendRequest(t.buildDetectSpec())
		}
		return nil
	}

	if t.templatePath != "" {
		// A Rich Message template bypasses the normal text/markdown/HTML
		// handling entirely -- the template itself defines the content.
		return t.sendRichMessage(body, title, notifyType, attachments)
	}

	message := formatTelegramMessage(title, body, t.notifyFormat, t.markdownMode)

	if t.parseMode() == "HTML" {
		// The caption goes through the same rewrite the message body does;
		// Telegram rejects the tags it does not know wherever they appear.
		message = rewriteTelegramHTML(message)
	}

	// A short message rides along as the media caption rather than being
	// sent on its own; sending both would notify twice for one notification.
	caption := ""
	if len(attachments) > 0 && body != "" && len(message) < telegramCaptionMaxLen {
		caption = message
	}

	for _, recipient := range t.targets {
		if t.includeImage {
			spec, err := t.buildImageSpec(recipient)
			if err != nil {
				return err
			}
			outcome.record(SendRequest(spec))
		}
		if caption == "" {
			spec, err := t.buildSpec(message, recipient)
			if err != nil {
				return err
			}
			outcome.record(SendRequest(spec))
		}

		// Each file goes to the endpoint Telegram wants for its type; a
		// photo posted as a document arrives as an unpreviewable file. Only
		// the first carries the caption.
		for index, attachment := range attachments {
			attachmentCaption := ""
			if index == 0 {
				attachmentCaption = caption
			}

			spec, err := t.buildAttachmentSpec(recipient, attachment, attachmentCaption, index)
			if err != nil {
				return err
			}
			outcome.record(SendRequest(spec))
		}
	}

	_ = notifyType

	return outcome.err()
}

// sendRichMessage sends a Telegram Rich Message built from the configured
// template to every target, followed by any attachments exactly as they would
// be sent for a normal notification.
func (t *TelegramTarget) sendRichMessage(body, title string, notifyType NotifyType, attachments []Attachment) error {
	imageURL := ""
	if t.includeImage {
		imageURL = appriseImageURL(notifyType, "256x256")
	}
	richMessage, err := renderNotifyTemplateWithImageURL(
		t.templatePath, t.templateTokens, body, title, notifyType, imageURL)
	if err != nil {
		return err
	}

	// The template must describe a Rich Message: a non-empty 'blocks' list
	// whose every entry is an object with a 'type' string.
	blocks, ok := richMessage["blocks"].([]any)
	if !ok || len(blocks) == 0 {
		return fmt.Errorf("telegram rich message template must contain a non-empty 'blocks' list")
	}
	for _, block := range blocks {
		entry, ok := block.(map[string]any)
		if !ok {
			return fmt.Errorf("telegram rich message template contains a block missing a 'type' string")
		}
		if _, ok := entry["type"].(string); !ok {
			return fmt.Errorf("telegram rich message template contains a block missing a 'type' string")
		}
	}

	var outcome sendOutcome
	for _, recipient := range t.targets {
		payload := map[string]any{
			"rich_message": richMessage,
		}
		if recipient.isNumeric {
			payload["chat_id"] = recipient.chatIDInt
		} else {
			payload["chat_id"] = recipient.chatID
		}
		if recipient.messageTopic > 0 {
			payload["message_thread_id"] = recipient.messageTopic
		}
		if !t.preview {
			payload["link_preview_options"] = map[string]any{"is_disabled": true}
		}

		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		outcome.record(SendRequest(RequestSpec{
			Method: "POST",
			URL:    telegramAPIBase + t.botToken + "/sendRichMessage",
			Headers: map[string]string{
				"User-Agent":   "Apprise",
				"Content-Type": "application/json",
			},
			Body: string(data),
		}))

		// Attachments are untouched by Rich Message mode -- they are always
		// sent afterward, exactly as a normal notification would send them.
		for index, attachment := range attachments {
			spec, err := t.buildAttachmentSpec(recipient, attachment, "", index)
			if err != nil {
				return err
			}
			outcome.record(SendRequest(spec))
		}
	}

	return outcome.err()
}

func (t *TelegramTarget) buildSpec(body string, recipient telegramRecipient) (RequestSpec, error) {
	payload := map[string]any{
		"disable_notification":     t.silent,
		"disable_web_page_preview": !t.preview,
		"text":                     body,
	}
	if parseMode := t.parseMode(); parseMode != "" {
		payload["parse_mode"] = parseMode
		if parseMode == "HTML" {
			payload["text"] = rewriteTelegramHTML(body)
		}
	}

	if recipient.isNumeric {
		payload["chat_id"] = recipient.chatIDInt
	} else {
		payload["chat_id"] = recipient.chatID
	}
	if recipient.messageTopic > 0 {
		payload["message_thread_id"] = recipient.messageTopic
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    telegramAPIBase + t.botToken + "/sendMessage",
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/json",
		},
		Body: string(data),
	}, nil
}

func (t *TelegramTarget) buildAttachmentSpec(
	recipient telegramRecipient,
	attachment Attachment,
	caption string,
	index int,
) (RequestSpec, error) {
	route := telegramRouteFor(attachment.MIMEType)

	values := formFields{}
	if caption != "" {
		values.Set("caption", caption)
		values.Set("show_caption_above_media", telegramTitleCase(t.content == "before"))
		values.Set("parse_mode", t.parseMode())
	}
	values.Set("title", attachment.FileName(index, ".dat"))
	values.Set("chat_id", recipient.chatID)
	if recipient.messageTopic > 0 {
		values.Set("message_thread_id", strconv.Itoa(recipient.messageTopic))
	}

	// Telegram is handed a filename and a handle with no type, so the part
	// carries no content type of its own.
	requestBody, contentType, err := singleFileAttachmentBody(values, route.field, attachment, false)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    telegramAPIBase + t.botToken + "/" + route.method,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": contentType,
		},
		Body: requestBody,
	}, nil
}

func (t *TelegramTarget) buildDetectSpec() RequestSpec {
	return RequestSpec{
		Method: "POST",
		URL:    telegramAPIBase + t.botToken + "/getUpdates",
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/json",
			"Accept":       "*/*",
		},
		Body: "",
	}
}

func (t *TelegramTarget) buildImageSpec(recipient telegramRecipient) (RequestSpec, error) {
	values := url.Values{}
	if recipient.isNumeric {
		values.Set("chat_id", strconv.FormatInt(recipient.chatIDInt, 10))
	} else {
		values.Set("chat_id", recipient.chatID)
	}
	if recipient.messageTopic > 0 {
		values.Set("message_thread_id", strconv.Itoa(recipient.messageTopic))
	}

	return RequestSpec{
		Method: "POST",
		URL:    telegramAPIBase + t.botToken + "/SendPhoto",
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/x-www-form-urlencoded",
		},
		Body: values.Encode(),
	}, nil
}

func parseTelegramRecipient(raw string, defaultTopic *int) (telegramRecipient, bool) {
	entry := strings.TrimSpace(raw)
	if entry == "" {
		return telegramRecipient{}, false
	}

	topic := 0
	if defaultTopic != nil {
		topic = *defaultTopic
	}

	base, parsedTopic, ok := splitTelegramTopic(entry)
	if ok {
		entry = base
		topic = parsedTopic
	}

	entry = strings.TrimSpace(entry)
	if entry == "" {
		return telegramRecipient{}, false
	}

	if id, err := strconv.ParseInt(entry, 10, 64); err == nil {
		return telegramRecipient{
			chatID:       entry,
			chatIDInt:    id,
			isNumeric:    true,
			messageTopic: topic,
		}, true
	}

	if strings.HasPrefix(entry, "@") {
		return telegramRecipient{chatID: entry, messageTopic: topic}, true
	}

	return telegramRecipient{chatID: "@" + entry, messageTopic: topic}, true
}

func splitTelegramTopic(entry string) (string, int, bool) {
	parts := strings.SplitN(entry, ":", 2)
	if len(parts) != 2 {
		return entry, 0, false
	}
	base := strings.TrimSpace(parts[0])
	if base == "" {
		return entry, 0, false
	}
	value := strings.TrimSpace(parts[1])
	if value == "" {
		return entry, 0, false
	}
	topic, err := strconv.Atoi(value)
	if err != nil {
		return entry, 0, false
	}
	return base, topic, true
}

func parseBoolValue(raw string, fallback bool) bool {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "1", "true", "yes", "on", "y":
		return true
	case "0", "false", "no", "off", "n":
		return false
	default:
		return fallback
	}
}

func parseOptionalIntValue(raw string) *int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &value
}

func (t *TelegramTarget) parseMode() string {
	switch t.notifyFormat {
	case "html", "text":
		return "HTML"
	case "markdown":
		return t.markdownMode
	default:
		return ""
	}
}

func telegramMarkdownMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "v2", "markdownv2":
		return "MarkdownV2"
	default:
		return "MARKDOWN"
	}
}

func formatTelegramMessage(title, body, format, markdownMode string) string {
	if title == "" {
		if format == "text" || format == "markdown" {
			return body
		}
		return body
	}
	if body == "" {
		return formatTelegramTitle(title, format, markdownMode)
	}
	return formatTelegramTitle(title, format, markdownMode) + "\r\n" + body
}

func formatTelegramTitle(title, format, markdownMode string) string {
	switch format {
	case "html":
		return "<b>" + html.EscapeString(title) + "</b>"
	case "markdown":
		return "*" + escapeTelegramMarkdownTitle(title, markdownMode) + "*"
	default:
		return title
	}
}

func escapeTelegramMarkdownTitle(title, markdownMode string) string {
	if markdownMode == "MarkdownV2" {
		replacer := strings.NewReplacer(
			"\\", "\\\\",
			"_", "\\_",
			"*", "\\*",
			"[", "\\[",
			"]", "\\]",
			"(", "\\(",
			")", "\\)",
			"~", "\\~",
			"`", "\\`",
			">", "\\>",
			"#", "\\#",
			"+", "\\+",
			"-", "\\-",
			"=", "\\=",
			"|", "\\|",
			"{", "\\{",
			"}", "\\}",
			".", "\\.",
			"!", "\\!",
		)
		return replacer.Replace(title)
	}

	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"*", "\\*",
		"_", "\\_",
		"`", "\\`",
		"[", "\\[",
	)
	return replacer.Replace(title)
}

func init() {
	RegisterSchemaEntryOrdered(33, SchemaEntry{
		"attachment_support": true,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"content": map[string]any{
					"default":  "before",
					"map_to":   "content",
					"name":     "Content Placement",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"before", "after"},
				},
				"cto": map[string]any{
					"default":  4.0,
					"map_to":   "cto",
					"name":     "Socket Connect Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"detect": map[string]any{
					"default":  true,
					"map_to":   "detect_owner",
					"name":     "Detect Bot Owner",
					"private":  false,
					"required": false,
					"type":     "bool",
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
					"default":  "html",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"image": map[string]any{
					"default":  false,
					"map_to":   "include_image",
					"name":     "Include Image",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"mdv": map[string]any{
					"default":  "v1",
					"map_to":   "mdv",
					"name":     "Markdown Version",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"v1", "v2"},
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
				"preview": map[string]any{
					"default":  false,
					"map_to":   "preview",
					"name":     "Web Page Preview",
					"private":  false,
					"required": false,
					"type":     "bool",
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
				"silent": map[string]any{
					"default":  false,
					"map_to":   "silent",
					"name":     "Silent Notification",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"store": map[string]any{
					"default":  true,
					"map_to":   "store",
					"name":     "Persistent Storage",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"template": map[string]any{
					"map_to":   "template",
					"name":     "Rich Message Template Path",
					"private":  true,
					"required": false,
					"type":     "string",
				},
				"thread": map[string]any{
					"alias_of": "topic",
				},
				"to": map[string]any{
					"alias_of": "targets",
					"delim":    []string{",", " "},
				},
				"topic": map[string]any{
					"map_to":   "topic",
					"name":     "Topic Thread ID",
					"private":  false,
					"required": false,
					"type":     "int",
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
			"templates": []string{"{schema}://{bot_token}", "{schema}://{bot_token}/{targets}"},
			"tokens": map[string]any{
				"bot_token": map[string]any{
					"map_to":   "bot_token",
					"name":     "Bot Token",
					"private":  true,
					"regex":    []string{"^(bot)?(?P<key>[0-9]+:[a-z0-9_-]+)$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "tgram",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"tgram"},
				},
				"target_user": map[string]any{
					"map_to":   "targets",
					"name":     "Target Chat ID",
					"private":  false,
					"regex":    []string{"^((-?[0-9]{1,32})|([a-z_-][a-z0-9_-]+))$", "i"},
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_user"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
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
		"secure_protocols": []string{"tgram"},
		"service_name":     "Telegram",
		"service_url":      "https://telegram.org/",
		"setup_url":        "https://appriseit.com/services/telegram/",
	})
}

// telegramTitleCase renders a boolean the way Python's str() does, which is
// what the form field carries upstream.
func telegramTitleCase(value bool) string {
	if value {
		return "True"
	}

	return "False"
}
