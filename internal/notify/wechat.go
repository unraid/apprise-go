package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const (
	wechatTokenURL  = "https://qyapi.weixin.qq.com/cgi-bin/gettoken"
	wechatNotifyURL = "https://qyapi.weixin.qq.com/cgi-bin/message/send"
)

var (
	// A user ID, with the @ prefix optional on input.
	wechatUserPattern = regexp.MustCompile(`^@?([A-Za-z0-9][A-Za-z0-9_@.\-]*)$`)
	// A department is #-prefixed and numeric, a tag +-prefixed and numeric.
	wechatDeptPattern  = regexp.MustCompile(`^#([0-9]+)$`)
	wechatTagPattern   = regexp.MustCompile(`^\+([0-9]+)$`)
	wechatAgentPattern = regexp.MustCompile(`^[0-9]+$`)
)

type WeChatTarget struct {
	corpID      string
	corpSecret  string
	agentID     string
	format      string
	users       []string
	departments []string
	tagIDs      []string
}

func NewWeChatTarget(target *ParsedURL) (*WeChatTarget, error) {
	corpID := strings.TrimSpace(target.User)
	if corpID == "" {
		return nil, fmt.Errorf("missing corp id")
	}

	corpSecret := strings.TrimSpace(target.Password)
	if corpSecret == "" {
		return nil, fmt.Errorf("missing app secret")
	}

	// The agent ID sits in the host field and is always numeric.
	agentID := strings.TrimSpace(target.Host)
	if !wechatAgentPattern.MatchString(agentID) {
		return nil, fmt.Errorf("invalid agent id: %s", agentID)
	}

	format := normalizeNotifyFormat(target.Query["format"])
	if format == "" {
		format = "text"
	}

	entries := splitPath(target.Path)
	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		entries = append(entries, parseDelimitedList(to)...)
	}

	wechat := &WeChatTarget{
		corpID:     corpID,
		corpSecret: corpSecret,
		agentID:    agentID,
		format:     format,
	}

	for _, entry := range sortedUniqueTargets(entries) {
		if match := wechatDeptPattern.FindStringSubmatch(entry); match != nil {
			wechat.departments = append(wechat.departments, match[1])
			continue
		}
		if match := wechatTagPattern.FindStringSubmatch(entry); match != nil {
			wechat.tagIDs = append(wechat.tagIDs, match[1])
			continue
		}
		if match := wechatUserPattern.FindStringSubmatch(entry); match != nil {
			// Both "all" and "@all" mean the broadcast recipient, which the
			// API spells "@all".
			user := match[1]
			if user == "all" {
				user = "@all"
			}
			wechat.users = append(wechat.users, user)
		}
	}

	if len(wechat.users) == 0 && len(wechat.departments) == 0 && len(wechat.tagIDs) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	return wechat, nil
}

// BuildRequest cannot describe this provider: the send URL carries an access
// token that only exists after a separate request.
func (w *WeChatTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	_, _, _ = body, title, notifyType

	return RequestSpec{}, fmt.Errorf("multi-step request")
}

func (w *WeChatTarget) Send(body, title string, notifyType NotifyType) error {
	_ = notifyType

	token, err := w.accessToken()
	if err != nil {
		return err
	}

	spec, err := w.messageSpec(body, title, token)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

// accessToken fetches a token for this send. Upstream keeps it in its
// persistent store for just under its two hour lifetime; with no store here
// every send fetches its own, which is what upstream also does the first time.
func (w *WeChatTarget) accessToken() (string, error) {
	query := url.Values{}
	query.Set("corpid", w.corpID)
	query.Set("corpsecret", w.corpSecret)

	spec := RequestSpec{
		Method: "GET",
		URL:    wechatTokenURL + "?" + query.Encode(),
		// Upstream issues this hop with no headers of its own, so it does not
		// carry the Apprise user agent the send does.
		Headers: map[string]string{
			"Accept": "*/*",
		},
	}

	var response struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
	}
	if err := doJSONRequest(spec, &response); err != nil {
		return "", err
	}

	// WeCom reports application errors in the body with a 200 status.
	if response.ErrCode != 0 {
		return "", fmt.Errorf("access token request failed: errcode=%d: %s", response.ErrCode, response.ErrMsg)
	}
	if response.AccessToken == "" {
		return "", fmt.Errorf("access token response contained no token")
	}

	return response.AccessToken, nil
}

func (w *WeChatTarget) messageSpec(body, title, token string) (RequestSpec, error) {
	// WeCom has no title field, so the title is folded into the message.
	content := mergeTitleBody(title, body)

	// Only markdown gets its own message type; HTML falls back to text.
	msgType := "text"
	if w.format == "markdown" {
		msgType = "markdown"
	}

	agentID, err := strconv.Atoi(w.agentID)
	if err != nil {
		return RequestSpec{}, fmt.Errorf("invalid agent id: %s", w.agentID)
	}

	payload := map[string]any{
		"agentid": agentID,
		"msgtype": msgType,
		msgType:   map[string]string{"content": content},
	}

	// Each recipient kind is only sent when it has entries.
	if len(w.users) > 0 {
		payload["touser"] = strings.Join(w.users, "|")
	}
	if len(w.departments) > 0 {
		payload["toparty"] = strings.Join(w.departments, "|")
	}
	if len(w.tagIDs) > 0 {
		payload["totag"] = strings.Join(w.tagIDs, "|")
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	query := url.Values{}
	query.Set("access_token", token)

	return RequestSpec{
		Method: "POST",
		URL:    wechatNotifyURL + "?" + query.Encode(),
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
		},
		Body: string(data),
	}, nil
}
func init() {
	RegisterSchemaEntryOrdered(161, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"cto": map[string]any{
					"default":  4.0,
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
					"default":  "text",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"optional": map[string]any{
					"default":  false,
					"map_to":   "optional",
					"name":     "Optional Service",
					"private":  false,
					"required": false,
					"type":     "bool",
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
				"redirect": map[string]any{
					"default":  true,
					"map_to":   "redirect",
					"name":     "Follow Redirects",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"retry": map[string]any{
					"default":  0,
					"map_to":   "retry",
					"max":      10,
					"min":      0,
					"name":     "Service Retry",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"rto": map[string]any{
					"default":  4.0,
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
				"wait": map[string]any{
					"default":  0.0,
					"map_to":   "wait",
					"max":      20.0,
					"min":      0.0,
					"name":     "Inter-Retry Wait",
					"private":  false,
					"required": false,
					"type":     "float",
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{user}:{password}@{host}", "{schema}://{user}:{password}@{host}/{targets}"},
			"tokens": map[string]any{
				"host": map[string]any{
					"map_to":   "agentid",
					"name":     "Agent ID",
					"private":  false,
					"regex":    []string{"^[0-9]+$", ""},
					"required": true,
					"type":     "string",
				},
				"password": map[string]any{
					"map_to":   "corpsecret",
					"name":     "App Secret",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "wechat",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"wechat"},
				},
				"target_department": map[string]any{
					"map_to":   "targets",
					"name":     "Target Department",
					"prefix":   "#",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_tag": map[string]any{
					"map_to":   "targets",
					"name":     "Target Tag",
					"prefix":   "+",
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
					"group":    []string{"target_department", "target_tag", "target_user"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"user": map[string]any{
					"map_to":   "corpid",
					"name":     "Corp ID",
					"private":  false,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled": true,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"protocols":        nil,
		"secure_protocols": []string{"wechat"},
		"service_name":     "WeChat (WeCom)",
		"service_url":      "https://work.weixin.qq.com/",
		"setup_url":        "https://appriseit.com/services/wechat/",
	})
}
