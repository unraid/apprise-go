package notify

import (
	"encoding/json"
	"fmt"
)

const freemobileURL = "https://smsapi.free-mobile.fr/sendmsg"

type FreeMobileTarget struct {
	user     string
	password string
}

func NewFreeMobileTarget(target *ParsedURL) (*FreeMobileTarget, error) {
	user := target.User
	password := target.Password
	if password == "" {
		password = target.Host
	}
	if user == "" || password == "" {
		return nil, fmt.Errorf("missing user or password")
	}

	return &FreeMobileTarget{user: user, password: password}, nil
}

func (f *FreeMobileTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	message := body
	if title != "" {
		message = title + "\r\n" + body
	}

	payload := map[string]string{
		"user": f.user,
		"pass": f.password,
		"msg":  message,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    freemobileURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
		},
		Body: string(data),
	}, nil
}

func (f *FreeMobileTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := f.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func init() {
	RegisterSchemaEntryOrdered(71, SchemaEntry{
		"service_name":       "Free-Mobile",
		"service_url":        "https://mobile.free.fr/",
		"setup_url":          "https://appriseit.com/services/freemobile/",
		"attachment_support": false,
		"category":           "native",
		"enabled":            true,
		"protocols":          []string(nil),
		"secure_protocols":   []string{"freemobile"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []string{},
			"packages_required":    []string{},
		},
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
			"kwargs": map[string]any{},
			"templates": []string{
				"{schema}://{user}@{password}",
			},
			"tokens": map[string]any{
				"password": map[string]any{
					"map_to":   "password",
					"name":     "Password",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "freemobile",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"freemobile"},
				},
				"user": map[string]any{
					"map_to":   "user",
					"name":     "Username",
					"private":  false,
					"required": true,
					"type":     "string",
				},
			},
		},
	})
}
