package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const bulkVSURL = "https://portal.bulkvs.com/api/v1.0/messageSend"
const bulkVSBatchSize = 4000

type BulkVSTarget struct {
	user     string
	password string
	source   string
	targets  []string
	batch    bool
}

func NewBulkVSTarget(target *ParsedURL) (*BulkVSTarget, error) {
	user := strings.TrimSpace(target.User)
	password := target.Password
	if user == "" || password == "" {
		return nil, fmt.Errorf("missing credentials")
	}

	sourceRaw := ""
	targets := []string{}
	hasInvalid := false

	appendTarget := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if normalized, ok := normalizePhone(raw); ok {
			targets = append(targets, normalized)
			return
		}
		hasInvalid = true
	}

	if fromValue, ok := target.Query["from"]; ok && fromValue != "" {
		sourceRaw = fromValue
		appendTarget(target.Host)
		for _, entry := range splitPath(target.Path) {
			appendTarget(entry)
		}
	} else {
		sourceRaw = target.Host
		for _, entry := range splitPath(target.Path) {
			appendTarget(entry)
		}
	}

	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		for _, entry := range parseDelimitedList(toValue) {
			appendTarget(entry)
		}
	}

	source, ok := normalizePhone(sourceRaw)
	if !ok {
		return nil, fmt.Errorf("invalid source")
	}

	if len(targets) == 0 && !hasInvalid {
		targets = append(targets, source)
	}

	batch := parseBool(target.Query["batch"], false)

	return &BulkVSTarget{
		user:     user,
		password: password,
		source:   source,
		targets:  targets,
		batch:    batch,
	}, nil
}

func (b *BulkVSTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(b.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	payload := map[string]any{
		"From":    b.source,
		"Message": message,
	}
	if b.batch {
		payload["To"] = b.targets[:minInt(len(b.targets), bulkVSBatchSize)]
	} else {
		payload["To"] = b.targets[0]
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    bulkVSURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Accept":        "application/json",
			"Content-Type":  "application/json",
			"Authorization": basicAuthHeader(b.user, b.password),
		},
		Body: string(data),
	}, nil
}

func (b *BulkVSTarget) Send(body, title string, notifyType NotifyType) error {
	if len(b.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	batchSize := 1
	if b.batch {
		batchSize = bulkVSBatchSize
	}

	for index := 0; index < len(b.targets); index += batchSize {
		end := index + batchSize
		if end > len(b.targets) {
			end = len(b.targets)
		}
		payload := map[string]any{
			"From":    b.source,
			"Message": message,
		}
		if b.batch {
			payload["To"] = b.targets[index:end]
		} else {
			payload["To"] = b.targets[index]
		}

		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		spec := RequestSpec{
			Method: "POST",
			URL:    bulkVSURL,
			Headers: map[string]string{
				"User-Agent":    "Apprise",
				"Accept":        "application/json",
				"Content-Type":  "application/json",
				"Authorization": basicAuthHeader(b.user, b.password),
			},
			Body: string(data),
		}

		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = notifyType

	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func init() {
	RegisterSchemaEntryOrdered(40, SchemaEntry{
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
					"map_to":   "source",
					"name":     "From Phone No",
					"private":  false,
					"regex":    []string{"^\\+?[0-9\\s)(+-]+$", "i"},
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
			"templates": []string{"{schema}://{user}:{password}@{from_phone}/{targets}", "{schema}://{user}:{password}@{from_phone}"},
			"tokens": map[string]any{
				"from_phone": map[string]any{
					"map_to":   "source",
					"name":     "From Phone No",
					"private":  false,
					"regex":    []string{"^\\+?[0-9\\s)(+-]+$", "i"},
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
					"default":  "bulkvs",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"bulkvs"},
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
		"secure_protocols": []string{"bulkvs"},
		"service_name":     "BulkVS",
		"service_url":      "https://www.bulkvs.com/",
		"setup_url":        "https://appriseit.com/services/bulkvs/",
	})
}
