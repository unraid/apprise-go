package notify

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const (
	slackModeWebhook  = "hook"
	slackModeGov      = "gov-hook"
	slackModeBot      = "bot"
	slackModeWorkflow = "workflow"
	slackModeTrigger  = "trigger"
)

// Workflow Builder posts to a fixed endpoint built from path segments rather
// than to a channel.
const (
	slackWorkflowURL = "https://hooks.slack.com/workflows"
	slackTriggerURL  = "https://hooks.slack.com/triggers"
)

var slackListDelims = regexp.MustCompile(`[ \t\r\n,#\\/]+`)
var slackChannelRegex = regexp.MustCompile(`(?i)^([+#@]?[A-Z0-9_-]{1,32})(?::([0-9.]+))?$`)

type SlackTarget struct {
	tokenA           string
	tokenB           string
	tokenC           string
	accessToken      string
	mode             string
	username         string
	includeImage     bool
	includeFooter    bool
	includeTimestamp bool
	useBlocks        bool
	targets          []string
	workflowPath     []string
	templatePath     string
	templateTokens   map[string]string

	// notifyFormat decides whether the payload claims markdown. Slack
	// renders the text differently depending on it, so ?format=text is not
	// cosmetic.
	notifyFormat string
}

func NewSlackTarget(target *ParsedURL) (*SlackTarget, error) {
	token := strings.TrimSpace(target.Host)
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	notifyFormat := strings.ToLower(strings.TrimSpace(target.Query["format"]))
	if notifyFormat == "" {
		// Slack is markdown-native upstream.
		notifyFormat = "markdown"
	}

	mode := strings.ToLower(strings.TrimSpace(target.Query["mode"]))
	if mode != "" {
		mode = slackNormalizeMode(mode)
		if mode == "" {
			return nil, fmt.Errorf("unsupported mode: %s", target.Query["mode"])
		}
	}

	entries := splitPath(target.Path)

	// Workflow Builder consumes every path segment; nothing here is a token
	// or a channel, so this branch has to come before either is read.
	if mode == slackModeWorkflow || mode == slackModeTrigger {
		workflowPath := append([]string{token}, entries...)

		// A workflow URL carries four segments and a trigger three; with the
		// mode named, the count has to match it exactly.
		expected := 4
		if mode == slackModeTrigger {
			expected = 3
		}
		if len(workflowPath) != expected {
			return nil, fmt.Errorf("a slack %s url requires exactly %d path segments, got %d",
				mode, expected, len(workflowPath))
		}

		templateTokens := map[string]string{}
		for key, value := range target.QueryPayload {
			templateTokens[key] = value
		}

		return &SlackTarget{
			notifyFormat:   notifyFormat,
			mode:           mode,
			username:       strings.TrimSpace(target.User),
			workflowPath:   workflowPath,
			templatePath:   strings.TrimSpace(target.Query["template"]),
			templateTokens: templateTokens,
		}, nil
	}

	tokenA := token
	tokenB := ""
	tokenC := ""
	accessToken := ""

	override := strings.TrimSpace(target.Query["token"])
	if override != "" {
		tokenEntries := splitSlackList(override)
		if len(tokenEntries) > 0 {
			if strings.HasPrefix(tokenEntries[0], "xo") {
				accessToken = tokenEntries[0]
			}
			if accessToken == "" {
				tokenA = tokenEntries[0]
				if len(tokenEntries) > 1 {
					tokenB = tokenEntries[1]
				}
				if len(tokenEntries) > 2 {
					tokenC = tokenEntries[2]
				}
			}
		}
	} else {
		if strings.HasPrefix(tokenA, "xo") {
			accessToken = tokenA
		}
		if accessToken == "" {
			if len(entries) > 0 {
				tokenB = entries[0]
			}
			if len(entries) > 1 {
				tokenC = entries[1]
			}
			if len(entries) > 2 {
				entries = entries[2:]
			} else {
				entries = nil
			}
		}
	}

	targets := entries
	if toValue, ok := target.Query["to"]; ok && strings.TrimSpace(toValue) != "" {
		targets = append(targets, splitSlackList(toValue)...)
	}

	includeImage := parseBool(target.Query["image"], true)
	includeFooter := parseBool(target.Query["footer"], true)
	includeTimestamp := parseBool(target.Query["timestamp"], true)
	useBlocks := parseBool(target.Query["blocks"], false)

	if accessToken != "" && mode == "" {
		mode = slackModeBot
	}
	if mode == "" {
		mode = slackModeWebhook
	}
	if mode == slackModeBot && accessToken == "" {
		return nil, fmt.Errorf("missing bot token")
	}
	if mode != slackModeBot && (tokenB == "" || tokenC == "") {
		return nil, fmt.Errorf("missing webhook credentials")
	}

	templateTokens := map[string]string{}
	for key, value := range target.QueryPayload {
		templateTokens[key] = value
	}

	return &SlackTarget{
		notifyFormat:     notifyFormat,
		tokenA:           tokenA,
		tokenB:           tokenB,
		tokenC:           tokenC,
		accessToken:      accessToken,
		mode:             mode,
		username:         strings.TrimSpace(target.User),
		includeImage:     includeImage,
		includeFooter:    includeFooter,
		includeTimestamp: includeTimestamp,
		useBlocks:        useBlocks,
		targets:          targets,
		templatePath:     strings.TrimSpace(target.Query["template"]),
		templateTokens:   templateTokens,
	}, nil
}

