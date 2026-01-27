package notify

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	matrixWebhookPath        = "/api/v1/matrix/hook"
	matrixV2APIPath          = "/_matrix/client/r0"
	matrixV3APIPath          = "/_matrix/client/v3"
	matrixV2MediaPath        = "/_matrix/media/r0"
	matrixV3MediaPath        = "/_matrix/media/v3"
	matrixT2BotWebhookURL    = "https://webhooks.t2bot.io/api/v1/matrix/hook/"
	matrixDefaultUserAgent   = "Apprise"
	matrixFixedTransactionID = "00000000-0000-4000-8000-000000000000"
)

const (
	matrixModeOff    = "off"
	matrixModeMatrix = "matrix"
	matrixModeSlack  = "slack"
	matrixModeT2Bot  = "t2bot"

	matrixVersionV2 = "2"
	matrixVersionV3 = "3"

	matrixMsgTypeText   = "text"
	matrixMsgTypeNotice = "notice"
)

var matrixRoomAliasRegex = regexp.MustCompile(`(?i)^\s*(?:#|%23)?([A-Za-z0-9._=-]+)(?:(?:\:|%3A)([A-Za-z0-9.-]+))?\s*$`)
var matrixRoomIDRegex = regexp.MustCompile(`(?i)^\s*(?:!|&#33;|%21)([A-Za-z0-9._=-]+)(?:(?:\:|%3A)([A-Za-z0-9.-]+))?\s*$`)
var matrixT2BotTokenRegex = regexp.MustCompile(`^[A-Za-z0-9]{64}$`)

type MatrixTarget struct {
	host                string
	port                int
	hasPort             bool
	secure              bool
	user                string
	password            string
	mode                string
	version             string
	msgType             string
	notifyFormat        string
	includeImage        bool
	discovery           bool
	rooms               []string
	accessToken         string
	homeServer          string
	userID              string
	transactionID       int
	transactionIDString string
	baseURLCached       string
	discoveryDone       bool
}

func NewMatrixTarget(target *ParsedURL) (*MatrixTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	rooms := normalizeMatrixRooms(splitPath(target.Path))
	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		rooms = append(rooms, normalizeMatrixRooms(parseDelimitedList(toValue))...)
	}

	user := strings.TrimSpace(target.User)
	password := strings.TrimSpace(target.Password)
	tokenQuery := strings.TrimSpace(target.Query["token"])
	if tokenQuery != "" {
		password = tokenQuery
	}

	mode := strings.ToLower(strings.TrimSpace(target.Query["mode"]))
	if mode == "" && password == "" && len(rooms) == 0 {
		mode = matrixModeT2Bot
	}
	if mode == matrixModeT2Bot {
		password = strings.TrimSpace(host)
		if tokenQuery != "" {
			password = tokenQuery
		}
	}

	if password == "" && user != "" {
		password = user
		user = ""
	}

	if mode == "" {
		mode = matrixModeOff
	}

	switch mode {
	case matrixModeOff, matrixModeMatrix, matrixModeSlack, matrixModeT2Bot:
	default:
		return nil, fmt.Errorf("unsupported matrix mode")
	}

	version := strings.TrimSpace(target.Query["version"])
	if version == "" {
		version = strings.TrimSpace(target.Query["v"])
	}
	version = strings.TrimSpace(strings.ToLower(strings.TrimPrefix(version, "v")))
	if version == "" {
		version = matrixVersionV3
	}
	switch version {
	case matrixVersionV2, matrixVersionV3:
	default:
		return nil, fmt.Errorf("unsupported matrix version")
	}

	msgType := strings.TrimSpace(target.Query["msgtype"])
	msgType = strings.ToLower(msgType)
	if msgType == "" {
		msgType = matrixMsgTypeText
	}
	switch msgType {
	case matrixMsgTypeText, matrixMsgTypeNotice:
	default:
		return nil, fmt.Errorf("unsupported matrix message type")
	}

	notifyFormat := normalizeNotifyFormat(target.Query["format"])
	if notifyFormat == "" {
		notifyFormat = "text"
	}
	switch notifyFormat {
	case "text", "html", "markdown":
	default:
		return nil, fmt.Errorf("unsupported matrix format")
	}

	includeImage := parseMatrixBool(target.Query["image"], false)
	discovery := parseMatrixBool(target.Query["discovery"], true)
	if mode != matrixModeOff {
		discovery = false
	}

	matrixTarget := &MatrixTarget{
		host:          host,
		port:          target.Port,
		hasPort:       target.HasPort,
		secure:        strings.EqualFold(target.Scheme, "matrixs"),
		user:          user,
		password:      password,
		mode:          mode,
		version:       version,
		msgType:       msgType,
		notifyFormat:  notifyFormat,
		includeImage:  includeImage,
		discovery:     discovery,
		rooms:         rooms,
		transactionID: 0,
	}

	if mode == matrixModeT2Bot {
		if strings.TrimSpace(matrixTarget.password) == "" {
			return nil, fmt.Errorf("missing t2bot token")
		}
		if !matrixT2BotTokenRegex.MatchString(matrixTarget.password) {
			return nil, fmt.Errorf("invalid t2bot token")
		}
		matrixTarget.accessToken = matrixTarget.password
	}

	return matrixTarget, nil
}

