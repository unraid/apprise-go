package notify

import (
	"fmt"
	"net/url"
	"strings"
)

const mailgunBatchSize = 2000
const mailgunDefaultName = "Apprise"

var mailgunAPIBase = map[string]string{
	"us": "https://api.mailgun.net/v3/",
	"eu": "https://api.eu.mailgun.net/v3/",
}

type MailgunTarget struct {
	apiKey   string
	fromAddr string
	fromName string
	host     string
	region   string
	targets  []emailEntry
	cc       map[string]struct{}
	bcc      map[string]struct{}
	headers  map[string]string
	tokens   map[string]string
	batch    bool
	verify   bool
	disabled bool
}

func NewMailgunTarget(target *ParsedURL) (*MailgunTarget, error) {
	user := strings.TrimSpace(target.User)
	host := strings.TrimSpace(target.Host)

	pathEntries := splitPath(target.Path)
	apiKey := ""
	if len(pathEntries) > 0 {
		apiKey = strings.TrimSpace(pathEntries[0])
		pathEntries = pathEntries[1:]
	}
	if apiKey == "" {
		return &MailgunTarget{disabled: true}, nil
	}

	if user == "" || host == "" {
		return nil, fmt.Errorf("missing sender")
	}
	fromAddr := user + "@" + host
	if !isSimpleEmail(fromAddr) {
		return nil, fmt.Errorf("invalid sender")
	}

	fromName := mailgunDefaultName
	rawFrom := strings.TrimSpace(target.Query["from"])
	if rawFrom == "" {
		rawFrom = strings.TrimSpace(target.Query["name"])
	}
	if rawFrom != "" {
		if isSimpleEmail(rawFrom) {
			fromAddr = rawFrom
			fromName = ""
		} else {
			fromName = rawFrom
		}
	}

	region := strings.ToLower(strings.TrimSpace(target.Query["region"]))
	if region == "" {
		region = "us"
	}
	if _, ok := mailgunAPIBase[region]; !ok {
		return nil, fmt.Errorf("invalid region")
	}

	targets := []emailEntry{}
	for _, entry := range pathEntries {
		if parsed, ok := parseEmailEntry(entry); ok {
			targets = append(targets, parsed)
		}
	}
	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		for _, entry := range parseDelimitedList(toValue) {
			if parsed, ok := parseEmailEntry(entry); ok {
				targets = append(targets, parsed)
			}
		}
	}
	if len(targets) == 0 {
		targets = []emailEntry{{name: "", email: fromAddr}}
	}

	cc := map[string]struct{}{}
	if ccValue, ok := target.Query["cc"]; ok && ccValue != "" {
		for _, entry := range parseDelimitedList(ccValue) {
			if parsed, ok := parseEmailEntry(entry); ok {
				cc[parsed.email] = struct{}{}
			}
		}
	}

	bcc := map[string]struct{}{}
	if bccValue, ok := target.Query["bcc"]; ok && bccValue != "" {
		for _, entry := range parseDelimitedList(bccValue) {
			if parsed, ok := parseEmailEntry(entry); ok {
				bcc[parsed.email] = struct{}{}
			}
		}
	}

	headers := map[string]string{}
	for key, value := range target.QueryAdd {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		headers[key] = value
	}

	tokens := map[string]string{}
	for key, value := range target.QueryPayload {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		tokens[key] = value
	}

	return &MailgunTarget{
		apiKey:   apiKey,
		fromAddr: fromAddr,
		fromName: fromName,
		host:     host,
		region:   region,
		targets:  targets,
		cc:       cc,
		bcc:      bcc,
		headers:  headers,
		tokens:   tokens,
		batch:    parseBoolWithDefault(target.Query["batch"], false),
		verify:   parseBoolWithDefault(target.Query["verify"], true),
	}, nil
}

