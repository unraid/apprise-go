package notify

import (
	"fmt"
	"net/url"
	"strings"
)

// Exotel serves each region from its own host.
var exotelRegionURLs = map[string]string{
	"us": "https://api.exotel.com/v1/Accounts/%s/Sms/send",
	"in": "https://api.in.exotel.com/v1/Accounts/%s/Sms/send",
}

type ExotelTarget struct {
	sid      string
	apikey   string
	token    string
	source   string
	targets  []string
	region   string
	priority string
	unicode  bool
}

func NewExotelTarget(target *ParsedURL) (*ExotelTarget, error) {
	sid := strings.TrimSpace(target.User)
	if sid == "" {
		return nil, fmt.Errorf("missing account sid")
	}

	token := strings.TrimSpace(target.Password)
	if raw := strings.TrimSpace(target.Query["token"]); raw != "" {
		token = raw
	}
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	// The API key defaults to the account SID.
	apikey := strings.TrimSpace(target.Query["apikey"])
	if apikey == "" {
		apikey = sid
	}

	source := strings.TrimSpace(target.Host)
	if source == "" {
		return nil, fmt.Errorf("missing source phone number")
	}

	entries := splitPath(target.Path)
	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		entries = append(entries, parseDelimitedList(to)...)
	}

	targets := make([]string, 0, len(entries))
	for _, entry := range entries {
		if normalized, ok := normalizePhone(entry); ok {
			targets = append(targets, normalized)
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	region := strings.ToLower(strings.TrimSpace(target.Query["region"]))
	if _, ok := exotelRegionURLs[region]; !ok {
		region = "us"
	}

	priority := "normal"
	switch strings.ToLower(strings.TrimSpace(target.Query["priority"])) {
	case "high", "+":
		priority = "high"
	}

	return &ExotelTarget{
		sid:      sid,
		apikey:   apikey,
		token:    token,
		source:   source,
		targets:  targets,
		region:   region,
		priority: priority,
		unicode:  parseBoolWithDefault(target.Query["unicode"], true),
	}, nil
}

func (e *ExotelTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	specs, err := e.buildRequests(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}

	return specs[0], nil
}

func (e *ExotelTarget) Send(body, title string, notifyType NotifyType) error {
	specs, err := e.buildRequests(body, title, notifyType)
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

// buildRequests sends one message per recipient.
func (e *ExotelTarget) buildRequests(body, title string, notifyType NotifyType) ([]RequestSpec, error) {
	_ = notifyType

	encoding := "plain"
	if e.unicode {
		encoding = "unicode"
	}

	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Accept":        "*/*",
		"Content-Type":  "application/x-www-form-urlencoded",
		"Authorization": basicAuthHeader(e.apikey, e.token),
	}

	endpoint := fmt.Sprintf(exotelRegionURLs[e.region], e.sid)

	specs := make([]RequestSpec, 0, len(e.targets))
	for _, recipient := range e.targets {
		values := url.Values{}
		values.Set("From", e.source)
		values.Set("Body", mergeTitleBody(title, body))
		values.Set("EncodingType", encoding)
		values.Set("Priority", e.priority)
		values.Set("To", recipient)

		specs = append(specs, RequestSpec{
			Method:  "POST",
			URL:     endpoint,
			Headers: headers,
			Body:    values.Encode(),
		})
	}

	return specs, nil
}

func init() {
	RegisterSchemaEntryOrdered(154, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
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
				"key": map[string]any{
					"alias_of": "apikey",
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
					"default":  "normal",
					"map_to":   "priority",
					"name":     "Priority",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"normal", "high"},
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
					"values":   []string{"us", "in"},
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
				"sid": map[string]any{
					"alias_of": "sid",
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
					"alias_of": "token",
				},
				"tz": map[string]any{
					"default":  nil,
					"map_to":   "tz",
					"name":     "Timezone",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"unicode": map[string]any{
					"default":  true,
					"map_to":   "unicode",
					"name":     "Unicode Characters",
					"private":  false,
					"required": false,
					"type":     "bool",
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
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{sid}:{token}@{from_phone}", "{schema}://{sid}:{token}@{from_phone}/{targets}"},
			"tokens": map[string]any{
				"from_phone": map[string]any{
					"map_to":   "source",
					"name":     "From Phone No / Sender ID",
					"private":  false,
					"regex":    []string{"^(?:(?=.{3,16}$)(?=.*[A-Z.-])[A-Z0-9][A-Z0-9.-]*|[0-9]{6}|\\+?[0-9\\s)(+-]{9,})$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "exotel",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"exotel"},
				},
				"sid": map[string]any{
					"map_to":   "sid",
					"name":     "Account SID",
					"private":  true,
					"required": true,
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
					"group":    []string{"target_phone"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Token",
					"private":  true,
					"required": true,
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
		"secure_protocols": []string{"exotel"},
		"service_name":     "Exotel",
		"service_url":      "https://exotel.com/",
		"setup_url":        "https://appriseit.com/services/exotel/",
	})
}