func (m *MatrixTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if m.mode == matrixModeOff {
		return RequestSpec{}, fmt.Errorf("matrix server mode uses multiple requests")
	}
	return m.buildWebhookRequest(body, title, notifyType)
}

func (m *MatrixTarget) Send(body, title string, notifyType NotifyType) error {
	if m.mode == matrixModeOff {
		return m.sendServer(body, title, notifyType)
	}
	spec, err := m.buildWebhookRequest(body, title, notifyType)
	if err != nil {
		return err
	}
	return SendRequest(spec)
}

func (m *MatrixTarget) sendServer(body, title string, notifyType NotifyType) error {
	if m.accessToken == "" && m.password != "" && m.user == "" {
		m.accessToken = m.password
		m.transactionIDString = matrixFixedTransactionID
	}

	if m.accessToken == "" {
		if ok := m.login(); !ok {
			if ok := m.register(); !ok {
				return fmt.Errorf("matrix authentication failed")
			}
		}
	}

	rooms := m.rooms
	if len(rooms) == 0 {
		rooms = m.joinedRooms()
		if len(rooms) == 0 {
			return nil
		}
	}

	for _, room := range rooms {
		roomID := m.roomJoin(room)
		if roomID == "" {
			continue
		}

		if m.includeImage && m.version == matrixVersionV2 {
			if ok := m.sendImage(roomID, title, notifyType); !ok {
				return fmt.Errorf("matrix image send failed")
			}
		}

		if ok := m.sendMessage(roomID, body, title, notifyType); !ok {
			return fmt.Errorf("matrix send failed")
		}
	}

	return nil
}

func (m *MatrixTarget) buildWebhookRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload := map[string]any{}
	urlStr := ""
	switch m.mode {
	case matrixModeT2Bot:
		payload = m.t2botPayload(body, title, notifyType)
		urlStr = matrixT2BotWebhookURL + m.accessToken
	case matrixModeSlack:
		payload = m.slackPayload(body, title, notifyType)
		urlStr = m.webhookURL()
	default:
		payload = m.matrixPayload(body, title)
		urlStr = m.webhookURL()
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	headers := map[string]string{
		"User-Agent":   matrixDefaultUserAgent,
		"Content-Type": "application/json",
	}

	return RequestSpec{
		Method:  "POST",
		URL:     urlStr,
		Headers: headers,
		Body:    string(data),
	}, nil
}

func (m *MatrixTarget) webhookURL() string {
	scheme := "http"
	if m.secure {
		scheme = "https"
	}
	token := m.password
	if token == "" {
		token = m.user
	}
	port := ""
	if m.hasPort {
		port = fmt.Sprintf(":%d", m.port)
	}
	return fmt.Sprintf("%s://%s%s%s/%s", scheme, m.host, port, matrixWebhookPath, token)
}