// isWorkflow reports whether this target posts to Workflow Builder, which has
// no channels and a completely different payload.
func (s *SlackTarget) isWorkflow() bool {
	return s.mode == slackModeWorkflow || s.mode == slackModeTrigger
}

func (s *SlackTarget) workflowSpec(body, title string, notifyType NotifyType) (RequestSpec, error) {
	base := slackWorkflowURL
	if s.mode == slackModeTrigger {
		base = slackTriggerURL
	}

	var payload map[string]any
	if s.templatePath != "" {
		rendered, err := renderNotifyTemplate(s.templatePath, s.templateTokens, body, title, notifyType, "72x72")
		if err != nil {
			return RequestSpec{}, err
		}
		payload = rendered
	} else {
		// The workflow has to accept this variable; there is no channel or
		// block structure to attach anything else to.
		text := body
		if title != "" {
			text = title + ": " + body
		}
		payload = map[string]any{"text": text}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    base + "/" + strings.Join(s.workflowPath, "/"),
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "application/json",
			"Content-Type": "application/json; charset=utf-8",
		},
		Body: string(data),
	}, nil
}

func (s *SlackTarget) Send(body, title string, notifyType NotifyType) error {
	return s.SendWithAttachments(body, title, notifyType, nil)
}

// SendWithAttachments posts the message, then uploads each file through
// Slack's external upload flow: ask for an upload URL, PUT the bytes there,
// then complete the upload against each channel.
func (s *SlackTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	// The channels a file is completed against come from what the message
	// posts report back, not from what was configured: Slack answers with the
	// resolved channel id, and the upload has to name that.
	channels, err := s.sendMessagesCollectingChannels(body, title, notifyType)
	if err != nil {
		return err
	}

	// Only a bot token can upload, and there is nowhere to put a file until
	// a message has told us which channel it landed in.
	if len(attachments) == 0 || s.mode != slackModeBot || len(channels) == 0 {
		return nil
	}

	for index, attachment := range attachments {
		if err := s.uploadAttachment(attachment, index, channels); err != nil {
			return err
		}
	}

	return nil
}

// uploadAttachment runs the three step external upload for one file.
func (s *SlackTarget) uploadAttachment(attachment Attachment, index int, channels []string) error {
	name := attachment.FileName(index, ".dat")

	query := url.Values{}
	query.Set("filename", name)
	query.Set("length", strconv.Itoa(len(attachment.Data)))

	var upload struct {
		FileID    string `json:"file_id"`
		UploadURL string `json:"upload_url"`
	}
	if err := doJSONRequest(RequestSpec{
		Method: "GET",
		URL:    "https://slack.com/api/files.getUploadURLExternal?" + query.Encode(),
		// Upstream sends the same headers for every call, so this GET
		// carries a content type despite having no body.
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Accept":        "application/json",
			"Authorization": "Bearer " + s.accessToken,
			"Content-Type":  "application/json; charset=utf-8",
		},
		// Upstream posts an empty JSON object here rather than nothing.
		Body: "{}",
	}, &upload); err != nil {
		return err
	}
	if upload.FileID == "" || upload.UploadURL == "" {
		return fmt.Errorf("slack did not return an upload url")
	}

	// Slack is handed a filename and a handle with no type, so the part
	// carries no content type of its own.
	uploadBody, contentType, err := singleFileAttachmentBody(
		formFields{}, "file",
		Attachment{Name: name, Data: attachment.Data}, false)
	if err != nil {
		return err
	}

	if err := SendRequest(RequestSpec{
		Method: "POST",
		URL:    upload.UploadURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Accept":        "application/json",
			"Authorization": "Bearer " + s.accessToken,
			"Content-Type":  contentType,
		},
		Body: uploadBody,
	}); err != nil {
		return err
	}

	// The file exists once uploaded but is not visible until it is completed
	// against a channel.
	for _, channel := range channels {
		data, err := json.Marshal(map[string]any{
			"files": []any{
				map[string]any{"id": upload.FileID, "title": attachment.Name},
			},
			"channel_id": channel,
		})
		if err != nil {
			return err
		}

		if err := SendRequest(RequestSpec{
			Method: "POST",
			URL:    "https://slack.com/api/files.completeUploadExternal",
			Headers: map[string]string{
				"User-Agent":    "Apprise",
				"Accept":        "application/json",
				"Authorization": "Bearer " + s.accessToken,
				"Content-Type":  "application/json; charset=utf-8",
			},
			Body: string(data),
		}); err != nil {
			return err
		}
	}

	return nil
}

