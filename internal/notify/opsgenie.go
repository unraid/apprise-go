package notify

import (
	"encoding/json"
	"fmt"
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

type OpsgenieTarget struct {
	genieAlert
	region string
}

func NewOpsgenieTarget(target *ParsedURL) (*OpsgenieTarget, error) {
	alert, err := parseGenieAlert(target)
	if err != nil {
		return nil, err
	}

	region := strings.ToLower(strings.TrimSpace(target.Query["region"]))
	if region == "" {
		region = opsgenieRegionUS
	}
	if _, ok := opsgenieRegionURLs[region]; !ok {
		return nil, fmt.Errorf("invalid region: %s", region)
	}

	return &OpsgenieTarget{genieAlert: alert, region: region}, nil
}

func (o *OpsgenieTarget) Send(body, title string, notifyType NotifyType) error {
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

func (o *OpsgenieTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	specs, err := o.buildRequests(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}
	if len(specs) == 0 {
		return RequestSpec{}, fmt.Errorf("unsupported action: %s", o.resolveAction(notifyType))
	}

	return specs[0], nil
}

// buildRequests returns nothing for any action other than new. The other
// actions operate on alerts created by an earlier notification, which upstream
// looks up in its persistent store; with no stored request IDs it logs that
// there is nothing to act on and sends no request, which is what a Go install
// always sees.
func (o *OpsgenieTarget) buildRequests(body, title string, notifyType NotifyType) ([]RequestSpec, error) {
	if o.resolveAction(notifyType) != "new" {
		return nil, nil
	}

	url, ok := opsgenieRegionURLs[o.region]
	if !ok {
		return nil, fmt.Errorf("invalid region: %s", o.region)
	}

	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Content-Type":  "application/json",
		"Authorization": fmt.Sprintf("GenieKey %s", o.apiKey),
	}

	batches := o.responderBatches()
	specs := make([]RequestSpec, 0, len(batches))
	for _, responders := range batches {
		data, err := json.Marshal(o.buildPayload(body, title, notifyType, responders))
		if err != nil {
			return nil, err
		}

		specs = append(specs, RequestSpec{
			Method:  "POST",
			URL:     url,
			Headers: headers,
			Body:    string(data),
		})
	}

	return specs, nil
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