func (m *MatrixTarget) matrixPayload(body, title string) map[string]any {
	payload := map[string]any{
		"displayName": matrixDefaultUserAgent,
		"format":      "plain",
		"text":        "",
	}
	if m.user != "" {
		payload["displayName"] = m.user
	}

	switch m.notifyFormat {
	case "html":
		payload["format"] = "html"
		payload["text"] = matrixHTMLTitleBody(title, body)
	case "markdown":
		payload["format"] = "html"
		payload["text"] = matrixMarkdownTitleBody(title, body)
	default:
		payload["format"] = "plain"
		payload["text"] = mergeTitleBody(title, body)
	}

	return payload
}

func (m *MatrixTarget) t2botPayload(body, title string, notifyType NotifyType) map[string]any {
	payload := m.matrixPayload(body, title)
	if m.includeImage {
		payload["avatarUrl"] = appriseImageURL(notifyType, "32x32")
	}
	return payload
}

func (m *MatrixTarget) slackPayload(body, title string, notifyType NotifyType) map[string]any {
	username := matrixDefaultUserAgent
	if m.user != "" {
		username = m.user
	}

	payload := map[string]any{
		"username": username,
		"mrkdwn":   m.notifyFormat == "markdown",
		"attachments": []any{
			map[string]any{
				"title":  matrixSlackEscape(title),
				"text":   matrixSlackEscape(body),
				"color":  appriseColor(notifyType),
				"ts":     json.Number(fmt.Sprintf("%.1f", float64(fixedTime().Unix()))),
				"footer": matrixDefaultUserAgent,
			},
		},
	}

	return payload
}

func (m *MatrixTarget) login() bool {
	if m.accessToken != "" {
		return true
	}
	if m.user == "" || m.password == "" {
		return false
	}

	var payload map[string]any
	if m.version == matrixVersionV3 {
		payload = map[string]any{
			"type": "m.login.password",
			"identifier": map[string]any{
				"type": "m.id.user",
				"user": m.user,
			},
			"password": m.password,
		}
	} else {
		payload = map[string]any{
			"type":     "m.login.password",
			"user":     m.user,
			"password": m.password,
		}
	}

	ok, response, _ := m.fetch("/login", payload, nil, http.MethodPost, "")
	if !ok {
		return false
	}
	accessToken, _ := response["access_token"].(string)
	if accessToken == "" {
		return false
	}
	m.accessToken = accessToken
	m.homeServer, _ = response["home_server"].(string)
	m.userID, _ = response["user_id"].(string)
	return true
}

func (m *MatrixTarget) register() bool {
	payload := map[string]any{
		"kind": "user",
		"auth": map[string]any{
			"type": "m.login.dummy",
		},
	}
	if m.user != "" {
		payload["username"] = m.user
	}
	if m.password != "" {
		payload["password"] = m.password
	}

	params := url.Values{}
	params.Set("kind", "user")
	ok, response, _ := m.fetch("/register", payload, params, http.MethodPost, "")
	if !ok {
		return false
	}
	accessToken, _ := response["access_token"].(string)
	if accessToken == "" {
		return false
	}
	m.accessToken = accessToken
	m.homeServer, _ = response["home_server"].(string)
	m.userID, _ = response["user_id"].(string)
	return true
}

func (m *MatrixTarget) joinedRooms() []string {
	ok, response, _ := m.fetch("/joined_rooms", nil, nil, http.MethodGet, "")
	if !ok {
		return nil
	}
	raw, ok := response["joined_rooms"]
	if !ok {
		return nil
	}
	rooms := []string{}
	switch typed := raw.(type) {
	case []string:
		rooms = append(rooms, typed...)
	case []any:
		for _, entry := range typed {
			if value, ok := entry.(string); ok {
				rooms = append(rooms, value)
			}
		}
	}
	return normalizeMatrixRooms(rooms)
}

