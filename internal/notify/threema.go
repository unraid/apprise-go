package notify

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

const threemaNotifyURL = "https://msgapi.threema.ch/send_simple"

type threemaRecipient struct {
	key   string
	value string
}

type ThreemaTarget struct {
	user       string
	secret     string
	recipients []threemaRecipient
}

func NewThreemaTarget(target *ParsedURL) (*ThreemaTarget, error) {
	user := strings.TrimSpace(target.User)
	if from := strings.TrimSpace(target.Query["from"]); from != "" {
		user = from
	} else if gwid := strings.TrimSpace(target.Query["gwid"]); gwid != "" {
		user = gwid
	}

	secret := strings.TrimSpace(target.Query["secret"])
	if secret == "" {
		secret = strings.TrimSpace(target.Host)
	}

	if user == "" {
		return nil, fmt.Errorf("missing gateway id")
	}
	if secret == "" {
		return nil, fmt.Errorf("missing secret")
	}

	targets := splitPath(target.Path)
	if toRaw := strings.TrimSpace(target.Query["to"]); toRaw != "" {
		targets = append(targets, parseDelimitedList(toRaw)...)
	}
	if toRaw := strings.TrimSpace(target.Query["targets"]); toRaw != "" {
		targets = append(targets, parseDelimitedList(toRaw)...)
	}
	if toRaw := strings.TrimSpace(target.Query["target_threema_id"]); toRaw != "" {
		targets = append(targets, parseDelimitedList(toRaw)...)
	}
	if toRaw := strings.TrimSpace(target.Query["target_email"]); toRaw != "" {
		targets = append(targets, parseDelimitedList(toRaw)...)
	}
	if toRaw := strings.TrimSpace(target.Query["target_phone"]); toRaw != "" {
		targets = append(targets, parseDelimitedList(toRaw)...)
	}

	recipients := make([]threemaRecipient, 0, len(targets))
	for _, entry := range targets {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if len(entry) == 8 {
			recipients = append(recipients, threemaRecipient{key: "to", value: entry})
			continue
		}
		if isSimpleEmail(entry) {
			recipients = append(recipients, threemaRecipient{key: "email", value: entry})
			continue
		}
		if normalized, ok := normalizePhone(entry); ok {
			recipients = append(recipients, threemaRecipient{key: "phone", value: normalized})
			continue
		}
	}

	sort.Slice(recipients, func(i, j int) bool {
		if recipients[i].key != recipients[j].key {
			return recipients[i].key < recipients[j].key
		}
		return recipients[i].value < recipients[j].value
	})

	return &ThreemaTarget{
		user:       user,
		secret:     secret,
		recipients: recipients,
	}, nil
}

func (t *ThreemaTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(t.recipients) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	spec := t.buildSpec(message, t.recipients[0])
	_ = notifyType
	return spec, nil
}

func (t *ThreemaTarget) Send(body, title string, notifyType NotifyType) error {
	if len(t.recipients) == 0 {
		return nil
	}

	message := mergeTitleBody(title, body)
	for _, recipient := range t.recipients {
		spec := t.buildSpec(message, recipient)
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = notifyType

	return nil
}

func (t *ThreemaTarget) buildSpec(message string, recipient threemaRecipient) RequestSpec {
	values := url.Values{}
	values.Set("secret", t.secret)
	values.Set("from", t.user)
	values.Set("text", message)
	values.Set(recipient.key, recipient.value)

	return RequestSpec{
		Method: "POST",
		URL:    threemaNotifyURL + "?" + values.Encode(),
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/x-www-form-urlencoded; charset=utf-8",
			"Accept":       "*/*",
		},
	}
}

func init() {
	RegisterSchemaEntryOrdered(61, SchemaEntry{
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
				"from": map[string]any{
					"alias_of": "gateway_id",
				},
				"gwid": map[string]any{
					"alias_of": "gateway_id",
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
			"templates": []string{"{schema}://{gateway_id}@{secret}/{targets}"},
			"tokens": map[string]any{
				"gateway_id": map[string]any{
					"map_to":   "user",
					"name":     "Gateway ID",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "threema",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"threema"},
				},
				"secret": map[string]any{
					"map_to":   "secret",
					"name":     "API Secret",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"target_email": map[string]any{
					"map_to":   "targets",
					"name":     "Target Email",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_phone": map[string]any{
					"map_to":   "targets",
					"name":     "Target Phone No",
					"prefix":   "+",
					"private":  false,
					"regex":    []string{"^[0-9\\s)(+-]+$", "i"},
					"required": false,
					"type":     "string",
				},
				"target_threema_id": map[string]any{
					"map_to":   "targets",
					"name":     "Target Threema ID",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_email", "target_phone", "target_threema_id"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": true,
					"type":     "list:string",
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
		"secure_protocols": []string{"threema"},
		"service_name":     "Threema Gateway",
		"service_url":      "https://gateway.threema.ch/",
		"setup_url":        "https://appriseit.com/services/threema/",
	})
}