func (s *SlackTarget) sendMessagesCollectingChannels(body, title string, notifyType NotifyType) ([]string, error) {
	if s.isWorkflow() {
		spec, err := s.workflowSpec(body, title, notifyType)
		if err != nil {
			return nil, err
		}

		return nil, SendRequest(spec)
	}

	posted := []string{}

	channels := s.targets
	if len(channels) == 0 {
		channels = []string{""}
	}

	for _, rawChannel := range channels {
		payload, err := s.buildPayload(body, title, notifyType)
		if err != nil {
			return nil, err
		}

		channel := strings.TrimSpace(rawChannel)
		if channel == "" {
			if s.mode == slackModeBot {
				payload["channel"] = "#general"
			}
		} else if isSimpleEmail(channel) {
			if s.mode != slackModeBot {
				continue
			}
			userID := s.lookupUserID(channel)
			if userID == "" {
				continue
			}
			payload["channel"] = userID
		} else {
			normalized, thread, ok := parseSlackTarget(channel)
			if !ok {
				continue
			}
			payload["channel"] = normalized
			if thread != "" {
				payload["thread_ts"] = thread
			}
		}

		spec, err := s.buildRequestSpec(payload)
		if err != nil {
			return nil, err
		}

		// Only the bot API answers with JSON; a webhook replies with the
		// literal text "ok", which is not decodable and carries no channel.
		if s.mode != slackModeBot {
			if err := SendRequest(spec); err != nil {
				return nil, err
			}
			continue
		}

		var response struct {
			Channel string `json:"channel"`
		}
		if err := doJSONRequest(spec, &response); err != nil {
			return nil, err
		}
		if response.Channel != "" {
			posted = append(posted, response.Channel)
		}
	}

	return posted, nil
}

func (s *SlackTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload, err := s.buildPayload(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}

	if len(s.targets) > 0 {
		channel := strings.TrimSpace(s.targets[0])
		if channel != "" {
			if isSimpleEmail(channel) {
				if s.mode == slackModeBot {
					userID := s.lookupUserID(channel)
					if userID != "" {
						payload["channel"] = userID
					}
				}
			} else if normalized, thread, ok := parseSlackTarget(channel); ok {
				payload["channel"] = normalized
				if thread != "" {
					payload["thread_ts"] = thread
				}
			}
		}
	} else if s.mode == slackModeBot {
		payload["channel"] = "#general"
	}

	return s.buildRequestSpec(payload)
}

func (s *SlackTarget) buildPayload(body, title string, notifyType NotifyType) (map[string]any, error) {
	// Upstream only names the poster when the URL supplies a user.
	payload := map[string]any{}
	if s.username != "" {
		payload["username"] = s.username
	}

	imageURL := ""
	if s.includeImage {
		imageURL = appriseImageURL(notifyType, "72x72")
	}

	if s.useBlocks {
		blockText := map[string]any{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": commonMarkToSlack(body),
			},
		}
		blocks := []any{blockText}
		if title != "" {
			header := map[string]any{
				"type": "header",
				"text": map[string]any{
					"type":  "plain_text",
					"text":  title,
					"emoji": true,
				},
			}
			blocks = append([]any{header}, blocks...)
		}

		if s.includeFooter {
			footer := map[string]any{
				"type": "context",
				"elements": []any{
					map[string]any{
						"type": "mrkdwn",
						"text": "Apprise",
					},
				},
			}
			if imageURL != "" {
				payload["icon_url"] = imageURL
				footer["elements"] = append([]any{
					map[string]any{
						"type":      "image",
						"image_url": imageURL,
						"alt_text":  string(notifyType),
					},
				}, footer["elements"].([]any)...)
			}
			blocks = append(blocks, footer)
		}

		payload["attachments"] = []any{
			map[string]any{
				"blocks": blocks,
				"color":  appriseColor(notifyType),
			},
		}
	} else {
		// Upstream reports whether the body is markdown rather than always
		// claiming it is; ?format=html or text turns this off, and Slack
		// renders the text differently as a result.
		payload["mrkdwn"] = s.notifyFormat == "markdown"
		attachment := map[string]any{
			"title": title,
			"text":  commonMarkToSlack(body),
			"color": appriseColor(notifyType),
		}
		if imageURL != "" {
			payload["icon_url"] = imageURL
		}
		if s.includeFooter {
			attachment["footer"] = "Apprise"
			if imageURL != "" {
				attachment["footer_icon"] = imageURL
			}
			if s.includeTimestamp {
				attachment["ts"] = json.Number(fmt.Sprintf("%.1f", float64(fixedTime().Unix())))
			}
		}
		payload["attachments"] = []any{attachment}
	}

	return payload, nil
}

