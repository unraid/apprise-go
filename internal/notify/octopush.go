package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	octopushURL = "https://api.octopush.com/v1/public/sms-campaign/send"

	// A batched campaign carries up to this many recipients per request.
	octopushBatchSize = 500
)

type OctopushTarget struct {
	apiLogin string
	apiKey   string
	targets  []string
	sender   string
	mtype    string
	purpose  string
	replies  bool
	batch    bool
}

func NewOctopushTarget(target *ParsedURL) (*OctopushTarget, error) {
	tokens := splitPath(target.Path)

	apiKey := strings.TrimSpace(target.Query["key"])
	if apiKey == "" && len(tokens) > 0 {
		apiKey = tokens[0]
		tokens = tokens[1:]
	}
	if apiKey == "" {
		return nil, fmt.Errorf("missing api key")
	}

	// The login is an email address, assembled from the credentials and host
	// unless ?login= supplies it outright.
	apiLogin := strings.TrimSpace(target.Query["login"])
	if apiLogin == "" {
		user := strings.TrimSpace(target.Password)
		if user == "" {
			user = strings.TrimSpace(target.User)
		}
		if user != "" {
			apiLogin = user + "@" + strings.TrimSpace(target.Host)
		}
	}
	if !strings.Contains(apiLogin, "@") {
		return nil, fmt.Errorf("missing or invalid api login")
	}

	entries := tokens
	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		entries = append(entries, parseDelimitedList(to)...)
	}

	targets := make([]string, 0, len(entries))
	for _, entry := range entries {
		if normalized, ok := normalizePhone(entry); ok {
			targets = append(targets, "+"+normalized)
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	// A user and password together mean the user names the sender.
	sender := strings.TrimSpace(target.Query["sender"])
	if sender == "" && target.User != "" && target.Password != "" {
		sender = strings.TrimSpace(target.User)
	}

	mtype := "sms_premium"
	if candidate := strings.ToLower(strings.TrimSpace(target.Query["type"])); candidate == "sms_low_cost" {
		mtype = candidate
	}

	purpose := "alert"
	if candidate := strings.ToLower(strings.TrimSpace(target.Query["purpose"])); candidate == "wholesale" {
		purpose = candidate
	}

	return &OctopushTarget{
		apiLogin: apiLogin,
		apiKey:   apiKey,
		targets:  targets,
		sender:   sender,
		mtype:    mtype,
		purpose:  purpose,
		replies:  parseBool(target.Query["replies"], false),
		batch:    parseBool(target.Query["batch"], false),
	}, nil
}

func (o *OctopushTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	specs, err := o.buildRequests(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}

	return specs[0], nil
}

func (o *OctopushTarget) Send(body, title string, notifyType NotifyType) error {
	specs, err := o.buildRequests(body, title, notifyType)
	if err != nil {
		return err
	}

	for _, spec := range specs {
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func (o *OctopushTarget) buildRequests(body, title string, notifyType NotifyType) ([]RequestSpec, error) {
	_ = notifyType

	sender := o.sender
	if sender == "" {
		sender = "Apprise"
	}

	headers := map[string]string{
		"User-Agent": "Apprise",
		// Octopush is stricter than most: it wants a JSON Accept and a
		// charset on the content type.
		"Accept":        "application/json",
		"Content-Type":  "application/json; charset=utf-8",
		"api-key":       o.apiKey,
		"api-login":     o.apiLogin,
		"cache-control": "no-cache",
	}

	size := 1
	if o.batch {
		size = octopushBatchSize
	}

	var specs []RequestSpec
	for start := 0; start < len(o.targets); start += size {
		end := min(start+size, len(o.targets))

		recipients := make([]any, 0, end-start)
		for _, phone := range o.targets[start:end] {
			recipients = append(recipients, map[string]any{"phone_number": phone})
		}

		data, err := json.Marshal(map[string]any{
			"recipients":   recipients,
			"text":         mergeTitleBody(title, body),
			"type":         o.mtype,
			"purpose":      o.purpose,
			"sender":       sender,
			"with_replies": o.replies,
		})
		if err != nil {
			return nil, err
		}

		specs = append(specs, RequestSpec{
			Method:  "POST",
			URL:     octopushURL,
			Headers: headers,
			Body:    string(data),
		})
	}

	return specs, nil
}

func init() {
	RegisterSchemaEntryOrdered(156, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"batch": map[string]any{
					"default":  false,
					"map_to":   "batch",
					"name":     "Batch Mode",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
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
				"key": map[string]any{
					"alias_of": "api_key",
				},
				"login": map[string]any{
					"alias_of": "api_login",
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
				"purpose": map[string]any{
					"default":  "alert",
					"map_to":   "purpose",
					"name":     "Purpose",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"alert", "wholesale"},
				},
				"redirect": map[string]any{
					"default":  true,
					"map_to":   "redirect",
					"name":     "Follow Redirects",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"replies": map[string]any{
					"default":  false,
					"map_to":   "replies",
					"name":     "Accept Replies",
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
				"type": map[string]any{
					"default":  "sms_premium",
					"map_to":   "mtype",
					"name":     "Type",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"sms_premium", "sms_low_cost"},
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
			"templates": []string{"{schema}://{api_login}/{api_key}/{targets}", "{schema}://{sender}:{api_login}/{api_key}/{targets}"},
			"tokens": map[string]any{
				"api_key": map[string]any{
					"map_to":   "api_key",
					"name":     "API Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"api_login": map[string]any{
					"map_to":   "api_login",
					"name":     "API Login",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "octopush",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"octopush"},
				},
				"sender": map[string]any{
					"map_to":   "sender",
					"name":     "Sender",
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
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_phone"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
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
		"secure_protocols": []string{"octopush"},
		"service_name":     "Octopush",
		"service_url":      "https://octopush.com/",
		"setup_url":        "https://appriseit.com/services/octopush/",
	})
}
