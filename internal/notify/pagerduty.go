package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	pagerDutyRegionUS = "us"
	pagerDutyRegionEU = "eu"
)

var pagerDutyRegionURLs = map[string]string{
	pagerDutyRegionUS: "https://events.pagerduty.com/v2/enqueue",
	pagerDutyRegionEU: "https://events.eu.pagerduty.com/v2/enqueue",
}

var pagerDutySeverityMap = map[NotifyType]string{
	NotifyInfo:    "info",
	NotifySuccess: "info",
	NotifyWarning: "warning",
	NotifyFailure: "critical",
}

var pagerDutySeverities = []string{
	"info",
	"warning",
	"critical",
	"error",
}

type PagerDutyTarget struct {
	apiKey         string
	integrationKey string
	source         string
	component      string
	group          string
	classID        string
	click          string
	region         string
	severity       string
	includeImage   bool
	details        map[string]string
}

func NewPagerDutyTarget(target *ParsedURL) (*PagerDutyTarget, error) {
	apiKey := strings.TrimSpace(target.Query["apikey"])
	if apiKey == "" {
		apiKey = strings.TrimSpace(target.Host)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("missing apikey")
	}

	integrationKey := strings.TrimSpace(target.Query["integrationkey"])
	if integrationKey == "" {
		integrationKey = strings.TrimSpace(target.User)
	}
	if integrationKey == "" {
		return nil, fmt.Errorf("missing integration key")
	}

	region := strings.ToLower(strings.TrimSpace(target.Query["region"]))
	if region == "" {
		region = pagerDutyRegionUS
	}
	if _, ok := pagerDutyRegionURLs[region]; !ok {
		return nil, fmt.Errorf("invalid region: %s", region)
	}

	severity := ""
	if rawSeverity := strings.ToLower(strings.TrimSpace(target.Query["severity"])); rawSeverity != "" {
		parsed := ""
		for _, candidate := range pagerDutySeverities {
			if strings.HasPrefix(candidate, rawSeverity) {
				parsed = candidate
				break
			}
		}
		if parsed == "" {
			return nil, fmt.Errorf("invalid severity: %s", rawSeverity)
		}
		severity = parsed
	}

	source := strings.TrimSpace(target.Query["source"])
	component := strings.TrimSpace(target.Query["component"])
	segments := splitPath(target.Path)
	if source == "" && len(segments) > 0 {
		source = strings.TrimSpace(segments[0])
	}
	if component == "" && len(segments) > 1 {
		component = strings.TrimSpace(segments[1])
	}
	if source == "" {
		source = "Apprise"
	}
	if component == "" {
		component = "Notification"
	}

	details := map[string]string{}
	for key, value := range target.QueryAdd {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		details[key] = value
	}

	return &PagerDutyTarget{
		apiKey:         apiKey,
		integrationKey: integrationKey,
		source:         source,
		component:      component,
		group:          strings.TrimSpace(target.Query["group"]),
		classID:        strings.TrimSpace(target.Query["class"]),
		click:          strings.TrimSpace(target.Query["click"]),
		region:         region,
		severity:       severity,
		includeImage:   parseBoolWithDefault(target.Query["image"], true),
		details:        details,
	}, nil
}

func (p *PagerDutyTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := p.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}
	return SendRequest(spec)
}

func (p *PagerDutyTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if p.apiKey == "" || p.integrationKey == "" {
		return RequestSpec{}, fmt.Errorf("missing pagerduty credentials")
	}

	severity := p.severity
	if severity == "" {
		severity = pagerDutySeverityMap[notifyType]
	}

	summary := mergeTitleBody(title, body)
	if summary == "" {
		summary = body
	}

	payload := map[string]any{
		"routing_key": p.integrationKey,
		"payload": map[string]any{
			"summary":   summary,
			"severity":  severity,
			"source":    p.source,
			"component": p.component,
		},
		"client":       "Apprise",
		"event_action": "trigger",
	}

	if p.group != "" {
		payload["payload"].(map[string]any)["group"] = p.group
	}
	if p.classID != "" {
		payload["payload"].(map[string]any)["class"] = p.classID
	}
	if len(p.details) > 0 {
		customDetails := map[string]string{}
		for key, value := range p.details {
			customDetails[key] = value
		}
		payload["payload"].(map[string]any)["custom_details"] = customDetails
	}
	if p.click != "" {
		payload["links"] = []map[string]string{{
			"href": p.click,
		}}
	}
	if p.includeImage {
		payload["images"] = []map[string]string{{
			"src": appriseImageURL(notifyType, "128x128"),
			"alt": string(notifyType),
		}}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	url, ok := pagerDutyRegionURLs[p.region]
	if !ok {
		return RequestSpec{}, fmt.Errorf("invalid region: %s", p.region)
	}

	return RequestSpec{
		Method: "POST",
		URL:    url,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/json",
			"Authorization": fmt.Sprintf(
				"Token token=%s",
				p.apiKey,
			),
		},
		Body: string(data),
	}, nil
}

func init() {
	RegisterSchemaEntryOrdered(103, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"class": map[string]any{
					"map_to":   "class_id",
					"name":     "Class",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"click": map[string]any{
					"map_to":   "click",
					"name":     "Click",
					"private":  false,
					"required": false,
					"type":     "string",
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
				"group": map[string]any{
					"map_to":   "group",
					"name":     "Group",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"image": map[string]any{
					"default":  true,
					"map_to":   "include_image",
					"name":     "Include Image",
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
				"severity": map[string]any{
					"map_to":   "severity",
					"name":     "Severity",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"info", "warning", "critical", "error"},
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
			"kwargs": map[string]any{
				"details": map[string]any{
					"map_to":   "details",
					"name":     "Custom Details",
					"prefix":   "+",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{integrationkey}@{apikey}", "{schema}://{integrationkey}@{apikey}/{source}", "{schema}://{integrationkey}@{apikey}/{source}/{component}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"component": map[string]any{
					"default":  "Notification",
					"map_to":   "component",
					"name":     "Component",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"integrationkey": map[string]any{
					"map_to":   "integrationkey",
					"name":     "Integration Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "pagerduty",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"pagerduty"},
				},
				"source": map[string]any{
					"default":  "Apprise",
					"map_to":   "source",
					"name":     "Source",
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
		"secure_protocols": []string{"pagerduty"},
		"service_name":     "Pager Duty",
		"service_url":      "https://pagerduty.com/",
		"setup_url":        "https://appriseit.com/services/pagerduty/",
	})
}
