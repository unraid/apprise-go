package notify

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
)

const (
	sendPulseEmailURL = "https://api.sendpulse.com/smtp/emails"
	sendPulseOAuthURL = "https://api.sendpulse.com/oauth/access_token"
	sendPulseSubject  = "<no subject>"
)

type SendPulseTarget struct {
	clientID     string
	clientSecret string
	fromAddr     string
	fromName     string
	targets      []string
	cc           map[string]struct{}
	bcc          map[string]struct{}
	names        map[string]string
	notifyFormat string
	templateID   int
	templateData map[string]string
}

func NewSendPulseTarget(target *ParsedURL) (*SendPulseTarget, error) {
	user := strings.TrimSpace(target.User)
	host := strings.TrimSpace(target.Host)

	rawTargets := []string{}
	names := map[string]string{}
	fromName := "Apprise"
	fromAddr := ""

	rawFrom := strings.TrimSpace(target.Query["from"])
	fromEntry, fromIsEmail := parseSendPulseEmail(rawFrom)
	userEntry, userIsEmail := parseSendPulseEmail(user)

	if fromIsEmail || userIsEmail {
		if host != "" {
			rawTargets = append(rawTargets, host)
			host = ""
		}
	}

	if fromIsEmail {
		fromAddr = fromEntry.email
		if fromEntry.name != "" {
			fromName = fromEntry.name
		}
	} else if rawFrom != "" {
		fromName = rawFrom
	}

	if fromAddr == "" {
		if user != "" && host != "" {
			userPart := strings.FieldsFunc(user, func(r rune) bool {
				return r == '@' || r == ' ' || r == '\t' || r == '\n'
			})
			if len(userPart) > 0 {
				fromAddr = userPart[0] + "@" + host
			}
		} else if userIsEmail {
			fromAddr = userEntry.email
			if fromEntry.email == "" && userEntry.name != "" {
				fromName = userEntry.name
			}
		}
	}

	if !isSimpleEmail(fromAddr) {
		return nil, fmt.Errorf("invalid from address")
	}
	if fromName != "" {
		names[fromAddr] = fromName
	}

	entries := splitPath(target.Path)
	clientID := strings.TrimSpace(target.Query["id"])
	if clientID == "" && len(rawTargets) > 0 {
		clientID = strings.TrimSpace(rawTargets[0])
		rawTargets = rawTargets[1:]
	}
	if clientID == "" && len(entries) > 0 {
		clientID = strings.TrimSpace(entries[0])
		entries = entries[1:]
	}
	if clientID == "" {
		return nil, fmt.Errorf("missing client id")
	}

	clientSecret := strings.TrimSpace(target.Query["secret"])
	if clientSecret == "" && len(rawTargets) > 0 {
		clientSecret = strings.TrimSpace(rawTargets[0])
		rawTargets = rawTargets[1:]
	}
	if clientSecret == "" && len(entries) > 0 {
		clientSecret = strings.TrimSpace(entries[0])
		entries = entries[1:]
	}
	if clientSecret == "" {
		return nil, fmt.Errorf("missing client secret")
	}

	rawTargets = append(rawTargets, entries...)
	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		rawTargets = append(rawTargets, toValue)
	}

	targets := []string{}
	for _, entry := range rawTargets {
		for _, candidate := range parseDelimitedList(entry) {
			if parsed, ok := parseSendPulseEmail(candidate); ok {
				targets = append(targets, parsed.email)
				if parsed.name != "" {
					names[parsed.email] = parsed.name
				}
			}
		}
	}
	if len(targets) == 0 {
		targets = append(targets, fromAddr)
	}

	cc := map[string]struct{}{}
	if ccValue := strings.TrimSpace(target.Query["cc"]); ccValue != "" {
		for _, entry := range parseDelimitedList(ccValue) {
			if parsed, ok := parseSendPulseEmail(entry); ok {
				cc[parsed.email] = struct{}{}
				if parsed.name != "" {
					names[parsed.email] = parsed.name
				}
			}
		}
	}

	bcc := map[string]struct{}{}
	if bccValue := strings.TrimSpace(target.Query["bcc"]); bccValue != "" {
		for _, entry := range parseDelimitedList(bccValue) {
			if parsed, ok := parseSendPulseEmail(entry); ok {
				bcc[parsed.email] = struct{}{}
				if parsed.name != "" {
					names[parsed.email] = parsed.name
				}
			}
		}
	}

	format := normalizeNotifyFormat(target.Query["format"])
	if format == "" {
		format = "html"
	}
	switch format {
	case "html", "markdown", "text":
	default:
		return nil, fmt.Errorf("invalid format")
	}

	templateID := 0
	if templateValue := strings.TrimSpace(target.Query["template"]); templateValue != "" {
		if parsed, err := strconv.Atoi(templateValue); err == nil {
			templateID = parsed
		} else {
			return nil, fmt.Errorf("invalid template id")
		}
	}

	templateData := map[string]string{}
	for key, value := range target.QueryAdd {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		templateData[key] = value
	}

	return &SendPulseTarget{
		clientID:     clientID,
		clientSecret: clientSecret,
		fromAddr:     fromAddr,
		fromName:     fromName,
		targets:      targets,
		cc:           cc,
		bcc:          bcc,
		names:        names,
		notifyFormat: format,
		templateID:   templateID,
		templateData: templateData,
	}, nil
}