func (m *MatrixTarget) roomJoin(room string) string {
	room = strings.TrimSpace(room)
	if room == "" {
		return ""
	}

	if id, ok := m.normalizeRoomID(room); ok {
		ok, _, _ := m.fetch("/join/"+url.PathEscape(id), map[string]any{}, nil, http.MethodPost, "")
		if ok {
			return id
		}
		return ""
	}

	alias, ok := m.normalizeRoomAlias(room)
	if !ok {
		return ""
	}
	ok, response, _ := m.fetch("/join/"+url.PathEscape(alias), map[string]any{}, nil, http.MethodPost, "")
	if !ok {
		return ""
	}
	if roomID, ok := response["room_id"].(string); ok && roomID != "" {
		return roomID
	}
	return ""
}

func (m *MatrixTarget) sendImage(roomID, title string, notifyType NotifyType) bool {
	payload := map[string]any{
		"msgtype": "m.image",
		"url":     appriseImageURL(notifyType, "32x32"),
		"body":    title,
	}
	if payload["body"] == "" {
		payload["body"] = string(notifyType)
	}
	path := fmt.Sprintf("/rooms/%s/send/m.room.message", url.PathEscape(roomID))
	ok, _, _ := m.fetch(path, payload, nil, http.MethodPost, "")
	return ok
}

func (m *MatrixTarget) sendMessage(roomID, body, title string, notifyType NotifyType) bool {
	payload := map[string]any{
		"msgtype": fmt.Sprintf("m.%s", m.msgType),
		"body":    matrixBodyWithTitle(title, body),
	}

	if m.notifyFormat == "html" {
		payload["format"] = "org.matrix.custom.html"
		payload["formatted_body"] = matrixHTMLBody(title, body)
	} else if m.notifyFormat == "markdown" {
		payload["format"] = "org.matrix.custom.html"
		payload["formatted_body"] = matrixMarkdownBody(title, body)
	}

	path := m.messagePath(roomID)
	ok, _, _ := m.fetch(path, payload, nil, m.messageMethod(), "")
	if ok && m.version == matrixVersionV3 && m.accessToken != "" && m.accessToken != m.password && m.transactionIDString == "" {
		m.transactionID++
	}
	_ = notifyType
	return ok
}

func (m *MatrixTarget) messagePath(roomID string) string {
	escapedRoom := url.PathEscape(roomID)
	if m.version == matrixVersionV3 {
		transaction := m.transactionIDString
		if transaction == "" {
			transaction = fmt.Sprintf("%d", m.transactionID)
		}
		return fmt.Sprintf("/rooms/%s/send/m.room.message/%s", escapedRoom, url.PathEscape(transaction))
	}
	return fmt.Sprintf("/rooms/%s/send/m.room.message", escapedRoom)
}

func (m *MatrixTarget) messageMethod() string {
	if m.version == matrixVersionV3 {
		return http.MethodPut
	}
	return http.MethodPost
}

func (m *MatrixTarget) normalizeRoomID(room string) (string, bool) {
	matches := matrixRoomIDRegex.FindStringSubmatch(room)
	if len(matches) < 2 {
		return "", false
	}
	roomID := matches[1]
	homeServer := ""
	if len(matches) > 2 {
		homeServer = matches[2]
	}
	if homeServer == "" {
		homeServer = m.homeServer
	}
	if homeServer == "" {
		homeServer = "None"
	}
	return fmt.Sprintf("!%s:%s", roomID, homeServer), true
}

func (m *MatrixTarget) normalizeRoomAlias(room string) (string, bool) {
	matches := matrixRoomAliasRegex.FindStringSubmatch(room)
	if len(matches) < 2 {
		return "", false
	}
	alias := matches[1]
	homeServer := ""
	if len(matches) > 2 {
		homeServer = matches[2]
	}
	if homeServer == "" {
		homeServer = m.homeServer
	}
	if homeServer == "" {
		homeServer = "None"
	}
	return fmt.Sprintf("#%s:%s", alias, homeServer), true
}

