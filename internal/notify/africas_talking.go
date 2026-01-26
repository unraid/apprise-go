package notify

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	africasTalkingBulkURL    = "https://api.africastalking.com/version1/messaging"
	africasTalkingPremiumURL = "https://content.africastalking.com/version1/messaging"
	africasTalkingSandboxURL = "https://api.sandbox.africastalking.com/version1/messaging"

	africasTalkingDefaultSender   = "AFRICASTKNG"
	africasTalkingDefaultMode     = "bulksms"
	africasTalkingDefaultBatchLen = 50
)

var africasTalkingTokenRe = regexp.MustCompile(`(?i)^[a-z0-9_-]+$`)
var africasTalkingModes = []string{"bulksms", "premium", "sandbox"}

type AfricasTalkingTarget struct {
	appuser string
	apikey  string
	sender  string
	mode    string
	batch   bool
	targets []string
}

func NewAfricasTalkingTarget(target *ParsedURL) (*AfricasTalkingTarget, error) {
	appuser := strings.TrimSpace(target.User)
	if rawUser := strings.TrimSpace(target.Query["user"]); rawUser != "" {
		appuser = rawUser
	}
	if appuser == "" {
		return nil, fmt.Errorf("missing appuser")
	}
	if !africasTalkingTokenRe.MatchString(appuser) {
		return nil, fmt.Errorf("invalid appuser")
	}

	apikey := strings.TrimSpace(target.Host)
	hostTarget := ""
	if rawKey := strings.TrimSpace(target.Query["apikey"]); rawKey != "" {
		apikey = rawKey
		hostTarget = strings.TrimSpace(target.Host)
	} else if rawKey := strings.TrimSpace(target.Query["key"]); rawKey != "" {
		apikey = rawKey
		hostTarget = strings.TrimSpace(target.Host)
	}
	if apikey == "" {
		return nil, fmt.Errorf("missing apikey")
	}
	if !africasTalkingTokenRe.MatchString(apikey) {
		return nil, fmt.Errorf("invalid apikey")
	}

	sender := strings.TrimSpace(target.Query["from"])
	if rawSender := strings.TrimSpace(target.Query["sender"]); rawSender != "" {
		sender = rawSender
	}
	if sender == "" {
		sender = africasTalkingDefaultSender
	}

	mode := africasTalkingDefaultMode
	if rawMode := strings.TrimSpace(target.Query["mode"]); rawMode != "" {
		normalized, ok := normalizeAfricasTalkingMode(rawMode)
		if !ok {
			return nil, fmt.Errorf("invalid mode")
		}
		mode = normalized
	}

	batch := parseBoolWithDefault(target.Query["batch"], false)

	entries := []string{}
	if hostTarget != "" {
		entries = append(entries, hostTarget)
	}
	entries = append(entries, splitPath(target.Path)...)
	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		entries = append(entries, parseDelimitedList(toValue)...)
	}

	targets := []string{}
	for _, entry := range entries {
		normalized, ok := normalizePhoneWithPlus(entry)
		if !ok {
			continue
		}
		targets = append(targets, normalized)
	}

	return &AfricasTalkingTarget{
		appuser: appuser,
		apikey:  apikey,
		sender:  sender,
		mode:    mode,
		batch:   batch,
		targets: targets,
	}, nil
}

func (a *AfricasTalkingTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(a.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	batchSize := 1
	if a.batch {
		batchSize = africasTalkingDefaultBatchLen
	}
	chunk := a.targets
	if len(chunk) > batchSize {
		chunk = chunk[:batchSize]
	}

	spec, err := a.buildRequest(chunk, message)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return spec, nil
}

func (a *AfricasTalkingTarget) Send(body, title string, notifyType NotifyType) error {
	if len(a.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	batchSize := 1
	if a.batch {
		batchSize = africasTalkingDefaultBatchLen
	}

	for idx := 0; idx < len(a.targets); idx += batchSize {
		end := idx + batchSize
		if end > len(a.targets) {
			end = len(a.targets)
		}
		spec, err := a.buildRequest(a.targets[idx:end], message)
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

func (a *AfricasTalkingTarget) buildRequest(targets []string, message string) (RequestSpec, error) {
	requestURL, ok := africasTalkingModeURL(a.mode)
	if !ok {
		return RequestSpec{}, fmt.Errorf("invalid mode")
	}

	payload := url.Values{}
	payload.Set("username", a.appuser)
	payload.Set("to", strings.Join(targets, ","))
	payload.Set("from", a.sender)
	payload.Set("message", message)

	return RequestSpec{
		Method: "POST",
		URL:    requestURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/x-www-form-urlencoded",
			"Accept":       "application/json",
			"apiKey":       a.apikey,
		},
		Body: payload.Encode(),
	}, nil
}

func normalizeAfricasTalkingMode(raw string) (string, bool) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return africasTalkingDefaultMode, true
	}
	for _, mode := range africasTalkingModes {
		if strings.HasPrefix(mode, raw) {
			return mode, true
		}
	}
	return "", false
}

func africasTalkingModeURL(mode string) (string, bool) {
	switch mode {
	case "bulksms":
		return africasTalkingBulkURL, true
	case "premium":
		return africasTalkingPremiumURL, true
	case "sandbox":
		return africasTalkingSandboxURL, true
	default:
		return "", false
	}
}

func init() {
	RegisterSchemaEntryOrdered(80, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"apikey": map[string]any{
					"alias_of": "apikey",
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
					"default":  "AFRICASTKNG",
					"map_to":   "sender",
					"name":     "From",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"mode": map[string]any{
					"default":  "bulksms",
					"map_to":   "mode",
					"name":     "SMS Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"bulksms", "premium", "sandbox"},
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
			"templates": []string{"{schema}://{appuser}@{apikey}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"regex":    []string{"^[A-Z0-9_-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"appuser": map[string]any{
					"map_to":   "appuser",
					"name":     "App User Name",
					"private":  false,
					"regex":    []string{"^[A-Z0-9_-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "atalk",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"atalk"},
				},
				"target_phone": map[string]any{
					"map_to":   "targets",
					"name":     "Target Phone",
					"private":  false,
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
			},
		},
		"enabled":   true,
		"protocols": nil,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"atalk"},
		"service_name":     "Africas Talking",
		"service_url":      "https://africastalking.com/",
		"setup_url":        "https://appriseit.com/services/africas_talking/",
	})
}
