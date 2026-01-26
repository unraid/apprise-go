package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const pushedURL = "https://api.pushed.co/1/push"

type PushedTarget struct {
	appKey    string
	appSecret string
}

func NewPushedTarget(target *ParsedURL) (*PushedTarget, error) {
	appKey := target.Host
	segments := splitPath(target.Path)
	if appKey == "" || len(segments) == 0 {
		return nil, fmt.Errorf("missing app credentials")
	}
	appSecret := segments[0]

	return &PushedTarget{
		appKey:    appKey,
		appSecret: appSecret,
	}, nil
}

func (p *PushedTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if title != "" {
		if body != "" {
			body = title + "\r\n" + body
		} else {
			body = title
		}
	}

	payload := map[string]any{
		"app_key":     p.appKey,
		"app_secret":  p.appSecret,
		"target_type": "app",
		"content":     body,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/json",
	}

	_ = title
	_ = notifyType

	return RequestSpec{
		Method:  "POST",
		URL:     pushedURL,
		Headers: headers,
		Body:    string(data),
	}, nil
}

func (p *PushedTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := p.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}
