package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	opsgenieRegionUS = "us"
	opsgenieRegionEU = "eu"
)

var opsgenieRegionURLs = map[string]string{
	opsgenieRegionUS: "https://api.opsgenie.com/v2/alerts",
	opsgenieRegionEU: "https://api.eu.opsgenie.com/v2/alerts",
}

var opsgenieActions = []string{
	"map",
	"new",
	"close",
	"delete",
	"acknowledge",
	"note",
}

var opsgeniePriorityMap = map[string]int{
	"l":  1,
	"m":  2,
	"n":  3,
	"h":  4,
	"e":  5,
	"1":  1,
	"2":  2,
	"3":  3,
	"4":  4,
	"5":  5,
	"p1": 1,
	"p2": 2,
	"p3": 3,
	"p4": 4,
	"p5": 5,
}

var opsgenieAlertMap = map[NotifyType]string{
	NotifyInfo:    "close",
	NotifySuccess: "close",
	NotifyWarning: "new",
	NotifyFailure: "new",
}

type OpsgenieTarget struct {
	apiKey    string
	region    string
	action    string
	priority  int
	details   map[string]string
	entity    string
	alias     string
	tags      []string
	targets   []map[string]string
	user      string
	batchSize int
}

func NewOpsgenieTarget(target *ParsedURL) (*OpsgenieTarget, error) {
	apiKey := strings.TrimSpace(target.Host)
	if apiKey == "" {
		return nil, fmt.Errorf("missing apikey")
	}

	region := strings.ToLower(strings.TrimSpace(target.Query["region"]))
	if region == "" {
		region = opsgenieRegionUS
	}
	if _, ok := opsgenieRegionURLs[region]; !ok {
		return nil, fmt.Errorf("invalid region: %s", region)
	}

	action, err := parseOpsgenieAction(target.Query["action"])
	if err != nil {
		return nil, err
	}

	priority := parseOpsgeniePriority(target.Query["priority"])

	details := map[string]string{}
	for key, value := range target.QueryAdd {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		details[key] = value
	}

	tags := []string{}
	if tagValue := strings.TrimSpace(target.Query["tags"]); tagValue != "" {
		tags = append(tags, parseDelimitedList(tagValue)...)
	}

	entries := splitPath(target.Path)
	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		entries = append(entries, parseDelimitedList(toValue)...)
	}

	targets := parseOpsgenieTargets(entries)
	batchSize := 1
	if parseBoolWithDefault(target.Query["batch"], false) {
		batchSize = 50
	}

	return &OpsgenieTarget{
		apiKey:    apiKey,
		region:    region,
		action:    action,
		priority:  priority,
		details:   details,
		entity:    strings.TrimSpace(target.Query["entity"]),
		alias:     strings.TrimSpace(target.Query["alias"]),
		tags:      tags,
		targets:   targets,
		user:      strings.TrimSpace(target.User),
		batchSize: batchSize,
	}, nil
}

func (o *OpsgenieTarget) Send(body, title string, notifyType NotifyType) error {
	action := o.resolveAction(notifyType)
	if action != "new" {
		return nil
	}

	if len(o.targets) == 0 {
		spec, err := o.BuildRequest(body, title, notifyType)
		if err != nil {
			return err
		}
		return SendRequest(spec)
	}

	for start := 0; start < len(o.targets); start += o.batchSize {
		end := start + o.batchSize
		if end > len(o.targets) {
			end = len(o.targets)
		}
		spec, err := o.buildRequestWithResponders(body, title, notifyType, o.targets[start:end])
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func (o *OpsgenieTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	action := o.resolveAction(notifyType)
	if action != "new" {
		return RequestSpec{}, fmt.Errorf("unsupported action: %s", action)
	}

	responders := []map[string]string{}
	if len(o.targets) > 0 {
		limit := o.batchSize
		if limit <= 0 || limit > len(o.targets) {
			limit = len(o.targets)
		}
		responders = o.targets[:limit]
	}

	return o.buildRequestWithResponders(body, title, notifyType, responders)
}

func (o *OpsgenieTarget) buildRequestWithResponders(body, title string, notifyType NotifyType, responders []map[string]string) (RequestSpec, error) {
	payload := o.buildPayload(body, title, notifyType, responders)
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	url, ok := opsgenieRegionURLs[o.region]
	if !ok {
		return RequestSpec{}, fmt.Errorf("invalid region: %s", o.region)
	}

	return RequestSpec{
		Method: "POST",
		URL:    url,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/json",
			"Authorization": fmt.Sprintf(
				"GenieKey %s",
				o.apiKey,
			),
		},
		Body: string(data),
	}, nil
}

func (o *OpsgenieTarget) resolveAction(notifyType NotifyType) string {
	if o.action == "map" {
		return opsgenieAlertMap[notifyType]
	}
	return o.action
}

func (o *OpsgenieTarget) buildPayload(body, title string, notifyType NotifyType, responders []map[string]string) map[string]any {
	details := map[string]string{}
	for key, value := range o.details {
		details[key] = value
	}
	if _, ok := details["type"]; !ok {
		details["type"] = string(notifyType)
	}

	message := title
	if strings.TrimSpace(message) == "" {
		message = body
	}

	if len(message) > 130 {
		message = message[:127] + "..."
	}

	payload := map[string]any{
		"source":      "Apprise Notifications",
		"message":     message,
		"description": body,
		"details":     details,
		"priority":    fmt.Sprintf("P%d", o.priority),
	}

	if len(o.tags) > 0 {
		payload["tags"] = o.tags
	}
	if o.entity != "" {
		payload["entity"] = o.entity
	}
	if o.alias != "" {
		payload["alias"] = o.alias
	}
	if o.user != "" {
		payload["user"] = o.user
	}
	if len(responders) > 0 {
		payload["responders"] = responders
	}

	return payload
}

func parseOpsgenieAction(raw string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return "map", nil
	}
	for _, candidate := range opsgenieActions {
		if strings.HasPrefix(candidate, normalized) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("invalid action: %s", raw)
}

func parseOpsgeniePriority(raw string) int {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return 3
	}
	for key, value := range opsgeniePriorityMap {
		if strings.HasPrefix(normalized, key) {
			return value
		}
	}
	return 3
}

