package notify

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const streamlabsBaseURL = "https://streamlabs.com/api/v1.0/"
const streamlabsCallAlerts = "ALERTS"
const streamlabsCallDonations = "DONATIONS"

var streamlabsAlertTypes = map[string]struct{}{
	"follow":       {},
	"subscription": {},
	"donation":     {},
	"host":         {},
}

type StreamlabsTarget struct {
	accessToken      string
	call             string
	alertType        string
	imageHref        string
	soundHref        string
	duration         int
	specialTextColor string
	amount           int
	currency         string
	name             string
	identifier       string
}

func NewStreamlabsTarget(target *ParsedURL) (*StreamlabsTarget, error) {
	accessToken := strings.TrimSpace(target.Host)
	if accessToken == "" {
		return nil, fmt.Errorf("missing access token")
	}

	call := normalizeStreamlabsCall(target.Query["call"])
	alertType := normalizeStreamlabsAlertType(target.Query["alert_type"])

	duration := parseIntWithDefault(target.Query["duration"], 1000)
	amount := parseIntWithDefault(target.Query["amount"], 0)

	currency := strings.ToUpper(strings.TrimSpace(target.Query["currency"]))
	if currency == "" {
		currency = "USD"
	}
	if len(currency) != 3 {
		currency = "USD"
	}

	name := strings.ToUpper(strings.TrimSpace(target.Query["name"]))
	if name == "" {
		name = "Anon"
	}

	identifier := strings.ToUpper(strings.TrimSpace(target.Query["identifier"]))
	if identifier == "" {
		identifier = "Apprise"
	}

	return &StreamlabsTarget{
		accessToken:      accessToken,
		call:             call,
		alertType:        alertType,
		imageHref:        target.Query["image_href"],
		soundHref:        strings.ToUpper(strings.TrimSpace(target.Query["sound_href"])),
		duration:         duration,
		specialTextColor: strings.ToUpper(strings.TrimSpace(target.Query["special_text_color"])),
		amount:           amount,
		currency:         currency,
		name:             name,
		identifier:       identifier,
	}, nil
}

func (s *StreamlabsTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	values := url.Values{}
	requestURL := streamlabsBaseURL + strings.ToLower(s.call)

	if s.call == streamlabsCallDonations {
		values.Set("name", s.name)
		values.Set("identifier", s.identifier)
		values.Set("amount", strconv.Itoa(s.amount))
		values.Set("currency", s.currency)
		values.Set("access_token", s.accessToken)
		values.Set("message", body)
	} else {
		values.Set("access_token", s.accessToken)
		values.Set("type", s.alertType)
		values.Set("image_href", s.imageHref)
		values.Set("sound_href", s.soundHref)
		values.Set("message", title)
		values.Set("user_massage", body)
		values.Set("duration", strconv.Itoa(s.duration))
		values.Set("special_text_color", s.specialTextColor)
	}

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    requestURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/x-www-form-urlencoded",
		},
		Body: values.Encode(),
	}, nil
}

func (s *StreamlabsTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := s.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func normalizeStreamlabsCall(raw string) string {
	value := strings.ToUpper(strings.TrimSpace(raw))
	switch value {
	case "ALERT", "ALERTS":
		return streamlabsCallAlerts
	case "DONATION", "DONATIONS":
		return streamlabsCallDonations
	default:
		return streamlabsCallAlerts
	}
}

func normalizeStreamlabsAlertType(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := streamlabsAlertTypes[value]; ok {
		return value
	}
	return "donation"
}

func parseIntWithDefault(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func init() {
	RegisterSchemaEntryOrdered(79, SchemaEntry{
		"service_name":       "Streamlabs",
		"service_url":        "https://streamlabs.com/",
		"setup_url":          "https://appriseit.com/services/streamlabs/",
		"attachment_support": false,
		"category":           "native",
		"enabled":            true,
		"protocols":          []string(nil),
		"secure_protocols":   []string{"strmlabs"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []string{},
			"packages_required":    []string{},
		},
		"details": map[string]any{
			"args": map[string]any{
				"alert_type": map[string]any{
					"default":  "donation",
					"map_to":   "alert_type",
					"name":     "Alert Type",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"follow", "subscription", "donation", "host"},
				},
				"amount": map[string]any{
					"default":  0,
					"map_to":   "amount",
					"min":      0,
					"name":     "Amount",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"call": map[string]any{
					"default":  "ALERTS",
					"map_to":   "call",
					"name":     "Call",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"ALERTS", "DONATIONS"},
				},
				"cto": map[string]any{
					"default":  4.0,
					"map_to":   "cto",
					"name":     "Socket Connect Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"currency": map[string]any{
					"default":  "USD",
					"map_to":   "currency",
					"name":     "Currency",
					"private":  false,
					"regex":    []string{"^[A-Z]{3}$", "i"},
					"required": false,
					"type":     "string",
				},
				"duration": map[string]any{
					"default":  1000,
					"map_to":   "duration",
					"min":      0,
					"name":     "Duration",
					"private":  false,
					"required": false,
					"type":     "int",
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
				"identifier": map[string]any{
					"default":  "Apprise",
					"map_to":   "identifier",
					"name":     "Identifier",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"image_href": map[string]any{
					"default":  "",
					"map_to":   "image_href",
					"name":     "Image Link",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"name": map[string]any{
					"default":  "Anon",
					"map_to":   "name",
					"name":     "Name",
					"private":  false,
					"regex":    []string{"^[^\\s].{1,24}$", "i"},
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
					"default":  4.0,
					"map_to":   "rto",
					"name":     "Socket Read Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"sound_href": map[string]any{
					"default":  "",
					"map_to":   "sound_href",
					"name":     "Sound Link",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"special_text_color": map[string]any{
					"default":  "",
					"map_to":   "special_text_color",
					"name":     "Special Text Color",
					"private":  false,
					"regex":    []string{"^[A-Z]$", "i"},
					"required": false,
					"type":     "string",
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
			"kwargs": map[string]any{},
			"templates": []string{
				"{schema}://{access_token}/",
			},
			"tokens": map[string]any{
				"access_token": map[string]any{
					"map_to":   "access_token",
					"name":     "Access Token",
					"private":  true,
					"regex":    []string{"^[a-z0-9]{40}$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "strmlabs",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"strmlabs"},
				},
			},
		},
	})
}
