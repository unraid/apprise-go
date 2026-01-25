package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	notificationAPIRegionUS = "us"
	notificationAPIRegionCA = "ca"
	notificationAPIRegionEU = "eu"
)

const (
	notificationAPIModeTemplate = "template"
	notificationAPIModeMessage  = "message"
)

var notificationAPIRegionURLs = map[string]string{
	notificationAPIRegionUS: "https://api.notificationapi.com",
	notificationAPIRegionCA: "https://api.ca.notificationapi.com",
	notificationAPIRegionEU: "https://api.eu.notificationapi.com",
}

var notificationAPIChannels = map[string]struct{}{
	"email":       {},
	"sms":         {},
	"inapp":       {},
	"web_push":    {},
	"mobile_push": {},
	"slack":       {},
}

type notificationAPITargetEntry struct {
	id     string
	email  string
	number string
}

type NotificationAPITarget struct {
	clientID     string
	clientSecret string
	messageType  string
	mode         string
	region       string
	channels     []string
	targets      []notificationAPITargetEntry
	cc           map[string]struct{}
	bcc          map[string]struct{}
	fromAddr     string
	fromName     string
	tokens       map[string]string
}

func NewNotificationAPITarget(target *ParsedURL) (*NotificationAPITarget, error) {
	entries := []string{}
	if strings.TrimSpace(target.Host) != "" {
		entries = append(entries, target.Host)
	}
	entries = append(entries, splitPath(target.Path)...)

	clientID := strings.TrimSpace(target.Query["id"])
	if clientID == "" && len(entries) > 0 {
		clientID = strings.TrimSpace(entries[0])
		entries = entries[1:]
	}
	if clientID == "" {
		return nil, fmt.Errorf("missing client id")
	}

	clientSecret := strings.TrimSpace(target.Query["secret"])
	if clientSecret == "" && len(entries) > 0 {
		clientSecret = strings.TrimSpace(entries[0])
		entries = entries[1:]
	}
	if clientSecret == "" {
		return nil, fmt.Errorf("missing client secret")
	}

	fromAddr := strings.TrimSpace(target.Query["from"])
	fromName := ""
	if fromAddr != "" {
		if !isSimpleEmail(fromAddr) {
			fromAddr = ""
		}
	}

	messageType := strings.TrimSpace(target.Query["type"])
	if messageType == "" {
		messageType = strings.TrimSpace(target.User)
	}

	mode := strings.ToLower(strings.TrimSpace(target.Query["mode"]))
	if mode != "" && mode != notificationAPIModeTemplate && mode != notificationAPIModeMessage {
		return nil, fmt.Errorf("invalid mode: %s", mode)
	}

	if mode == "" {
		if messageType == "" {
			mode = notificationAPIModeMessage
		} else {
			mode = notificationAPIModeTemplate
		}
	}

	if messageType == "" {
		messageType = "apprise"
	}

	region := strings.ToLower(strings.TrimSpace(target.Query["region"]))
	if region == "" {
		region = notificationAPIRegionUS
	}
	if _, ok := notificationAPIRegionURLs[region]; !ok {
		return nil, fmt.Errorf("invalid region: %s", region)
	}

	channelSet := map[string]struct{}{}
	if rawChannels := strings.TrimSpace(target.Query["channels"]); rawChannels != "" {
		for _, entry := range parseDelimitedList(rawChannels) {
			entry = strings.ToLower(strings.TrimSpace(entry))
			if entry == "" {
				continue
			}
			if _, ok := notificationAPIChannels[entry]; !ok {
				return nil, fmt.Errorf("invalid channel: %s", entry)
			}
			channelSet[entry] = struct{}{}
		}
	}

	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		entries = append(entries, parseDelimitedList(toValue)...)
	}

	targets, err := parseNotificationAPITargets(entries, channelSet)
	if err != nil {
		return nil, err
	}

	cc := map[string]struct{}{}
	if ccValue := strings.TrimSpace(target.Query["cc"]); ccValue != "" {
		for _, entry := range parseDelimitedList(ccValue) {
			entry = strings.TrimSpace(entry)
			if isSimpleEmail(entry) {
				cc[entry] = struct{}{}
			}
		}
	}

	bcc := map[string]struct{}{}
	if bccValue := strings.TrimSpace(target.Query["bcc"]); bccValue != "" {
		for _, entry := range parseDelimitedList(bccValue) {
			entry = strings.TrimSpace(entry)
			if isSimpleEmail(entry) {
				bcc[entry] = struct{}{}
			}
		}
	}

	channels := make([]string, 0, len(channelSet))
	for channel := range channelSet {
		channels = append(channels, channel)
	}

	if fromAddr != "" {
		fromName = "Apprise"
	}

	tokens := map[string]string{}
	for key, value := range target.QueryPayload {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		tokens[key] = value
	}

	return &NotificationAPITarget{
		clientID:     clientID,
		clientSecret: clientSecret,
		messageType:  messageType,
		mode:         mode,
		region:       region,
		channels:     channels,
		targets:      targets,
		cc:           cc,
		bcc:          bcc,
		fromAddr:     fromAddr,
		fromName:     fromName,
		tokens:       tokens,
	}, nil
}

func (n *NotificationAPITarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := n.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}
	return SendRequest(spec)
}

func (n *NotificationAPITarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(n.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	baseURL, ok := notificationAPIRegionURLs[n.region]
	if !ok {
		return RequestSpec{}, fmt.Errorf("invalid region: %s", n.region)
	}

	payload := n.buildPayload(body, title, notifyType, n.targets[0])
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    fmt.Sprintf("%s/%s/sender", baseURL, n.clientID),
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/json",
			"Authorization": basicAuthHeader(
				n.clientID,
				n.clientSecret,
			),
		},
		Body: string(data),
	}, nil
}