func parseOpsgenieTargets(entries []string) []map[string]string {
	normalized := normalizeOpsgenieEntries(entries)

	targets := []map[string]string{}
	for _, entry := range normalized {
		target := strings.TrimSpace(entry)
		if len(target) < 2 {
			continue
		}

		prefix := target[:1]
		value := target
		switch prefix {
		case "@", "#", "*", "^":
			value = strings.TrimSpace(target[1:])
		default:
			prefix = "@"
		}

		if value == "" {
			continue
		}

		switch prefix {
		case "#":
			if isUUID(value) {
				targets = append(targets, map[string]string{
					"type": "team",
					"id":   value,
				})
			} else {
				targets = append(targets, map[string]string{
					"type": "team",
					"name": value,
				})
			}
		case "*":
			if isUUID(value) {
				targets = append(targets, map[string]string{
					"type": "schedule",
					"id":   value,
				})
			} else {
				targets = append(targets, map[string]string{
					"type": "schedule",
					"name": value,
				})
			}
		case "^":
			if isUUID(value) {
				targets = append(targets, map[string]string{
					"type": "escalation",
					"id":   value,
				})
			} else {
				targets = append(targets, map[string]string{
					"type": "escalation",
					"name": value,
				})
			}
		default:
			if isUUID(value) {
				targets = append(targets, map[string]string{
					"type": "user",
					"id":   value,
				})
			} else {
				targets = append(targets, map[string]string{
					"type":     "user",
					"username": value,
				})
			}
		}
	}

	return targets
}

func normalizeOpsgenieEntries(entries []string) []string {
	unique := map[string]struct{}{}
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		unique[trimmed] = struct{}{}
	}

	normalized := make([]string, 0, len(unique))
	for entry := range unique {
		normalized = append(normalized, entry)
	}
	sort.Strings(normalized)
	return normalized
}

var uuidPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func isUUID(value string) bool {
	return uuidPattern.MatchString(strings.ToLower(value))
}

func init() {
	RegisterSchemaEntryOrdered(25, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"action": map[string]any{
					"default":  "map",
					"map_to":   "action",
					"name":     "Action",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"map", "new", "close", "delete", "acknowledge", "note"},
				},
				"alias": map[string]any{
					"map_to":   "alias",
					"name":     "Alias",
					"private":  false,
					"required": false,
					"type":     "string",
				},
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
				"entity": map[string]any{
					"map_to":   "entity",
					"name":     "Entity",
					"private":  false,
					"required": false,
					"type":     "string",
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
					"default":  3,
					"map_to":   "priority",
					"name":     "Priority",
					"private":  false,
					"required": false,
					"type":     "choice:int",
					"values":   []any{1, 2, 3, 4, 5},
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
				"tags": map[string]any{
					"map_to":   "tags",
					"name":     "Tags",
					"private":  false,
					"required": false,
					"type":     "string",
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
				"details": map[string]any{
					"map_to":   "details",
					"name":     "Details",
					"prefix":   "+",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"mapping": map[string]any{
					"map_to":   "mapping",
					"name":     "Action Mapping",
					"prefix":   ":",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{apikey}", "{schema}://{user}@{apikey}", "{schema}://{apikey}/{targets}", "{schema}://{user}@{apikey}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "opsgenie",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"opsgenie"},
				},
				"target_escalation": map[string]any{
					"map_to":   "targets",
					"name":     "Target Escalation",
					"prefix":   "^",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_schedule": map[string]any{
					"map_to":   "targets",
					"name":     "Target Schedule",
					"prefix":   "*",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_team": map[string]any{
					"map_to":   "targets",
					"name":     "Target Team",
					"prefix":   "#",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_user": map[string]any{
					"map_to":   "targets",
					"name":     "Target User",
					"prefix":   "@",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_escalation", "target_schedule", "target_team", "target_user"},
					"map_to":   "targets",
					"name":     "Targets ",
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
		"protocols": nil,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"opsgenie"},
		"service_name":     "Opsgenie",
		"service_url":      "https://opsgenie.com/",
		"setup_url":        "https://appriseit.com/services/opsgenie/",
	})
}
