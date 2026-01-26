package notify

import (
	"fmt"
	"net/url"
	"strings"
)

const voipmsURL = "https://voip.ms/api/v1/rest.php"

type VoipmsTarget struct {
	email   string
	passwd  string
	source  string
	targets []string
}

func NewVoipmsTarget(target *ParsedURL) (*VoipmsTarget, error) {
	emailLocal := strings.TrimSpace(target.Password)
	if emailLocal == "" || strings.TrimSpace(target.Host) == "" {
		return nil, fmt.Errorf("missing email")
	}
	email := emailLocal + "@" + strings.TrimSpace(target.Host)

	passwd := strings.TrimSpace(target.User)
	if passwd == "" {
		return nil, fmt.Errorf("missing password")
	}

	rawTargets := splitPath(target.Path)

	sourceRaw := strings.TrimSpace(target.Query["from"])
	if sourceRaw == "" {
		if len(rawTargets) > 0 {
			sourceRaw = rawTargets[0]
			rawTargets = rawTargets[1:]
		}
	}
	if sourceRaw == "" {
		return nil, fmt.Errorf("missing source")
	}

	source, ok := normalizeVoipmsNumber(sourceRaw)
	if !ok {
		return nil, fmt.Errorf("invalid source")
	}

	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		rawTargets = append(rawTargets, parseDelimitedList(toValue)...)
	}

	targets := []string{}
	for _, entry := range rawTargets {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if normalized, ok := normalizeVoipmsNumber(entry); ok {
			targets = append(targets, normalized)
		}
	}

	if len(targets) == 0 {
		targets = []string{source}
	}

	return &VoipmsTarget{
		email:   email,
		passwd:  passwd,
		source:  source,
		targets: targets,
	}, nil
}

func (v *VoipmsTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(v.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	payload := v.buildPayload(message, v.targets[0])

	requestURL := voipmsURL + "?" + payload.Encode()

	_ = notifyType

	return RequestSpec{
		Method: "GET",
		URL:    requestURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/x-www-form-urlencoded",
		},
	}, nil
}

func (v *VoipmsTarget) Send(body, title string, notifyType NotifyType) error {
	if len(v.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)

	for _, target := range v.targets {
		payload := v.buildPayload(message, target)
		requestURL := voipmsURL + "?" + payload.Encode()

		spec := RequestSpec{
			Method: "GET",
			URL:    requestURL,
			Headers: map[string]string{
				"User-Agent":   "Apprise",
				"Accept":       "*/*",
				"Content-Type": "application/x-www-form-urlencoded",
			},
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = notifyType

	return nil
}

func (v *VoipmsTarget) buildPayload(message, target string) url.Values {
	values := url.Values{}
	values.Set("api_username", v.email)
	values.Set("api_password", v.passwd)
	values.Set("did", v.source)
	values.Set("message", message)
	values.Set("method", "sendSMS")
	values.Set("dst", target)
	return values
}

func normalizeVoipmsNumber(raw string) (string, bool) {
	normalized, ok := normalizePhone(raw)
	if !ok {
		return "", false
	}
	if len(normalized) == 11 {
		if normalized[0] != '1' {
			return "", false
		}
		return normalized[1:], true
	}
	if len(normalized) != 10 {
		return "", false
	}
	return normalized, true
}

func init() {
	RegisterSchemaEntryOrdered(9, SchemaEntry{
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
					"alias_of": "from_phone",
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
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{password}:{email}/{from_phone}/{targets}"},
			"tokens": map[string]any{
				"email": map[string]any{
					"map_to":   "email",
					"name":     "User Email",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"from_phone": map[string]any{
					"map_to":   "source",
					"name":     "From Phone No",
					"private":  false,
					"regex":    []string{"^\\+?[0-9\\s)(+-]+$", "i"},
					"required": false,
					"type":     "string",
				},
				"password": map[string]any{
					"map_to":   "password",
					"name":     "Password",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "voipms",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"voipms"},
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
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_phone"},
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
		"secure_protocols": []string{"voipms"},
		"service_name":     "VoIPms",
		"service_url":      "https://voip.ms",
		"setup_url":        "https://appriseit.com/services/voipms/",
	})
}