func (m *MailgunTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if m.disabled {
		return RequestSpec{}, fmt.Errorf("missing apikey")
	}
	if len(m.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	batchSize := 1
	if m.batch {
		batchSize = mailgunBatchSize
	}

	payload := m.buildPayload(body, title, m.targets[:minInt(len(m.targets), batchSize)])
	encoded := payload.Encode()

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    m.buildURL(),
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "application/json",
			"Content-Type": "application/x-www-form-urlencoded",
			"Authorization": basicAuthHeader(
				"api",
				m.apiKey,
			),
		},
		Body: encoded,
	}, nil
}

func (m *MailgunTarget) Send(body, title string, notifyType NotifyType) error {
	if m.disabled {
		return nil
	}
	if len(m.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	batchSize := 1
	if m.batch {
		batchSize = mailgunBatchSize
	}

	for index := 0; index < len(m.targets); index += batchSize {
		end := index + batchSize
		if end > len(m.targets) {
			end = len(m.targets)
		}

		payload := m.buildPayload(body, title, m.targets[index:end])
		spec := RequestSpec{
			Method: "POST",
			URL:    m.buildURL(),
			Headers: map[string]string{
				"User-Agent":   "Apprise",
				"Accept":       "application/json",
				"Content-Type": "application/x-www-form-urlencoded",
				"Authorization": basicAuthHeader(
					"api",
					m.apiKey,
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

func (m *MailgunTarget) buildPayload(body, title string, recipients []emailEntry) url.Values {
	values := url.Values{}
	if m.verify {
		values.Set("o:skip-verification", "False")
	} else {
		values.Set("o:skip-verification", "True")
	}
	values.Set("from", formatEmail(m.fromName, m.fromAddr))
	values.Set("subject", title)
	values.Set("html", body)

	toList := []string{}
	toEmails := map[string]struct{}{}
	for _, entry := range recipients {
		toList = append(toList, formatEmail(entry.name, entry.email))
		toEmails[entry.email] = struct{}{}
	}
	values.Set("to", strings.Join(toList, ","))

	cc := subtractEmailSet(m.cc, m.bcc, toEmails)
	if len(cc) > 0 {
		ccList := make([]string, 0, len(cc))
		for _, email := range cc {
			ccList = append(ccList, formatEmail("", email))
		}
		values.Set("cc", strings.Join(ccList, ","))
	}

	bcc := subtractEmailSet(m.bcc, nil, toEmails)
	if len(bcc) > 0 {
		values.Set("bcc", strings.Join(bcc, ","))
	}

	for key, value := range m.tokens {
		values.Set("v:"+key, value)
	}
	for key, value := range m.headers {
		values.Set("h:"+key, value)
	}

	return values
}

func (m *MailgunTarget) buildURL() string {
	return mailgunAPIBase[m.region] + m.host + "/messages"
}

func subtractEmailSet(source, remove map[string]struct{}, targets map[string]struct{}) []string {
	entries := []string{}
	for email := range source {
		if _, ok := targets[email]; ok {
			continue
		}
		if remove != nil {
			if _, ok := remove[email]; ok {
				continue
			}
		}
		entries = append(entries, email)
	}
	return entries
}

func init() {
	RegisterSchemaEntryOrdered(85, SchemaEntry{
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
					"alias_of": "name",
				},
				"name": map[string]any{
					"map_to":   "from_addr",
					"name":     "From Name",
					"private":  false,
					"required": false,
					"type":     "string",
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
				"region": map[string]any{
					"default":  "us",
					"map_to":   "region_name",
					"name":     "Region Name",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"us", "eu"},
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
			"kwargs": map[string]any{
				"headers": map[string]any{
					"map_to":   "headers",
					"name":     "Email Header",
					"prefix":   "+",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"tokens": map[string]any{
					"map_to":   "tokens",
					"name":     "Template Tokens",
					"prefix":   ":",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{user}@{host}:{apikey}/", "{schema}://{user}@{host}:{apikey}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
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
					"default":  "mailgun",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"mailgun"},
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
		"secure_protocols": []string{"mailgun"},
		"service_name":     "Mailgun",
		"service_url":      "https://www.mailgun.com/",
		"setup_url":        "https://appriseit.com/services/mailgun/",
	})
}
