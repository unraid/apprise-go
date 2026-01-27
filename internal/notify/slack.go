package notify

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	slackModeWebhook = "hook"
	slackModeGov     = "gov-hook"
	slackModeBot     = "bot"
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
}

func NewSlackTarget(target *ParsedURL) (*SlackTarget, error) {
	token := strings.TrimSpace(target.Host)
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	mode := strings.ToLower(strings.TrimSpace(target.Query["mode"]))
	if mode != "" {
		mode = slackNormalizeMode(mode)
		if mode == "" {
			return nil, fmt.Errorf("unsupported mode: %s", target.Query["mode"])
		}
	}

	entries := splitPath(target.Path)
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

	return &SlackTarget{
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
	}, nil
}

func (s *SlackTarget) Send(body, title string, notifyType NotifyType) error {
	channels := s.targets
	if len(channels) == 0 {
		channels = []string{""}
	}

	for _, rawChannel := range channels {
		payload, err := s.buildPayload(body, title, notifyType)
		if err != nil {
			return err
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
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
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
	username := s.username
	if username == "" {
		username = "Apprise"
	}

	payload := map[string]any{
		"username": username,
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
				"text": body,
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
		payload["mrkdwn"] = true
		attachment := map[string]any{
			"title": title,
			"text":  body,
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
