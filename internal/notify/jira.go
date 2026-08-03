package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Jira Service Management keeps Opsgenie's us/eu region argument so that an
// existing Opsgenie URL transfers unchanged, but both regions resolve to the
// same endpoint.
const jiraAlertURL = "https://api.atlassian.com/jsm/ops/integration/v2/alerts"

var jiraRegions = map[string]struct{}{
	"us": {},
	"eu": {},
}

type JiraTarget struct {
	genieAlert
	region string
}

func NewJiraTarget(target *ParsedURL) (*JiraTarget, error) {
	alert, err := parseGenieAlert(target)
	if err != nil {
		return nil, err
	}

	region := strings.ToLower(strings.TrimSpace(target.Query["region"]))
	if region == "" {
		region = "us"
	}
	if _, ok := jiraRegions[region]; !ok {
		return nil, fmt.Errorf("invalid region: %s", region)
	}

	return &JiraTarget{genieAlert: alert, region: region}, nil
}

func (j *JiraTarget) Send(body, title string, notifyType NotifyType) error {
	specs, err := j.buildRequests(body, title, notifyType)
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

func (j *JiraTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	specs, err := j.buildRequests(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}
	if len(specs) == 0 {
		return RequestSpec{}, fmt.Errorf("unsupported action: %s", j.resolveAction(notifyType))
	}

	return specs[0], nil
}

// buildRequests returns nothing for any action other than new, for the same
// reason Opsgenie does: the other actions act on an alert an earlier
// notification created, which upstream looks up in its persistent store.
func (j *JiraTarget) buildRequests(body, title string, notifyType NotifyType) ([]RequestSpec, error) {
	if j.resolveAction(notifyType) != "new" {
		return nil, nil
	}

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Content-Type": "application/json",
		// Upstream sends "Accepts", not "Accept". Copying the typo is the
		// point: this has to match what the service actually receives.
		"Accepts":       "application/json",
		"Authorization": fmt.Sprintf("GenieKey %s", j.apiKey),
	}

	batches := j.responderBatches()
	specs := make([]RequestSpec, 0, len(batches))
	for _, responders := range batches {
		data, err := json.Marshal(j.buildPayload(body, title, notifyType, responders))
		if err != nil {
			return nil, err
		}

		specs = append(specs, RequestSpec{
			Method:  "POST",
			URL:     jiraAlertURL,
			Headers: headers,
			Body:    string(data),
		})
	}

	return specs, nil
}
func init() {
	RegisterSchemaEntryOrdered(160, SchemaEntry{
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
				"priority": map[string]any{
					"default":  3,
					"map_to":   "priority",
					"name":     "Priority",
					"private":  false,
					"required": false,
					"type":     "choice:int",
					"values":   []any{1, 2, 3, 4, 5},
				},
				"redirect": map[string]any{
					"default":  true,
					"map_to":   "redirect",
					"name":     "Follow Redirects",
					"private":  false,
					"required": false,
					"type":     "bool",
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
					"default":  "jira",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"jira"},
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
		"enabled": true,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"jira"},
		"service_name":     "Jira Service Management",
		"service_url":      "https://atlassian.com/",
		"setup_url":        "https://github.com/caronc/apprise/wiki/Notify_jira",
	})
}