func (n *NotificationAPITarget) buildPayload(body, title string, notifyType NotifyType, target notificationAPITargetEntry) map[string]any {
	payload := map[string]any{
		"type": n.messageType,
	}

	if n.mode == notificationAPIModeTemplate {
		parameters := map[string]any{}
		for key, value := range n.tokens {
			parameters[key] = value
		}

		parameters["appBody"] = body
		parameters["appTitle"] = title
		parameters["appType"] = string(notifyType)
		parameters["appId"] = "Apprise"
		parameters["appDescription"] = "Apprise Notifications"
		parameters["appColor"] = appriseColor(notifyType)
		parameters["appImageUrl"] = appriseImageURL(notifyType, "72x72")
		parameters["appUrl"] = "https://github.com/unraid/apprise-go"
		payload["parameters"] = parameters
	} else {
		textBody := body
		if len(n.channels) == 0 {
			n.channels = []string{"email"}
		}
		for _, channel := range n.channels {
			switch channel {
			case "sms":
				message := textBody
				if title != "" {
					message = title + "\n" + textBody
				}
				payload["sms"] = map[string]any{
					"message": message,
				}
			case "email":
				subject := title
				if subject == "" {
					subject = "Apprise"
				}
				payload["email"] = map[string]any{
					"subject": subject,
					"html":    body,
				}
				if n.fromAddr != "" {
					payload["email"].(map[string]any)["senderEmail"] = n.fromAddr
					payload["email"].(map[string]any)["senderName"] = n.fromName
				}
			case "inapp":
				fallback := title
				if fallback == "" {
					fallback = "Apprise"
				}
				payload["inapp"] = map[string]any{
					"title": fallback,
					"image": appriseImageURL(notifyType, "72x72"),
				}
			case "web_push":
				fallback := title
				if fallback == "" {
					fallback = "Apprise"
				}
				payload["web_push"] = map[string]any{
					"title":   fallback,
					"message": textBody,
					"icon":    appriseImageURL(notifyType, "72x72"),
				}
			case "mobile_push":
				fallback := title
				if fallback == "" {
					fallback = "Apprise"
				}
				payload["mobile_push"] = map[string]any{
					"title":   fallback,
					"message": textBody,
				}
			case "slack":
				message := textBody
				if title != "" {
					message = title + "\n" + textBody
				}
				payload["slack"] = map[string]any{
					"text": message,
				}
			}
		}
	}

	if n.fromAddr != "" {
		payload["options"] = map[string]any{
			"email": map[string]any{
				"fromAddress": n.fromAddr,
				"fromName":    n.fromName,
			},
		}
	} else if len(n.cc) > 0 || len(n.bcc) > 0 {
		payload["options"] = map[string]any{
			"email": map[string]any{},
		}
	}

	to := map[string]any{
		"id": target.id,
	}
	if target.email != "" {
		to["email"] = target.email
	}
	if target.number != "" {
		to["number"] = target.number
	}
	payload["to"] = to

	if len(n.cc) > 0 || len(n.bcc) > 0 {
		ccSet := n.cc
		bccSet := n.bcc
		if target.email != "" {
			ccSet = subtractSet(ccSet, n.bcc, target.email)
			bccSet = subtractSet(bccSet, nil, target.email)
		}
		if len(ccSet) > 0 {
			payload["options"].(map[string]any)["email"].(map[string]any)["ccAddresses"] = setToList(ccSet)
		}
		if len(bccSet) > 0 {
			payload["options"].(map[string]any)["email"].(map[string]any)["bccAddresses"] = setToList(bccSet)
		}
	}

	return payload
}

func parseNotificationAPITargets(entries []string, channels map[string]struct{}) ([]notificationAPITargetEntry, error) {
	targets := []notificationAPITargetEntry{}
	current := notificationAPITargetEntry{}

	for _, raw := range entries {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}

		if isSimpleEmail(trimmed) {
			if current.email == "" {
				current.email = trimmed
				if len(channels) == 0 {
					channels["email"] = struct{}{}
				}
				continue
			}
			if current.id != "" {
				targets = append(targets, current)
				current = notificationAPITargetEntry{email: trimmed}
				if len(channels) == 0 {
					channels["email"] = struct{}{}
				}
				continue
			}
			return nil, fmt.Errorf("too many emails for target")
		}

		if number, ok := normalizePhoneWithPlus(trimmed); ok {
			if current.number == "" {
				current.number = number
				if len(channels) == 0 {
					channels["sms"] = struct{}{}
				}
				continue
			}
			if current.id != "" {
				targets = append(targets, current)
				current = notificationAPITargetEntry{number: number}
				if len(channels) == 0 {
					channels["sms"] = struct{}{}
				}
				continue
			}
			return nil, fmt.Errorf("too many phone numbers for target")
		}

		if match := notificationAPIIDRe.FindStringSubmatch(trimmed); match != nil {
			id := match[1]
			if current.id == "" {
				current.id = id
				continue
			}
			targets = append(targets, current)
			current = notificationAPITargetEntry{id: id}
			continue
		}
	}

	if current.id != "" {
		targets = append(targets, current)
	} else if current.email != "" || current.number != "" {
		return nil, fmt.Errorf("missing id for target")
	}

	return targets, nil
}

func setToList(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	return out
}

func subtractSet(input map[string]struct{}, remove map[string]struct{}, item string) map[string]struct{} {
	out := map[string]struct{}{}
	for value := range input {
		if value == item {
			continue
		}
		if remove != nil {
			if _, ok := remove[value]; ok {
				continue
			}
		}
		out[value] = struct{}{}
	}
	return out
}

var notificationAPIIDRe = regexp.MustCompile(`^\s*(?:@|%40)?([\w_-]+)\s*$`)
