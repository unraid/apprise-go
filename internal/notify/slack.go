package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	slackModeWebhook = "hook"
	slackModeGov     = "gov-hook"
)

var slackListDelims = regexp.MustCompile(`[ \t\r\n,#\\/]+`)

type SlackTarget struct {
	tokenA           string
	tokenB           string
	tokenC           string
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

	override := strings.TrimSpace(target.Query["token"])
	if override != "" {
		tokenEntries := splitSlackList(override)
		if len(tokenEntries) > 0 {
			if strings.HasPrefix(tokenEntries[0], "xo") {
				return nil, fmt.Errorf("bot mode unsupported")
			}
			tokenA = tokenEntries[0]
			if len(tokenEntries) > 1 {
				tokenB = tokenEntries[1]
			}
			if len(tokenEntries) > 2 {
				tokenC = tokenEntries[2]
			}
		}
	} else {
		if strings.HasPrefix(tokenA, "xo") {
			return nil, fmt.Errorf("bot mode unsupported")
		}
		if len(entries) > 0 {
			tokenB = entries[0]
		}
		if len(entries) > 1 {
			tokenC = entries[1]
		}
		entries = entries[2:]
	}

	targets := entries
	if toValue, ok := target.Query["to"]; ok && strings.TrimSpace(toValue) != "" {
		targets = append(targets, splitSlackList(toValue)...)
	}

	includeImage := parseBool(target.Query["image"], true)
	includeFooter := parseBool(target.Query["footer"], true)
	includeTimestamp := parseBool(target.Query["timestamp"], true)
	useBlocks := parseBool(target.Query["blocks"], false)

	if mode == "" {
		mode = slackModeWebhook
	}

	return &SlackTarget{
		tokenA:           tokenA,
		tokenB:           tokenB,
		tokenC:           tokenC,
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
	spec, err := s.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (s *SlackTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
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
				attachment["ts"] = time.Now().Unix()
			}
		}
		payload["attachments"] = []any{attachment}
	}

	if len(s.targets) > 0 {
		payload["channel"] = s.targets[0]
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	url := fmt.Sprintf("https://hooks.slack.com/services/%s/%s/%s", s.tokenA, s.tokenB, s.tokenC)
	if s.mode == slackModeGov {
		url = fmt.Sprintf("https://hooks.slack-gov.com/services/%s/%s/%s", s.tokenA, s.tokenB, s.tokenC)
	}

	return RequestSpec{
		Method: "POST",
		URL:    url,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "application/json",
			"Content-Type": "application/json; charset=utf-8",
		},
		Body: string(data),
	}, nil
}

func slackNormalizeMode(mode string) string {
	lower := strings.ToLower(mode)
	switch {
	case strings.HasPrefix(lower, "gov"):
		return slackModeGov
	case strings.HasPrefix(lower, "hook"):
		return slackModeWebhook
	default:
		return ""
	}
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