func (m *MatrixTarget) fetch(path string, payload map[string]any, params url.Values, method, urlOverride string) (bool, map[string]any, int) {
	urlStr, err := m.buildURL(path, params, urlOverride)
	if err != nil {
		return false, map[string]any{}, http.StatusInternalServerError
	}

	headers := map[string]string{
		"User-Agent":   matrixDefaultUserAgent,
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}
	if m.accessToken != "" {
		headers["Authorization"] = "Bearer " + m.accessToken
	}

	body := ""
	if payload != nil && method != http.MethodGet {
		data, err := json.Marshal(payload)
		if err != nil {
			return false, map[string]any{}, http.StatusInternalServerError
		}
		body = string(data)
	}

	spec := RequestSpec{
		Method:  method,
		URL:     urlStr,
		Headers: headers,
		Body:    body,
	}

	status, response, err := matrixSend(spec)
	if err != nil {
		return false, map[string]any{}, http.StatusInternalServerError
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return false, response, status
	}
	return true, response, status
}

func (m *MatrixTarget) buildURL(path string, params url.Values, urlOverride string) (string, error) {
	if urlOverride != "" {
		return appendQuery(urlOverride, params), nil
	}

	baseURL, err := m.baseURL()
	if err != nil {
		return "", err
	}

	if path == "/upload" {
		if m.version == matrixVersionV3 {
			return appendQuery(baseURL+matrixV3MediaPath+path, params), nil
		}
		return appendQuery(baseURL+matrixV2MediaPath+path, params), nil
	}

	if m.version == matrixVersionV3 {
		return appendQuery(baseURL+matrixV3APIPath+path, params), nil
	}
	return appendQuery(baseURL+matrixV2APIPath+path, params), nil
}

func (m *MatrixTarget) baseURL() (string, error) {
	if m.discovery && m.secure {
		if m.discoveryDone {
			if m.baseURLCached != "" {
				return m.baseURLCached, nil
			}
		} else {
			baseURL, err := m.serverDiscovery()
			m.discoveryDone = true
			m.baseURLCached = baseURL
			if err != nil {
				return "", err
			}
			if baseURL != "" {
				return baseURL, nil
			}
		}
	}
	scheme := "http"
	if m.secure {
		scheme = "https"
	}
	port := ""
	if m.hasPort {
		port = fmt.Sprintf(":%d", m.port)
	}
	return fmt.Sprintf("%s://%s%s", scheme, m.host, port), nil
}

func (m *MatrixTarget) serverDiscovery() (string, error) {
	verifyURL := fmt.Sprintf("https://%s%s/.well-known/matrix/client", m.host, m.portSuffix())
	ok, response, status := m.fetch("", nil, nil, http.MethodGet, verifyURL)
	if status == http.StatusNotFound {
		return "", nil
	}
	if !ok {
		return "", fmt.Errorf("matrix discovery failed")
	}
	if len(response) == 0 {
		return "", nil
	}

	baseURL := ""
	if entry, ok := response["m.homeserver"].(map[string]any); ok {
		if value, ok := entry["base_url"].(string); ok {
			baseURL = strings.TrimRight(value, "/")
		}
	}
	if baseURL == "" {
		return "", fmt.Errorf("matrix discovery missing base_url")
	}

	verifyURL = baseURL + "/_matrix/client/versions"
	ok, _, _ = m.fetch("", nil, nil, http.MethodGet, verifyURL)
	if !ok {
		return "", fmt.Errorf("matrix discovery verification failed")
	}

	return baseURL, nil
}

func (m *MatrixTarget) portSuffix() string {
	if m.hasPort {
		return fmt.Sprintf(":%d", m.port)
	}
	return ""
}

