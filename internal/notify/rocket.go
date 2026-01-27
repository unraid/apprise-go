package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const (
	rocketModeWebhook = "webhook"
	rocketModeBasic   = "basic"
	rocketModeToken   = "token"
)

type RocketChatTarget struct {
	mode      string
	webhook   string
	host      string
	port      int
	secure    bool
	avatar    bool
	user      string
	password  string
	authToken string
	authUser  string
	channels  []string
	rooms     []string
	users     []string
}

func NewRocketChatTarget(target *ParsedURL) (*RocketChatTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	mode := strings.ToLower(strings.TrimSpace(target.Query["mode"]))
	if mode != "" && mode != rocketModeWebhook && mode != rocketModeBasic && mode != rocketModeToken {
		return nil, fmt.Errorf("unsupported mode: %s", mode)
	}

	user := strings.TrimSpace(target.User)
	password := strings.TrimSpace(target.Password)
	webhook := strings.TrimSpace(target.Query["webhook"])
	if webhook == "" && password != "" && strings.Contains(password, "/") {
		webhook = password
	}
	if webhook == "" && user != "" && password == "" && strings.Contains(user, "/") {
		webhook = user
	}

	if mode == "" {
		if webhook != "" {
			mode = rocketModeWebhook
		} else if len(password) > 32 {
			mode = rocketModeToken
		} else {
			mode = rocketModeBasic
		}
	}

	if mode == rocketModeWebhook {
		if webhook == "" {
			return nil, fmt.Errorf("missing webhook")
		}
	}

	if (mode == rocketModeBasic || mode == rocketModeToken) && (user == "" || password == "") {
		return nil, fmt.Errorf("missing credentials")
	}

	avatarProvided := false
	avatar := false
	if rawAvatar, ok := target.Query["avatar"]; ok {
		avatarProvided = true
		avatar = parseBool(rawAvatar, true)
	}
	if !avatarProvided {
		if mode == rocketModeBasic {
			avatar = false
		} else {
			avatar = true
		}
	}

	rawTargets := splitPath(target.Path)
	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		rawTargets = append(rawTargets, parseDelimitedList(toValue)...)
	}
	channels, rooms, users := splitRocketTargets(rawTargets)

	targetEntry := &RocketChatTarget{
		mode:     mode,
		webhook:  webhook,
		host:     host,
		port:     target.Port,
		secure:   strings.EqualFold(target.Scheme, "rockets"),
		avatar:   avatar,
		user:     user,
		password: password,
		channels: channels,
		rooms:    rooms,
		users:    users,
	}
	if mode == rocketModeToken {
		targetEntry.authToken = password
		targetEntry.authUser = user
	}

	return targetEntry, nil
}

func (r *RocketChatTarget) Send(body, title string, notifyType NotifyType) error {
	switch r.mode {
	case rocketModeWebhook:
		return r.sendWebhook(body, title, notifyType)
	case rocketModeToken, rocketModeBasic:
		return r.sendAuthenticated(body, title, notifyType)
	default:
		return fmt.Errorf("unsupported mode")
	}
}

func (r *RocketChatTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if r.mode != rocketModeWebhook {
		return RequestSpec{}, fmt.Errorf("rocket chat authenticated modes use multiple requests")
	}
	return r.buildWebhookRequest(body, title, notifyType, "")
}

func (r *RocketChatTarget) sendWebhook(body, title string, notifyType NotifyType) error {
	targets := r.webhookTargets()
	if len(targets) == 0 {
		spec, err := r.buildWebhookRequest(body, title, notifyType, "")
		if err != nil {
			return err
		}
		return SendRequest(spec)
	}

	for _, target := range targets {
		spec, err := r.buildWebhookRequest(body, title, notifyType, target)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}
	return nil
}

func (r *RocketChatTarget) sendAuthenticated(body, title string, notifyType NotifyType) error {
	if r.mode == rocketModeBasic {
		if err := r.login(); err != nil {
			return err
		}
		defer func() { _ = r.logout() }()
	}

	payload := r.buildPayload(body, title, notifyType)
	headers := r.authHeaders()
	urlStr := r.apiURL("/api/v1/chat.postMessage")

	for _, user := range r.users {
		payload["channel"] = "@" + user
		delete(payload, "roomId")
		spec, err := rocketRequest(urlStr, payload, headers)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	for _, channel := range r.channels {
		payload["channel"] = "#" + channel
		delete(payload, "roomId")
		spec, err := rocketRequest(urlStr, payload, headers)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	for _, room := range r.rooms {
		payload["roomId"] = room
		delete(payload, "channel")
		spec, err := rocketRequest(urlStr, payload, headers)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func (r *RocketChatTarget) buildWebhookRequest(body, title string, notifyType NotifyType, target string) (RequestSpec, error) {
	payload := r.buildPayload(body, title, notifyType)
	if strings.TrimSpace(target) != "" {
		payload["channel"] = target
	}

	urlStr := r.apiURL(fmt.Sprintf("/hooks/%s", r.webhook))
	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/json",
	}
	return rocketRequest(urlStr, payload, headers)
}

func (r *RocketChatTarget) buildPayload(body, title string, notifyType NotifyType) map[string]any {
	payload := map[string]any{
		"text": mergeTitleBody(title, body),
	}
	if r.avatar {
		payload["avatar"] = appriseImageURL(notifyType, "128x128")
	}
	return payload
}

func (r *RocketChatTarget) login() error {
	values := url.Values{}
	values.Set("username", r.user)
	values.Set("password", r.password)

	spec := RequestSpec{
		Method: "POST",
		URL:    r.apiURL("/api/v1/login"),
		Headers: map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		},
		Body: values.Encode(),
	}

	status, response, err := rocketSend(spec)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("rocket chat login failed")
	}

	if response["status"] != "success" {
		return fmt.Errorf("rocket chat login failed")
	}
	data, _ := response["data"].(map[string]any)
	authToken, _ := data["authToken"].(string)
	userID, _ := data["userId"].(string)
	if authToken == "" || userID == "" {
		return fmt.Errorf("rocket chat login failed")
	}
	r.authToken = authToken
	r.authUser = userID
	return nil
}

func (r *RocketChatTarget) logout() error {
	if r.authToken == "" || r.authUser == "" {
		return nil
	}
	spec := RequestSpec{
		Method: "POST",
		URL:    r.apiURL("/api/v1/logout"),
		Headers: map[string]string{
			"X-Auth-Token": r.authToken,
			"X-User-Id":    r.authUser,
		},
		Body: "",
	}
	if err := SendRequest(spec); err != nil {
		return err
	}
	return nil
}

func (r *RocketChatTarget) authHeaders() map[string]string {
	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/json",
	}
	if r.authToken != "" {
		headers["X-Auth-Token"] = r.authToken
	}
	if r.authUser != "" {
		headers["X-User-Id"] = r.authUser
	}
	return headers
}

