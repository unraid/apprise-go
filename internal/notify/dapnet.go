package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const dapnetURL = "http://www.hampager.de:8080/calls"
const dapnetBatchSize = 50

const (
	dapnetPriorityNormal = iota
	dapnetPriorityEmergency
)

var dapnetCallSignRe = regexp.MustCompile(`(?i)^([0-9a-z]{1,2}[0-9][a-z0-9]{1,3})(?:-([0-9]{1,2}))?\s*$`)

type DapnetTarget struct {
	user     string
	password string
	priority int
	txgroups []string
	batch    bool
	targets  []string
}

type dapnetPayload struct {
	Text                  string   `json:"text"`
	CallSignNames         []string `json:"callSignNames"`
	TransmitterGroupNames []string `json:"transmitterGroupNames"`
	Emergency             bool     `json:"emergency"`
}

func NewDapnetTarget(target *ParsedURL) (*DapnetTarget, error) {
	user := strings.TrimSpace(target.User)
	password := target.Password
	if user == "" || password == "" {
		return nil, fmt.Errorf("missing credentials")
	}

	priority := parseDapnetPriority(target.Query["priority"])
	txgroups := parseDapnetTxGroups(target.Query["txgroups"])
	batch := parseBoolWithDefault(target.Query["batch"], false)

	entries := []string{}
	if target.Host != "" {
		entries = append(entries, target.Host)
	}
	entries = append(entries, splitPath(target.Path)...)
	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		entries = append(entries, parseDelimitedList(toValue)...)
	}

	targets := []string{}
	seen := map[string]struct{}{}
	for _, entry := range entries {
		callSign, ok := normalizeDapnetCallSign(entry)
		if !ok {
			continue
		}
		if _, exists := seen[callSign]; exists {
			continue
		}
		seen[callSign] = struct{}{}
		targets = append(targets, callSign)
	}

	return &DapnetTarget{
		user:     user,
		password: password,
		priority: priority,
		txgroups: txgroups,
		batch:    batch,
		targets:  targets,
	}, nil
}

func (d *DapnetTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(d.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	batchSize := 1
	if d.batch {
		batchSize = dapnetBatchSize
	}
	chunk := d.targets
	if len(chunk) > batchSize {
		chunk = chunk[:batchSize]
	}

	spec, err := d.buildRequest(chunk, message)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return spec, nil
}

func (d *DapnetTarget) Send(body, title string, notifyType NotifyType) error {
	if len(d.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	batchSize := 1
	if d.batch {
		batchSize = dapnetBatchSize
	}

	for idx := 0; idx < len(d.targets); idx += batchSize {
		end := idx + batchSize
		if end > len(d.targets) {
			end = len(d.targets)
		}
		spec, err := d.buildRequest(d.targets[idx:end], message)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = notifyType

	return nil
}

func (d *DapnetTarget) buildRequest(targets []string, message string) (RequestSpec, error) {
	payload := dapnetPayload{
		Text:                  message,
		CallSignNames:         targets,
		TransmitterGroupNames: d.txgroups,
		Emergency:             d.priority == dapnetPriorityEmergency,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    dapnetURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Content-Type":  "application/json; charset=utf-8",
			"Authorization": basicAuthHeader(d.user, d.password),
		},
		Body: string(data),
	}, nil
}

func parseDapnetPriority(raw string) int {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return dapnetPriorityNormal
	}
	priorityMap := []struct {
		prefix string
		value  int
	}{
		{"n", dapnetPriorityNormal},
		{"e", dapnetPriorityEmergency},
		{"0", dapnetPriorityNormal},
		{"1", dapnetPriorityEmergency},
	}
	for _, entry := range priorityMap {
		if strings.HasPrefix(raw, entry.prefix) {
			return entry.value
		}
	}
	return dapnetPriorityNormal
}

func parseDapnetTxGroups(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{"dl-all"}
	}

	entries := parseDelimitedList(raw)
	if len(entries) == 0 {
		return []string{"dl-all"}
	}

	groups := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		groups = append(groups, strings.ToLower(entry))
	}
	if len(groups) == 0 {
		return []string{"dl-all"}
	}
	return groups
}

func normalizeDapnetCallSign(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	match := dapnetCallSignRe.FindStringSubmatch(raw)
	if match == nil {
		return "", false
	}
	return strings.ToUpper(match[1]), true
}

func init() {
	RegisterSchemaEntryOrdered(117, SchemaEntry{
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
				"overflow": map[string]any{
					"default":  "upstream",
					"map_to":   "overflow",
					"name":     "Overflow Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"split", "truncate", "upstream"},
				},
				"priority": map[string]any{
					"default":  0,
					"map_to":   "priority",
					"name":     "Priority",
					"private":  false,
					"required": false,
					"type":     "choice:int",
					"values":   []any{0, 1},
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
					"map_to":   "targets",
					"name":     "Target Callsign",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"txgroups": map[string]any{
					"default":  "dl-all",
					"map_to":   "txgroups",
					"name":     "Transmitter Groups",
					"private":  true,
					"required": false,
					"type":     "string",
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
			"templates": []string{"{schema}://{user}:{password}@{targets}"},
			"tokens": map[string]any{
				"password": map[string]any{
					"map_to":   "password",
					"name":     "Password",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "dapnet",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"dapnet"},
				},
				"target_callsign": map[string]any{
					"map_to":   "targets",
					"name":     "Target Callsign",
					"private":  false,
					"regex":    []string{"^[a-z0-9]{2,5}(-[a-z0-9]{1,2})?$", "i"},
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_callsign"},
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
		"secure_protocols": []string{"dapnet"},
		"service_name":     "Dapnet",
		"service_url":      "https://hampager.de/",
		"setup_url":        "https://appriseit.com/services/dapnet/",
	})
}
