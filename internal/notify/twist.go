package notify

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const twistAPIBase = "https://api.twist.com/api/v3/"

var twistChannelIDPattern = regexp.MustCompile(`^(?:\d+:)?\d+$`)

type TwistTarget struct {
	email      string
	password   string
	channelIDs []string
	token      string
}

func NewTwistTarget(target *ParsedURL) (*TwistTarget, error) {
	user := strings.TrimSpace(target.User)
	pass := strings.TrimSpace(target.Password)
	host := strings.TrimSpace(target.Host)
	entries := splitPath(target.Path)

	email := ""
	password := ""
	if pass == "" {
		if len(entries) == 0 {
			return nil, fmt.Errorf("missing password")
		}
		password = strings.TrimSpace(entries[0])
		entries = entries[1:]
		if user != "" && host != "" {
			email = user + "@" + host
		} else if strings.Contains(host, "@") {
			email = host
		}
	} else {
		emailUser := pass
		emailHost := host
		password = user
		if emailUser != "" && emailHost != "" {
			email = emailUser + "@" + emailHost
		} else if strings.Contains(emailUser, "@") {
			email = emailUser
		}
	}

	if !isSimpleEmail(email) {
		return nil, fmt.Errorf("invalid email")
	}
	if password == "" {
		return nil, fmt.Errorf("missing password")
	}

	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		entries = append(entries, parseDelimitedList(toValue)...)
	}

	channelIDs := []string{}
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if twistChannelIDPattern.MatchString(entry) {
			channelIDs = append(channelIDs, entry)
		}
	}
	if len(channelIDs) == 0 {
		return nil, fmt.Errorf("missing channel ids")
	}

	return &TwistTarget{
		email:      email,
		password:   password,
		channelIDs: channelIDs,
	}, nil
}

func (t *TwistTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	form := url.Values{}
	form.Set("email", t.email)
	form.Set("password", t.password)
	return RequestSpec{
		Method: "POST",
		URL:    twistAPIBase + "users/login",
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/x-www-form-urlencoded; charset=utf-8",
		},
		Body: form.Encode(),
	}, nil
}

func (t *TwistTarget) Send(body, title string, notifyType NotifyType) error {
	if t.token == "" {
		if err := t.login(); err != nil {
			return err
		}
	}

	for _, channelID := range t.channelIDs {
		channelID = twistChannelOnly(channelID)

		payload := url.Values{}
		payload.Set("channel_id", channelID)
		payload.Set("title", title)
		payload.Set("content", body)

		spec := RequestSpec{
			Method: "POST",
			URL:    twistAPIBase + "threads/add",
			Headers: map[string]string{
				"User-Agent":   "Apprise",
				"Content-Type": "application/x-www-form-urlencoded; charset=utf-8",
				"Authorization": fmt.Sprintf(
					"Bearer %s",
					t.token,
				),
			},
			Body: payload.Encode(),
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = notifyType
	return nil
}

func (t *TwistTarget) login() error {
	payload := url.Values{}
	payload.Set("email", t.email)
	payload.Set("password", t.password)

	spec := RequestSpec{
		Method: "POST",
		URL:    twistAPIBase + "users/login",
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/x-www-form-urlencoded; charset=utf-8",
		},
		Body: payload.Encode(),
	}

	var response struct {
		Token string `json:"token"`
	}
	if err := doJSONRequest(spec, &response); err != nil {
		return err
	}
	if response.Token == "" {
		return fmt.Errorf("missing token")
	}
	t.token = response.Token
	return nil
}

func twistChannelOnly(value string) string {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) == 0 {
		return value
	}
	return parts[len(parts)-1]
}

func init() {
	RegisterSchemaEntryOrdered(48, SchemaEntry{
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
					"default":  "markdown",
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
			"templates": []string{"{schema}://{password}:{email}", "{schema}://{password}:{email}/{targets}"},
			"tokens": map[string]any{
				"email": map[string]any{
					"map_to":   "email",
					"name":     "Email",
					"private":  false,
					"required": true,
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
					"default":  "twist",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"twist"},
				},
				"target_channel": map[string]any{
					"map_to":   "targets",
					"name":     "Target Channel",
					"prefix":   "#",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_channel_id": map[string]any{
					"map_to":   "targets",
					"name":     "Target Channel ID",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_channel", "target_channel_id"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
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
		"secure_protocols": []string{"twist"},
		"service_name":     "Twist",
		"service_url":      "https://twist.com",
		"setup_url":        "https://appriseit.com/services/twist/",
	})
}