func matrixSend(spec RequestSpec) (int, map[string]any, error) {
	req, err := spec.HTTPRequest()
	if err != nil {
		return http.StatusInternalServerError, map[string]any{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return http.StatusInternalServerError, map[string]any{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, map[string]any{}, err
	}

	response := map[string]any{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &response); err != nil {
			return resp.StatusCode, map[string]any{}, nil
		}
	}

	return resp.StatusCode, response, nil
}

func appendQuery(raw string, params url.Values) string {
	if len(params) == 0 {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	values := parsed.Query()
	for key, entries := range params {
		for _, entry := range entries {
			values.Add(key, entry)
		}
	}
	parsed.RawQuery = values.Encode()
	return parsed.String()
}

func matrixHTMLTitleBody(title, body string) string {
	if title == "" {
		return body
	}
	return "<h1>" + matrixEscapeHTML(title, true) + "</h1>" + body
}

func matrixMarkdownTitleBody(title, body string) string {
	content := matrixMarkdown(body)
	if title == "" {
		return content
	}
	return "<h1>" + matrixEscapeHTML(title, true) + "</h1>" + content
}

func matrixHTMLBody(title, body string) string {
	if title == "" {
		return body
	}
	return "<h1>" + title + "</h1>" + body
}

func matrixMarkdownBody(title, body string) string {
	content := matrixMarkdown(body)
	if title == "" {
		return content
	}
	return "<h1>" + matrixEscapeHTML(title, false) + "</h1>" + content
}

func matrixBodyWithTitle(title, body string) string {
	if title == "" {
		return body
	}
	return "# " + title + "\r\n" + body
}

func matrixMarkdown(body string) string {
	return body
}

func matrixSlackEscape(value string) string {
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "\r\n", "\\n")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\r", "\\n")
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(value)
}

func matrixEscapeHTML(value string, whitespace bool) string {
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	escaped := replacer.Replace(value)
	if whitespace {
		escaped = strings.ReplaceAll(escaped, "\t", "&emsp;")
		escaped = strings.ReplaceAll(escaped, " ", "&nbsp;")
	}
	return escaped
}

func normalizeMatrixRooms(entries []string) []string {
	if len(entries) == 0 {
		return nil
	}
	rooms := make([]string, 0, len(entries))
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		rooms = append(rooms, trimmed)
	}
	return rooms
}

func parseMatrixBool(raw string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func init() {
	RegisterSchemaEntryOrdered(2, SchemaEntry{
		"attachment_support": true,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"cto": map[string]any{
					"default":  4,
					"map_to":   "cto",
					"name":     "Socket Connect Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"discovery": map[string]any{
					"default":  true,
					"map_to":   "discovery",
					"name":     "Server Discovery",
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
					"default":  "text",
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
				"mode": map[string]any{
					"default":  "off",
					"map_to":   "mode",
					"name":     "Webhook Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"off", "matrix", "slack", "t2bot"},
				},
				"msgtype": map[string]any{
					"default":  "text",
					"map_to":   "msgtype",
					"name":     "Message Type",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"text", "notice"},
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
				"token": map[string]any{
					"alias_of": "token",
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
				"version": map[string]any{
					"default":  "3",
					"map_to":   "version",
					"name":     "Matrix API Verion",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"2", "3"},
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{token}", "{schema}://{user}@{token}", "{schema}://{user}:{password}@{host}/{targets}", "{schema}://{user}:{password}@{host}:{port}/{targets}", "{schema}://{token}@{host}/{targets}", "{schema}://{token}@{host}:{port}/{targets}", "{schema}://{user}:{token}@{host}/{targets}", "{schema}://{user}:{token}@{host}:{port}/{targets}"},
			"tokens": map[string]any{
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": false,
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
					"values":   []string{"matrix", "matrixs"},
				},
				"target_room_alias": map[string]any{
					"map_to":   "targets",
					"name":     "Target Room Alias",
					"prefix":   "#",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_room_id": map[string]any{
					"map_to":   "targets",
					"name":     "Target Room ID",
					"prefix":   "!",
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
					"group":    []string{"target_room_alias", "target_room_id", "target_user"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "password",
					"name":     "Access Token",
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
			},
		},
		"enabled":   true,
		"protocols": []string{"matrix"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"matrixs"},
		"service_name":     "Matrix",
		"service_url":      "https://matrix.org/",
		"setup_url":        "https://appriseit.com/services/matrix/",
	})
}
