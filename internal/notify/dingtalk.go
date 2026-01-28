package notify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type DingTalkTarget struct {
	token   string
	secret  string
	targets []string
}

func NewDingTalkTarget(target *ParsedURL) (*DingTalkTarget, error) {
	token := strings.TrimSpace(target.Host)
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}
	secret := strings.TrimSpace(target.User)
	if rawSecret := strings.TrimSpace(target.Query["secret"]); rawSecret != "" {
		secret = rawSecret
	}
	if secret != "" && !dingtalkSecretRegex.MatchString(secret) {
		return nil, fmt.Errorf("invalid secret")
	}

	targets := splitPath(target.Path)
	if toValue, ok := target.Query["to"]; ok && strings.TrimSpace(toValue) != "" {
		targets = append(targets, parseDelimitedList(toValue)...)
	}

	return &DingTalkTarget{
		token:   token,
		secret:  secret,
		targets: targets,
	}, nil
}

func (d *DingTalkTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := d.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (d *DingTalkTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	message := mergeTitleBody(title, body)

	targets := d.targets
	if targets == nil {
		targets = []string{}
	}

	payload := map[string]any{
		"msgtype": "text",
		"at": map[string]any{
			"atMobiles": targets,
			"isAtAll":   false,
		},
		"text": map[string]any{
			"content": message,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	u := url.URL{
		Scheme: "https",
		Host:   "oapi.dingtalk.com",
		Path:   "/robot/send",
	}
	q := url.Values{}
	q.Set("access_token", d.token)
	if d.secret != "" {
		timestamp, signature := d.signature()
		q.Set("timestamp", timestamp)
		q.Set("sign", signature)
	}
	u.RawQuery = q.Encode()

	return RequestSpec{
		Method: "POST",
		URL:    u.String(),
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
		},
		Body: string(data),
	}, nil
}

func (d *DingTalkTarget) signature() (string, string) {
	timestamp := fmt.Sprintf("%d", fixedTime().UnixNano()/int64(time.Millisecond))
	seed := timestamp + "\n" + d.secret
	mac := hmac.New(sha256.New, []byte(d.secret))
	_, _ = mac.Write([]byte(seed))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return timestamp, url.QueryEscape(signature)
}

var dingtalkSecretRegex = regexp.MustCompile(`(?i)^[a-z0-9]+$`)

func init() {
	RegisterSchemaEntryOrdered(15, SchemaEntry{
		"attachment_support": false,
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
				"secret": map[string]any{
					"alias_of": "secret",
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
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{token}/", "{schema}://{token}/{targets}/", "{schema}://{secret}@{token}/", "{schema}://{secret}@{token}/{targets}/"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "dingtalk",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"dingtalk"},
				},
				"secret": map[string]any{
					"map_to":   "secret",
					"name":     "Secret",
					"private":  true,
					"regex":    []string{"^[a-z0-9]+$", "i"},
					"required": false,
					"type":     "string",
				},
				"target_phone_no": map[string]any{
					"map_to":   "targets",
					"name":     "Target Phone No",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_phone_no"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Token",
					"private":  true,
					"regex":    []string{"^[a-z0-9]+$", "i"},
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": nil,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"dingtalk"},
		"service_name":     "DingTalk",
		"service_url":      "https://www.dingtalk.com/",
		"setup_url":        "https://appriseit.com/services/dingtalk/",
	})
}