func (r *RocketChatTarget) webhookTargets() []string {
	targets := make([]string, 0, len(r.channels)+len(r.rooms)+len(r.users))
	for _, user := range r.users {
		targets = append(targets, "@"+user)
	}
	for _, channel := range r.channels {
		targets = append(targets, "#"+channel)
	}
	for _, room := range r.rooms {
		targets = append(targets, room)
	}
	return targets
}

func (r *RocketChatTarget) apiURL(path string) string {
	scheme := "http"
	if r.secure {
		scheme = "https"
	}
	host := r.host
	if r.port != 0 {
		host = fmt.Sprintf("%s:%d", host, r.port)
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, path)
}

func splitRocketTargets(entries []string) ([]string, []string, []string) {
	if len(entries) == 0 {
		return nil, nil, nil
	}
	channels := []string{}
	rooms := []string{}
	users := []string{}
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			name := strings.TrimPrefix(trimmed, "#")
			if name != "" {
				channels = append(channels, name)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "@") {
			name := strings.TrimPrefix(trimmed, "@")
			if name != "" {
				users = append(users, name)
			}
			continue
		}
		rooms = append(rooms, trimmed)
	}
	return channels, rooms, users
}

func rocketRequest(urlStr string, payload map[string]any, headers map[string]string) (RequestSpec, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}
	return RequestSpec{
		Method:  "POST",
		URL:     urlStr,
		Headers: headers,
		Body:    string(data),
	}, nil
}

func rocketSend(spec RequestSpec) (int, map[string]any, error) {
	status, response, err := matrixSend(spec)
	if err != nil {
		return status, map[string]any{}, err
	}
	return status, response, nil
}

func init() {
	RegisterSchemaEntryOrdered(120, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"avatar": map[string]any{
					"default":  false,
					"map_to":   "avatar",
					"name":     "Use Avatar",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"cto": map[string]any{
					"default":  4,
					"map_to":   "cto",
					"name":     "Socket Connect Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
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
					"default":  "markdown",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"mode": map[string]any{
					"map_to":   "mode",
					"name":     "Webhook Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"webhook", "token", "basic"},
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
				"rto": map[string]any{
					"default":  4,
					"map_to":   "rto",
					"name":     "Socket Read Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"store": map[string]any{
					"default":  true,
					"map_to":   "store",
					"name":     "Persistent Storage",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"to": map[string]any{
					"alias_of": "targets",
					"delim":    []string{",", " "},
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
				"webhook": map[string]any{
					"alias_of": "webhook",
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{user}:{password}@{host}:{port}/{targets}", "{schema}://{user}:{password}@{host}/{targets}", "{schema}://{user}:{token}@{host}:{port}/{targets}", "{schema}://{user}:{token}@{host}/{targets}", "{schema}://{webhook}@{host}", "{schema}://{webhook}@{host}:{port}", "{schema}://{webhook}@{host}/{targets}", "{schema}://{webhook}@{host}:{port}/{targets}"},
			"tokens": map[string]any{
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"password": map[string]any{
					"map_to":   "password",
					"name":     "Password",
					"private":  true,
					"required": false,
					"type":     "string",
				},
				"port": map[string]any{
					"map_to":   "port",
					"max":      65535,
					"min":      1,
					"name":     "Port",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"rocket", "rockets"},
				},
				"target_channel": map[string]any{
					"map_to":   "targets",
					"name":     "Target Channel",
					"prefix":   "#",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_room": map[string]any{
					"map_to":   "targets",
					"name":     "Target Room ID",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_user": map[string]any{
					"map_to":   "targets",
					"name":     "Target User",
					"prefix":   "@",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_channel", "target_room", "target_user"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "password",
					"name":     "API Token",
					"private":  true,
					"required": false,
					"type":     "string",
				},
				"user": map[string]any{
					"map_to":   "user",
					"name":     "Username",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"webhook": map[string]any{
					"map_to":   "webhook",
					"name":     "Webhook",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"rocket"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"rockets"},
		"service_name":     "Rocket.Chat",
		"service_url":      "https://rocket.chat/",
		"setup_url":        "https://appriseit.com/services/rocketchat/",
	})
}
