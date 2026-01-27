package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const ntfyCloudURL = "https://ntfy.sh/"

type NtfyMode string

const (
	NtfyModeCloud   NtfyMode = "cloud"
	NtfyModePrivate NtfyMode = "private"
)

type NtfyTarget struct {
	target       *ParsedURL
	mode         NtfyMode
	topics       []string
	includeImage bool
}

func NewNtfyTarget(target *ParsedURL) (*NtfyTarget, error) {
	mode := NtfyModePrivate
	if rawMode, ok := target.Query["mode"]; ok && rawMode != "" {
		switch strings.ToLower(rawMode) {
		case string(NtfyModeCloud):
			mode = NtfyModeCloud
		case string(NtfyModePrivate):
			mode = NtfyModePrivate
		}
	}

	topics := splitPath(target.Path)
	if len(topics) == 0 {
		mode = NtfyModeCloud
		if target.Host != "" {
			topics = []string{target.Host}
		}
	}

	includeImage := true
	if rawImage, ok := target.Query["image"]; ok {
		includeImage = parseBool(rawImage, true)
	}

	return &NtfyTarget{
		target:       target,
		mode:         mode,
		topics:       topics,
		includeImage: includeImage,
	}, nil
}

func (n *NtfyTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(n.topics) == 0 {
		return RequestSpec{}, fmt.Errorf("no topics")
	}

	urlStr, err := n.notifyURL()
	if err != nil {
		return RequestSpec{}, err
	}

	payload := map[string]any{
		"topic":   n.topics[0],
		"title":   title,
		"message": body,
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

	_ = notifyType
	if n.includeImage {
		// TODO: attach image when asset support is added.
	}

	return RequestSpec{
		Method:  "POST",
		URL:     urlStr,
		Headers: headers,
		Body:    string(data),
	}, nil
}

func (n *NtfyTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := n.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (n *NtfyTarget) notifyURL() (string, error) {
	if n.mode == NtfyModeCloud {
		return ntfyCloudURL, nil
	}

	scheme := "http"
	if strings.ToLower(n.target.Scheme) == "ntfys" {
		scheme = "https"
	}

	host := n.target.Host
	if host == "" {
		return "", fmt.Errorf("missing host")
	}

	if n.target.Port != 0 {
		host = fmt.Sprintf("%s:%d", host, n.target.Port)
	}

	u := url.URL{Scheme: scheme, Host: host, Path: "/"}
	return u.String(), nil
}

func parseBool(value string, fallback bool) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "1", "true", "yes", "on", "y":
		return true
	case "0", "false", "no", "off", "n":
		return false
	default:
		return fallback
	}
}
