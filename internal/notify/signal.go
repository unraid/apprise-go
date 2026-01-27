package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const signalBatchSize = 10

var signalGroupRegex = regexp.MustCompile(`(?i)^[a-z0-9_=-]+$`)

type SignalTarget struct {
	host     string
	port     int
	secure   bool
	user     string
	password string
	hasPass  bool
	source   string
	targets  []string
	batch    bool
	status   bool
}

func NewSignalTarget(target *ParsedURL) (*SignalTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	sourceRaw := strings.TrimSpace(target.Query["from"])
	if sourceRaw == "" {
		sourceRaw = strings.TrimSpace(target.Query["source"])
	}

	rawTargets := splitPath(target.Path)
	if sourceRaw == "" {
		if len(rawTargets) == 0 {
			return nil, fmt.Errorf("missing source")
		}
		sourceRaw = rawTargets[0]
		rawTargets = rawTargets[1:]
	}

	sourceDigits, ok := normalizePhone(sourceRaw)
	if !ok {
		return nil, fmt.Errorf("invalid source")
	}
	source := "+" + sourceDigits

	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		rawTargets = append(rawTargets, parseDelimitedList(toValue)...)
	}

	targets := []string{}
	for _, entry := range rawTargets {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if normalized, ok := normalizePhone(entry); ok {
			targets = append(targets, "+"+normalized)
			continue
		}

		group := parseSignalGroup(entry)
		if group != "" {
			targets = append(targets, "group."+group)
		}
	}

	if len(targets) == 0 {
		targets = []string{source}
	}

	return &SignalTarget{
		host:     host,
		port:     target.Port,
		secure:   target.Scheme == "signals",
		user:     strings.TrimSpace(target.User),
		password: target.Password,
		hasPass:  target.HasPassword,
		source:   source,
		targets:  targets,
		batch:    parseBoolWithDefault(target.Query["batch"], false),
		status:   parseBoolWithDefault(target.Query["status"], false),
	}, nil
}

func (s *SignalTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(s.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	if s.status {
		message = strings.TrimSpace(notifyTypeASCII(notifyType) + " " + message)
	}

	recipients := s.targets
	if s.batch && len(recipients) > signalBatchSize {
		recipients = recipients[:signalBatchSize]
	} else if !s.batch {
		recipients = recipients[:1]
	}

	payload := map[string]any{
		"message":    message,
		"number":     s.source,
		"text_mode":  "normal",
		"recipients": recipients,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/json",
	}
	if s.user != "" {
		password := s.password
		if !s.hasPass {
			password = "None"
		}
		headers["Authorization"] = basicAuthHeader(s.user, password)
	}

	return RequestSpec{
		Method:  "POST",
		URL:     s.buildURL(),
		Headers: headers,
		Body:    string(data),
	}, nil
}

func (s *SignalTarget) Send(body, title string, notifyType NotifyType) error {
	if len(s.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	if s.status {
		message = strings.TrimSpace(notifyTypeASCII(notifyType) + " " + message)
	}

	batchSize := 1
	if s.batch {
		batchSize = signalBatchSize
	}

	for index := 0; index < len(s.targets); index += batchSize {
		end := index + batchSize
		if end > len(s.targets) {
			end = len(s.targets)
		}

		payload := map[string]any{
			"message":    message,
			"number":     s.source,
			"text_mode":  "normal",
			"recipients": s.targets[index:end],
		}

		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		headers := map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
		}
		if s.user != "" {
			password := s.password
			if !s.hasPass {
				password = "None"
			}
			headers["Authorization"] = basicAuthHeader(s.user, password)
		}

		spec := RequestSpec{
			Method:  "POST",
			URL:     s.buildURL(),
			Headers: headers,
			Body:    string(data),
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func (s *SignalTarget) buildURL() string {
	schema := "http"
	if s.secure {
		schema = "https"
	}

	url := schema + "://" + s.host
	if s.port != 0 {
		url += fmt.Sprintf(":%d", s.port)
	}
	return url + "/v2/send"
}

func parseSignalGroup(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}

	lower := strings.ToLower(value)
	switch {
	case strings.HasPrefix(lower, "@group."):
		value = value[len("@group."):]
	case strings.HasPrefix(lower, "group."):
		value = value[len("group."):]
	case strings.HasPrefix(value, "@"):
		value = value[1:]
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !signalGroupRegex.MatchString(value) {
		return ""
	}
	return value
}

func init() {
	RegisterSchemaEntryOrdered(121, SchemaEntry{
		"attachment_support": true,
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
				"status": map[string]any{
					"default":  false,
					"map_to":   "status",
					"name":     "Show Status",
					"private":  false,
					"required": false,
					"type":     "bool",
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
			"templates": []string{"{schema}://{host}/{from_phone}", "{schema}://{host}:{port}/{from_phone}", "{schema}://{user}@{host}/{from_phone}", "{schema}://{user}@{host}:{port}/{from_phone}", "{schema}://{user}:{password}@{host}/{from_phone}", "{schema}://{user}:{password}@{host}:{port}/{from_phone}", "{schema}://{host}/{from_phone}/{targets}", "{schema}://{host}:{port}/{from_phone}/{targets}", "{schema}://{user}@{host}/{from_phone}/{targets}", "{schema}://{user}@{host}:{port}/{from_phone}/{targets}", "{schema}://{user}:{password}@{host}/{from_phone}/{targets}", "{schema}://{user}:{password}@{host}:{port}/{from_phone}/{targets}"},
			"tokens": map[string]any{
				"from_phone": map[string]any{
					"map_to":   "source",
					"name":     "From Phone No",
					"private":  false,
					"regex":    []string{"^\\+?[0-9\\s)(+-]+$", "i"},
					"required": true,
					"type":     "string",
				},
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
					"values":   []string{"signal", "signals"},
				},
				"target_channel": map[string]any{
					"map_to":   "targets",
					"name":     "Target Group ID",
					"prefix":   "@",
					"private":  false,
					"regex":    []string{"^[a-z0-9_=-]+$", "i"},
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
					"group":    []string{"target_channel", "target_phone"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
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
		"protocols": []string{"signal"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"signals"},
		"service_name":     "Signal API",
		"service_url":      "https://bbernhard.github.io/signal-cli-rest-api/",
		"setup_url":        "https://appriseit.com/services/signal/",
	})
}
