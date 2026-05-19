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
	targets      []telegramRecipient
	notifyFormat string
	markdownMode string
	silent       bool
	preview      bool
	detect       bool
	includeImage bool
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

	format := normalizeNotifyFormat(target.Query["format"])
	if format == "" {
		format = "html"
	}
	switch format {
	case "html", "markdown", "text":
	default:
		return nil, fmt.Errorf("invalid format")
	}

	return &TelegramTarget{
		botToken:     botToken,
		targets:      targets,
		notifyFormat: format,
		markdownMode: telegramMarkdownMode(target.Query["mdv"]),
		silent:       parseBoolValue(target.Query["silent"], false),
		preview:      parseBoolValue(target.Query["preview"], false),
		detect:       detect,
		includeImage: parseBoolValue(target.Query["image"], false),
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
	if len(t.targets) == 0 {
		if t.detect {
			return SendRequest(t.buildDetectSpec())
		}
		return nil
	}

	message := formatTelegramMessage(title, body, t.notifyFormat, t.markdownMode)
	for _, recipient := range t.targets {
		if t.includeImage {
			spec, err := t.buildImageSpec(recipient)
			if err != nil {
				return err
			}
			if err := SendRequest(spec); err != nil {
				return err
			}
		}
		spec, err := t.buildSpec(message, recipient)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = notifyType

	return nil
}

func (t *TelegramTarget) buildSpec(body string, recipient telegramRecipient) (RequestSpec, error) {
	payload := map[string]any{
		"disable_notification":     t.silent,
		"disable_web_page_preview": !t.preview,
		"text":                     body,
	}
	if parseMode := t.parseMode(); parseMode != "" {
		payload["parse_mode"] = parseMode
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
					"default":  4,
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
				"rto": map[string]any{
					"default":  4,
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
			},
			"kwargs":    map[string]any{},
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
