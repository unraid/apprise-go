package notify

import (
	"cmp"
	"encoding/json"
	"fmt"
	"strings"
)

const pushplusURL = "https://www.pushplus.plus/send"

const (
	pushplusChannelWeChat  = "wechat"
	pushplusChannelWebhook = "webhook"
	pushplusChannelWeCom   = "cp"
	pushplusChannelMail    = "mail"
	pushplusChannelSMS     = "sms"

	pushplusDefaultTemplate = "html"
)

var pushplusChannels = map[string]struct{}{
	pushplusChannelWeChat:  {},
	pushplusChannelWebhook: {},
	pushplusChannelWeCom:   {},
	pushplusChannelMail:    {},
	pushplusChannelSMS:     {},
}

// pushplusFormatTemplates maps an Apprise notify format onto the server side
// rendering hint PushPlus expects.
var pushplusFormatTemplates = map[string]string{
	"html":     "html",
	"markdown": "markdown",
	"text":     "txt",
}

type PushplusTarget struct {
	token    string
	topics   []string
	channel  string
	webhook  string
	template string
}

func NewPushplusTarget(target *ParsedURL) (*PushplusTarget, error) {
	token := target.Host
	if rawToken, ok := target.Query["token"]; ok && rawToken != "" {
		token = rawToken
	}
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	topics := splitPath(target.Path)
	for _, key := range []string{"to", "topic"} {
		if raw := strings.TrimSpace(target.Query[key]); raw != "" {
			topics = append(topics, parseDelimitedList(raw)...)
		}
	}

	// The webhook name may be supplied either as ?name= or as the user
	// portion of the URL, which also implies the webhook channel.
	webhook := strings.TrimSpace(target.Query["name"])
	if webhook == "" {
		webhook = strings.TrimSpace(target.User)
	}

	channel := ""
	// wecom:// is an alias that pins the channel to WeCom.
	if strings.EqualFold(target.Scheme, "wecom") {
		channel = pushplusChannelWeCom
	}
	if raw := strings.TrimSpace(cmp.Or(target.Query["channel"], target.Query["mode"])); raw != "" {
		candidate := strings.ToLower(raw)
		if candidate == "wecom" {
			candidate = pushplusChannelWeCom
		}
		if _, ok := pushplusChannels[candidate]; ok {
			channel = candidate
		}
	}
	if channel == "" {
		channel = pushplusChannelWeChat
		if webhook != "" {
			channel = pushplusChannelWebhook
		}
	}

	template := pushplusDefaultTemplate
	if mapped, ok := pushplusFormatTemplates[normalizeNotifyFormat(target.Query["format"])]; ok {
		template = mapped
	}

	return &PushplusTarget{
		token:    token,
		topics:   topics,
		channel:  channel,
		webhook:  webhook,
		template: template,
	}, nil
}

func (p *PushplusTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	return p.buildRequests(body, title, notifyType)[0], nil
}

func (p *PushplusTarget) Send(body, title string, notifyType NotifyType) error {
	// Upstream keeps going after a failed target; see sendOutcome.
	var outcome sendOutcome
	for _, spec := range p.buildRequests(body, title, notifyType) {
		outcome.record(SendRequest(spec))
	}

	return outcome.err()
}

// buildRequests returns one request per group topic, or a single personal
// notification when no topic is configured.
func (p *PushplusTarget) buildRequests(body, title string, notifyType NotifyType) []RequestSpec {
	_ = notifyType

	resolvedTitle := title
	if resolvedTitle == "" {
		resolvedTitle = body
	}

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/json",
	}

	topics := p.topics
	if len(topics) == 0 {
		topics = []string{""}
	}

	specs := make([]RequestSpec, 0, len(topics))
	for _, topic := range topics {
		payload := map[string]any{
			"token":    p.token,
			"title":    resolvedTitle,
			"content":  body,
			"template": p.template,
			"channel":  p.channel,
		}
		if topic != "" {
			payload["topic"] = topic
		}
		if p.channel == pushplusChannelWebhook && p.webhook != "" {
			payload["webhook"] = p.webhook
		}

		data, err := json.Marshal(payload)
		if err != nil {
			continue
		}

		specs = append(specs, RequestSpec{
			Method:  "POST",
			URL:     pushplusURL,
			Headers: headers,
			Body:    string(data),
		})
	}

	return specs
}

func init() {
	RegisterSchemaEntryOrdered(69, SchemaEntry{
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
					"default":  "html",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"channel": map[string]any{
					"default":  "wechat",
					"map_to":   "channel",
					"name":     "Channel",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"wechat", "webhook", "cp", "mail", "sms"},
				},
				"mode": map[string]any{
					"alias_of": "channel",
				},
				"name": map[string]any{
					"map_to":   "webhook",
					"name":     "Webhook Name",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"to": map[string]any{
					"alias_of": "targets",
					"delim":    []string{",", " "},
				},
				"token": map[string]any{
					"alias_of": "token",
				},
				"topic": map[string]any{
					"alias_of": "targets",
					"delim":    []string{",", " "},
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
			"templates": []string{"{schema}://{token}", "{schema}://{token}/{targets}"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"pushplus", "wecom"},
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{},
					"map_to":   "targets",
					"name":     "Group Topics",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "User Token",
					"private":  true,
					"regex":    []string{"^[a-z0-9_-]{32,64}$", "i"},
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
		"secure_protocols": []string{"pushplus", "wecom"},
		"service_name":     "Pushplus",
		"service_url":      "https://www.pushplus.plus/",
		"setup_url":        "https://appriseit.com/services/pushplus/",
	})
}