func (s *SendPulseTarget) Send(body, title string, notifyType NotifyType) error {
	token, err := s.login()
	if err != nil {
		return err
	}

	for _, target := range s.targets {
		payload := s.buildEmailPayload(body, title, target)
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		req, err := http.NewRequest("POST", sendPulseEmailURL, strings.NewReader(string(data)))
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "Apprise")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "*/*")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return &HTTPStatusError{StatusCode: resp.StatusCode}
		}
	}

	_ = notifyType
	return nil
}

func (s *SendPulseTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload := map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     s.clientID,
		"client_secret": s.clientSecret,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = body
	_ = title
	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    sendPulseOAuthURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/json",
		},
		Body: string(data),
	}, nil
}

func (s *SendPulseTarget) login() (string, error) {
	spec, err := s.BuildRequest("", "", NotifyInfo)
	if err != nil {
		return "", err
	}

	req, err := spec.HTTPRequest()
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", &HTTPStatusError{StatusCode: resp.StatusCode}
	}

	var response struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", err
	}
	if response.AccessToken == "" {
		return "", fmt.Errorf("missing access token")
	}
	return response.AccessToken, nil
}

func (s *SendPulseTarget) buildEmailPayload(body, title, target string) map[string]any {
	subject := title
	if subject == "" {
		subject = sendPulseSubject
	}

	emailPayload := map[string]any{
		"from": map[string]any{
			"name":  s.fromName,
			"email": s.fromAddr,
		},
		"to":      []map[string]any{},
		"subject": subject,
		"text":    body,
	}

	to := map[string]any{
		"email": target,
	}
	if name := s.names[target]; name != "" {
		to["name"] = name
	}
	emailPayload["to"] = []map[string]any{to}

	if s.notifyFormat == "html" {
		emailPayload["html"] = base64.StdEncoding.EncodeToString([]byte(body))
	}

	if len(s.cc) > 0 {
		ccList := []map[string]any{}
		for entry := range s.cc {
			if entry == target {
				continue
			}
			item := map[string]any{"email": entry}
			if name := s.names[entry]; name != "" {
				item["name"] = name
			}
			ccList = append(ccList, item)
		}
		if len(ccList) > 0 {
			emailPayload["cc"] = ccList
		}
	}

	if len(s.bcc) > 0 {
		bccList := []map[string]any{}
		for entry := range s.bcc {
			if entry == target {
				continue
			}
			item := map[string]any{"email": entry}
			if name := s.names[entry]; name != "" {
				item["name"] = name
			}
			bccList = append(bccList, item)
		}
		if len(bccList) > 0 {
			emailPayload["bcc"] = bccList
		}
	}

	if s.templateID > 0 {
		emailPayload["template"] = map[string]any{
			"id":        s.templateID,
			"variables": s.templateData,
		}
	}

	return map[string]any{
		"email": emailPayload,
	}
}

func parseSendPulseEmail(raw string) (emailEntry, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return emailEntry{}, false
	}

	if parsed, ok := parseEmailEntry(trimmed); ok {
		return parsed, true
	}

	if addr, err := mail.ParseAddress(trimmed); err == nil {
		if isSimpleEmail(addr.Address) {
			return emailEntry{name: strings.TrimSpace(addr.Name), email: addr.Address}, true
		}
	}

	if isSimpleEmail(trimmed) {
		return emailEntry{email: trimmed}, true
	}

	return emailEntry{}, false
}

func init() {
	RegisterSchemaEntryOrdered(104, SchemaEntry{
		"attachment_support": true,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"bcc": map[string]any{
					"delim":    []string{",", " "},
					"group":    []any{},
					"map_to":   "bcc",
					"name":     "Blind Carbon Copy",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"cc": map[string]any{
					"delim":    []string{",", " "},
					"group":    []any{},
					"map_to":   "cc",
					"name":     "Carbon Copy",
					"private":  false,
					"required": false,
					"type":     "list:string",
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
					"default":  "html",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"from": map[string]any{
					"map_to":   "from_addr",
					"name":     "From Email",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"id": map[string]any{
					"alias_of": "client_id",
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
					"alias_of": "client_secret",
				},
				"store": map[string]any{
					"default":  true,
					"map_to":   "store",
					"name":     "Persistent Storage",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"template": map[string]any{
					"map_to":   "template",
					"name":     "Template ID",
					"private":  false,
					"required": false,
					"type":     "int",
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
			"kwargs": map[string]any{
				"template_data": map[string]any{
					"map_to":   "template_data",
					"name":     "Template Data",
					"prefix":   "+",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{user}@{host}/{client_secret}/", "{schema}://{user}@{host}/{client_id}/{client_secret}/{targets}"},
			"tokens": map[string]any{
				"client_id": map[string]any{
					"map_to":   "client_id",
					"name":     "Client ID",
					"private":  true,
					"regex":    []string{"^[A-Z0-9._-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"client_secret": map[string]any{
					"map_to":   "client_secret",
					"name":     "Client Secret",
					"private":  true,
					"regex":    []string{"^[A-Z0-9._-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Domain",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "sendpulse",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"sendpulse"},
				},
				"target_email": map[string]any{
					"map_to":   "targets",
					"name":     "Target Email",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_email"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"user": map[string]any{
					"map_to":   "user",
					"name":     "User Name",
					"private":  false,
					"required": false,
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
		"secure_protocols": []string{"sendpulse"},
		"service_name":     "SendPulse",
		"service_url":      "https://sendpulse.com",
		"setup_url":        "https://appriseit.com/services/sendpulse/",
	})
}
