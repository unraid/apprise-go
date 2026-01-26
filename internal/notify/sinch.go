package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

const sinchURLTemplate = "https://%s.sms.api.sinch.com/xms/v1/%s/batches"

var sinchRegions = map[string]struct{}{
	"us": {},
	"eu": {},
}

type SinchTarget struct {
	servicePlanID string
	apiToken      string
	source        string
	targets       []string
	region        string
}

func NewSinchTarget(target *ParsedURL) (*SinchTarget, error) {
	servicePlanID := strings.TrimSpace(target.User)
	apiToken := target.Password
	if raw := target.Query["spi"]; raw != "" {
		servicePlanID = raw
	}
	if raw := target.Query["token"]; raw != "" {
		apiToken = raw
	}
	if servicePlanID == "" || apiToken == "" {
		return nil, fmt.Errorf("missing credentials")
	}

	sourceRaw := strings.TrimSpace(target.Host)
	if raw := target.Query["from"]; raw != "" {
		sourceRaw = raw
	} else if raw := target.Query["source"]; raw != "" {
		sourceRaw = raw
	}
	sourceDigits, ok := normalizePhoneWithBounds(sourceRaw, 5, 14)
	if !ok {
		return nil, fmt.Errorf("invalid source")
	}

	source := sourceDigits
	if len(sourceDigits) >= 11 && len(sourceDigits) <= 14 {
		source = "+" + sourceDigits
	} else if len(sourceDigits) != 5 && len(sourceDigits) != 6 {
		return nil, fmt.Errorf("invalid source length")
	}

	region := strings.ToLower(strings.TrimSpace(target.Query["region"]))
	if region == "" {
		region = "us"
	}
	if _, ok := sinchRegions[region]; !ok {
		return nil, fmt.Errorf("invalid region")
	}

	targets := []string{}
	appendTarget := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if normalized, ok := normalizePhone(raw); ok {
			targets = append(targets, "+"+normalized)
		}
	}

	for _, entry := range splitPath(target.Path) {
		appendTarget(entry)
	}
	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		for _, entry := range parseDelimitedList(toValue) {
			appendTarget(entry)
		}
	}

	if len(targets) == 0 {
		if len(sourceDigits) >= 11 && len(sourceDigits) <= 14 {
			targets = append(targets, source)
		}
	}

	return &SinchTarget{
		servicePlanID: servicePlanID,
		apiToken:      apiToken,
		source:        source,
		targets:       targets,
		region:        region,
	}, nil
}

func (s *SinchTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(s.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	payload := map[string]any{
		"body": message,
		"from": s.source,
		"to":   []string{s.targets[0]},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	requestURL := fmt.Sprintf(sinchURLTemplate, s.region, s.servicePlanID)

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    requestURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Accept":        "*/*",
			"Authorization": fmt.Sprintf("Bearer %s", s.apiToken),
			"Content-Type":  "application/json",
		},
		Body: string(data),
	}, nil
}

func (s *SinchTarget) Send(body, title string, notifyType NotifyType) error {
	if len(s.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	requestURL := fmt.Sprintf(sinchURLTemplate, s.region, s.servicePlanID)

	for _, target := range s.targets {
		payload := map[string]any{
			"body": message,
			"from": s.source,
			"to":   []string{target},
		}

		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		spec := RequestSpec{
			Method: "POST",
			URL:    requestURL,
			Headers: map[string]string{
				"User-Agent":    "Apprise",
				"Accept":        "*/*",
				"Authorization": fmt.Sprintf("Bearer %s", s.apiToken),
				"Content-Type":  "application/json",
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

func init() {
	RegisterSchemaEntryOrdered(57, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
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
				"region": map[string]any{
					"default":  "us",
					"map_to":   "region",
					"name":     "Region",
					"private":  false,
					"regex":    []string{"^[a-z]{2}$", "i"},
					"required": false,
					"type":     "string",
				},
				"rto": map[string]any{
					"default":  4,
					"map_to":   "rto",
					"name":     "Socket Read Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"spi": map[string]any{
					"alias_of": "service_plan_id",
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
				"token": map[string]any{
					"alias_of": "api_token",
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
			"templates": []string{"{schema}://{service_plan_id}:{api_token}@{from_phone}", "{schema}://{service_plan_id}:{api_token}@{from_phone}/{targets}"},
			"tokens": map[string]any{
				"api_token": map[string]any{
					"map_to":   "api_token",
					"name":     "Auth Token",
					"private":  true,
					"regex":    []string{"^[a-f0-9]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"from_phone": map[string]any{
					"map_to":   "source",
					"name":     "From Phone No",
					"private":  false,
					"regex":    []string{"^\\+?[0-9\\s)(+-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "sinch",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"sinch"},
				},
				"service_plan_id": map[string]any{
					"map_to":   "service_plan_id",
					"name":     "Account SID",
					"private":  true,
					"regex":    []string{"^[a-f0-9]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"short_code": map[string]any{
					"map_to":   "targets",
					"name":     "Target Short Code",
					"private":  false,
					"regex":    []string{"^[0-9]{5,6}$", "i"},
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
					"group":    []string{"short_code", "target_phone"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
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
		"secure_protocols": []string{"sinch"},
		"service_name":     "Sinch",
		"service_url":      "https://sinch.com/",
		"setup_url":        "https://appriseit.com/services/sinch/",
	})
}
