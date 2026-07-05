package notify

import (
	"encoding/base64"
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
	avatarURL    string
	authType     string
	user         string
	password     string
	token        string
	notifyFormat string
	priority     string
	delay        string
	click        string
	email        string
	tags         []string
	actions      string
	attach       string
	filename     string
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

	user := strings.TrimSpace(target.User)
	password := strings.TrimSpace(target.Password)
	token := strings.TrimSpace(target.Query["token"])
	authType := strings.ToLower(strings.TrimSpace(target.Query["auth"]))
	if authType == "" {
		if token != "" {
			authType = "token"
		} else if user != "" {
			authType = "basic"
		}
	}
	if authType == "token" && token == "" {
		if password != "" {
			token = password
		} else if user != "" {
			token = user
		}
	}

	notifyFormat := normalizeNotifyFormat(target.Query["format"])
	if notifyFormat == "" {
		notifyFormat = "text"
	}

	return &NtfyTarget{
		target:       target,
		mode:         mode,
		topics:       topics,
		includeImage: includeImage,
		avatarURL:    strings.TrimSpace(target.Query["avatar_url"]),
		authType:     authType,
		user:         user,
		password:     password,
		token:        token,
		notifyFormat: notifyFormat,
		priority:     strings.TrimSpace(target.Query["priority"]),
		delay:        strings.TrimSpace(target.Query["delay"]),
		click:        strings.TrimSpace(target.Query["click"]),
		email:        strings.TrimSpace(target.Query["email"]),
		tags:         parseDelimitedList(target.Query["tags"]),
		actions:      strings.TrimSpace(target.Query["actions"]),
		attach:       strings.TrimSpace(target.Query["attach"]),
		filename:     strings.TrimSpace(target.Query["filename"]),
	}, nil
}

func (n *NtfyTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	requests, err := n.BuildRequestsWithAttachments(body, title, notifyType, nil)
	if err != nil {
		return RequestSpec{}, err
	}
	if len(requests) == 0 {
		return RequestSpec{}, fmt.Errorf("no requests")
	}
	return requests[0], nil
}

func (n *NtfyTarget) BuildRequestsWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) ([]RequestSpec, error) {
	if len(n.topics) == 0 {
		return nil, fmt.Errorf("no topics")
	}

	if len(attachments) > 0 {
		specs := []RequestSpec{}
		for _, topic := range n.topics {
			for i, attachment := range attachments {
				sendBody := ""
				sendTitle := ""
				if i == 0 {
					sendBody = body
					sendTitle = title
				}
				spec, err := n.buildAttachmentRequest(topic, attachment, sendBody, sendTitle, notifyType)
				if err != nil {
					return nil, err
				}
				specs = append(specs, spec)
			}
		}
		return specs, nil
	}

	urlStr, err := n.notifyURL()
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"topic":   n.topics[0],
		"title":   title,
		"message": body,
	}
	if n.attach != "" {
		payload["attach"] = n.attach
		if n.filename != "" {
			payload["filename"] = n.filename
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	headers := n.headers(notifyType)
	headers["Content-Type"] = "application/json"

	return []RequestSpec{{
		Method:  "POST",
		URL:     urlStr,
		Headers: headers,
		Body:    string(data),
	}}, nil
}

func (n *NtfyTarget) headers(notifyType NotifyType) map[string]string {
	headers := map[string]string{
		"User-Agent": "Apprise",
		"Accept":     "*/*",
	}

	if n.mode == NtfyModePrivate {
		if n.authType == "basic" && n.user != "" {
			pass := n.password
			if pass == "" {
				pass = "None"
			}
			encoded := base64.StdEncoding.EncodeToString([]byte(n.user + ":" + pass))
			headers["Authorization"] = "Basic " + encoded
		} else if n.authType == "token" && n.token != "" {
			headers["Authorization"] = "Bearer " + n.token
		}
	}

	_ = notifyType
	if n.includeImage {
		icon := n.avatarURL
		if icon == "" {
			icon = appriseImageURL(notifyType, "256x256")
		}
		headers["X-Icon"] = icon
	}
	if n.notifyFormat == "markdown" {
		headers["X-Markdown"] = "yes"
	}
	if n.priority != "" && strings.ToLower(n.priority) != "default" {
		headers["X-Priority"] = n.priority
	}
	if n.delay != "" {
		headers["X-Delay"] = n.delay
	}
	if n.click != "" {
		headers["X-Click"] = n.click
	}
	if n.email != "" {
		headers["X-Email"] = n.email
	}
	if len(n.tags) > 0 {
		headers["X-Tags"] = strings.Join(n.tags, ",")
	}
	if n.actions != "" {
		headers["X-Actions"] = n.actions
	}
	return headers
}

func (n *NtfyTarget) buildAttachmentRequest(topic string, attachment Attachment, body, title string, notifyType NotifyType) (RequestSpec, error) {
	urlStr, err := n.attachmentURL(topic)
	if err != nil {
		return RequestSpec{}, err
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return RequestSpec{}, err
	}
	values := u.Query()
	values.Set("filename", attachment.Name)
	if title != "" {
		values.Set("title", title)
	}
	if body != "" {
		values.Set("message", body)
	}
	u.RawQuery = values.Encode()

	return RequestSpec{
		Method:  "POST",
		URL:     u.String(),
		Headers: n.headers(notifyType),
		Body:    string(attachment.Data),
	}, nil
}

func (n *NtfyTarget) Send(body, title string, notifyType NotifyType) error {
	return n.SendWithAttachments(body, title, notifyType, nil)
}

func (n *NtfyTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	specs, err := n.BuildRequestsWithAttachments(body, title, notifyType, attachments)
	if err != nil {
		return err
	}

	for _, spec := range specs {
		if err := SendRequest(spec); err != nil {
			return err
		}
	}
	return nil
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

func (n *NtfyTarget) attachmentURL(topic string) (string, error) {
	if n.mode == NtfyModeCloud {
		u := url.URL{Scheme: "https", Host: "ntfy.sh", Path: "/" + topic}
		return u.String(), nil
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
	u := url.URL{Scheme: scheme, Host: host, Path: "/" + topic}
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
