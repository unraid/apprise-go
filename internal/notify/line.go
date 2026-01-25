package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const lineURL = "https://api.line.me/v2/bot/message/push"

var lineTargetDelimiters = regexp.MustCompile(`[ \t\r\n,#\\/]+`)

type LineTarget struct {
	token        string
	targets      []string
	includeImage bool
}

func NewLineTarget(target *ParsedURL) (*LineTarget, error) {
	targets := splitPathSegments(target.Path)

	token := target.Query["token"]
	if token == "" {
		token = target.Host
		if token != "" && !strings.HasSuffix(token, "=") {
			for i, entry := range targets {
				if strings.HasSuffix(entry, "=") {
					token = token + "/" + strings.Join(targets[:i+1], "/")
					targets = targets[i+1:]
					break
				}
			}
		}
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("missing token")
	}

	includeImage := parseBool(target.Query["image"], true)

	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		for _, entry := range lineTargetDelimiters.Split(toValue, -1) {
			entry = strings.TrimSpace(entry)
			if entry != "" {
				targets = append(targets, entry)
			}
		}
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	return &LineTarget{
		token:        token,
		targets:      targets,
		includeImage: includeImage,
	}, nil
}

func (l *LineTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(l.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	return l.buildRequestForTarget(l.targets[0], body, title, notifyType)
}

func (l *LineTarget) Send(body, title string, notifyType NotifyType) error {
	if len(l.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	for _, target := range l.targets {
		spec, err := l.buildRequestForTarget(target, body, title, notifyType)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func (l *LineTarget) buildRequestForTarget(target, body, title string, notifyType NotifyType) (RequestSpec, error) {
	message := body
	if title != "" {
		message = title + "\r\n" + body
	}

	sender := map[string]string{
		"name": "Apprise",
	}
	if l.includeImage {
		sender["iconUrl"] = appriseImageURL(notifyType, "128x128")
	}

	payload := map[string]any{
		"to": target,
		"messages": []map[string]any{
			{
				"type":   "text",
				"text":   message,
				"sender": sender,
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Accept":        "*/*",
		"Content-Type":  "application/json",
		"Authorization": fmt.Sprintf("Bearer %s", l.token),
	}

	return RequestSpec{
		Method:  "POST",
		URL:     lineURL,
		Headers: headers,
		Body:    string(data),
	}, nil
}