func (s *SlackTarget) buildRequestSpec(payload map[string]any) (RequestSpec, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "application/json",
		"Content-Type": "application/json; charset=utf-8",
	}

	url := ""
	switch s.mode {
	case slackModeGov:
		url = fmt.Sprintf("https://hooks.slack-gov.com/services/%s/%s/%s", s.tokenA, s.tokenB, s.tokenC)
	case slackModeBot:
		url = "https://slack.com/api/chat.postMessage"
		headers["Authorization"] = "Bearer " + s.accessToken
	default:
		url = fmt.Sprintf("https://hooks.slack.com/services/%s/%s/%s", s.tokenA, s.tokenB, s.tokenC)
	}

	return RequestSpec{
		Method:  "POST",
		URL:     url,
		Headers: headers,
		Body:    string(data),
	}, nil
}

func slackNormalizeMode(mode string) string {
	lower := strings.ToLower(mode)
	switch {
	case strings.HasPrefix(lower, "gov"):
		return slackModeGov
	case strings.HasPrefix(lower, "bot"):
		return slackModeBot
	case strings.HasPrefix(lower, "hook"):
		return slackModeWebhook
	case strings.HasPrefix(lower, "workflow"):
		return slackModeWorkflow
	case strings.HasPrefix(lower, "trigger"):
		return slackModeTrigger
	default:
		return ""
	}
}

func parseSlackTarget(raw string) (string, string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", false
	}
	match := slackChannelRegex.FindStringSubmatch(trimmed)
	if match == nil {
		return "", "", false
	}
	channel := match[1]
	thread := ""
	if len(match) > 2 {
		thread = match[2]
	}
	if channel == "" {
		return "", thread, false
	}
	if strings.HasPrefix(channel, "+") {
		channel = channel[1:]
	} else if !strings.HasPrefix(channel, "#") && !strings.HasPrefix(channel, "@") {
		channel = "#" + channel
	}
	return channel, thread, true
}

func splitSlackList(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	parts := slackListDelims.Split(trimmed, -1)
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values = append(values, part)
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func (s *SlackTarget) lookupUserID(email string) string {
	if s.accessToken == "" {
		return ""
	}

	endpoint := "https://slack.com/api/users.lookupByEmail"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}

	query := url.Values{}
	query.Set("email", email)
	req.URL.RawQuery = query.Encode()
	req.Header.Set("User-Agent", "Apprise")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+s.accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var payload struct {
		OK   bool `json:"ok"`
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ""
	}
	if !payload.OK {
		return ""
	}
	return payload.User.ID
}

func init() {
	RegisterSchemaOverride("slack", applySlackOverrides)
}

func applySlackOverrides(target *ParsedURL, values map[string]SchemaValue) {
	if rawToken := strings.TrimSpace(target.Query["token"]); rawToken != "" {
		entries := splitSlackList(rawToken)
		if len(entries) > 0 && strings.HasPrefix(entries[0], "xo") {
			values["access_token"] = schemaValueString(entries[0])
			values["token_a"] = schemaValueAny(nil)
			values["token_b"] = schemaValueAny(nil)
			values["token_c"] = schemaValueAny(nil)
		} else {
			if len(entries) > 0 {
				values["token_a"] = schemaValueString(entries[0])
			}
			if len(entries) > 1 {
				values["token_b"] = schemaValueString(entries[1])
			}
			if len(entries) > 2 {
				values["token_c"] = schemaValueString(entries[2])
			}
			values["access_token"] = schemaValueAny(nil)
		}
	}
}
