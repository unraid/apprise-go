package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	pingramRegionUS = "us"
	pingramRegionCA = "ca"
	pingramRegionEU = "eu"
)

const (
	pingramModeTemplate = "template"
	pingramModeMessage  = "message"
)

var pingramRegionURLs = map[string]string{
	pingramRegionUS: "https://api.pingram.io",
	pingramRegionCA: "https://api.ca.pingram.io",
	pingramRegionEU: "https://api.eu.pingram.io",
}

var pingramChannels = map[string]struct{}{
	"email":       {},
	"sms":         {},
	"inapp":       {},
	"web_push":    {},
	"mobile_push": {},
	"slack":       {},
	"call":        {},
}

var pingramAPIKeyRe = regexp.MustCompile(`(?i)^pingram_(sk|pk)_[\w-]+$`)

type pingramTargetEntry struct {
	id     string
	email  string
	number string
}

type PingramTarget struct {
	apiKey       string
	messageType  string
	mode         string
	notifyFormat string
	region       string
	channels     []string
	targets      []pingramTargetEntry
	cc           map[string]struct{}
	bcc          map[string]struct{}
	fromAddr     string
	fromName     string
	tokens       map[string]string
}

func NewPingramTarget(target *ParsedURL) (*PingramTarget, error) {
	entries := []string{}
	if strings.TrimSpace(target.Host) != "" {
		entries = append(entries, target.Host)
	}
	entries = append(entries, splitPath(target.Path)...)

	apiKey := strings.TrimSpace(target.Query["apikey"])
	if apiKey == "" && len(entries) > 0 {
		apiKey = strings.TrimSpace(entries[0])
		entries = entries[1:]
	}
	if !pingramAPIKeyRe.MatchString(apiKey) {
		return nil, fmt.Errorf("invalid api key: %s", apiKey)
	}

	fromAddr := strings.TrimSpace(target.Query["from"])
	fromName := ""
	if fromAddr != "" {
		if !isSimpleEmail(fromAddr) {
			fromAddr = ""
		}
	}

	messageType := strings.TrimSpace(target.Query["type"])
	if messageType == "" {
		messageType = strings.TrimSpace(target.User)
	}

	mode := strings.ToLower(strings.TrimSpace(target.Query["mode"]))
	if mode != "" && mode != pingramModeTemplate && mode != pingramModeMessage {
		return nil, fmt.Errorf("invalid mode: %s", mode)
	}

	if mode == "" {
		if messageType == "" {
			mode = pingramModeMessage
		} else {
			mode = pingramModeTemplate
		}
	}

	if messageType == "" {
		messageType = "apprise"
	}

	region := strings.ToLower(strings.TrimSpace(target.Query["region"]))
	if region == "" {
		region = pingramRegionUS
	}
	if _, ok := pingramRegionURLs[region]; !ok {
		return nil, fmt.Errorf("invalid region: %s", region)
	}

	channelSet := map[string]struct{}{}
	if rawChannels := strings.TrimSpace(target.Query["channels"]); rawChannels != "" {
		for _, entry := range parseDelimitedList(rawChannels) {
			entry = strings.ToLower(strings.TrimSpace(entry))
			if entry == "" {
				continue
			}
			if _, ok := pingramChannels[entry]; !ok {
				return nil, fmt.Errorf("invalid channel: %s", entry)
			}
			channelSet[entry] = struct{}{}
		}
	}

	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		entries = append(entries, parseDelimitedList(toValue)...)
	}

	targets := parsePingramTargets(entries, channelSet)

	cc := map[string]struct{}{}
	if ccValue := strings.TrimSpace(target.Query["cc"]); ccValue != "" {
		for _, entry := range parseDelimitedList(ccValue) {
			entry = strings.TrimSpace(entry)
			if isSimpleEmail(entry) {
				cc[entry] = struct{}{}
			}
		}
	}

	bcc := map[string]struct{}{}
	if bccValue := strings.TrimSpace(target.Query["bcc"]); bccValue != "" {
		for _, entry := range parseDelimitedList(bccValue) {
			entry = strings.TrimSpace(entry)
			if isSimpleEmail(entry) {
				bcc[entry] = struct{}{}
			}
		}
	}

	channels := make([]string, 0, len(channelSet))
	for channel := range channelSet {
		channels = append(channels, channel)
	}

	if fromAddr != "" {
		fromName = "Apprise"
	}

	tokens := map[string]string{}
	for key, value := range target.QueryPayload {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		tokens[key] = value
	}

	notifyFormat := normalizeNotifyFormat(target.Query["format"])
	if notifyFormat == "" {
		notifyFormat = "text"
	}

	return &PingramTarget{
		apiKey:       apiKey,
		notifyFormat: notifyFormat,
		messageType:  messageType,
		mode:         mode,
		region:       region,
		channels:     channels,
		targets:      targets,
		cc:           cc,
		bcc:          bcc,
		fromAddr:     fromAddr,
		fromName:     fromName,
		tokens:       tokens,
	}, nil
}

func (n *PingramTarget) Send(body, title string, notifyType NotifyType) error {
	if len(n.targets) == 0 {
		return fmt.Errorf("missing targets")
	}
	var outcome sendOutcome
	for _, target := range n.targets {
		spec, err := n.buildRequestFor(body, title, notifyType, target)
		if err != nil {
			return err
		}
		outcome.record(SendRequest(spec))
	}
	return outcome.err()
}

func (n *PingramTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(n.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}
	return n.buildRequestFor(body, title, notifyType, n.targets[0])
}

func (n *PingramTarget) buildRequestFor(body, title string, notifyType NotifyType, target pingramTargetEntry) (RequestSpec, error) {
	baseURL, ok := pingramRegionURLs[n.region]
	if !ok {
		return RequestSpec{}, fmt.Errorf("invalid region: %s", n.region)
	}

	payload := n.buildPayload(body, title, notifyType, target)
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    baseURL + "/send",
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + n.apiKey,
		},
		Body: string(data),
	}, nil
}

func (n *PingramTarget) buildPayload(body, title string, notifyType NotifyType, target pingramTargetEntry) map[string]any {
	payload := map[string]any{
		"type": n.messageType,
	}

	if n.mode == pingramModeTemplate {
		parameters := map[string]any{}
		for key, value := range n.tokens {
			parameters[key] = value
		}

		parameters["appBody"] = body
		parameters["appTitle"] = title
		parameters["appType"] = string(notifyType)
		parameters["appId"] = "Apprise"
		parameters["appDescription"] = appriseAppDesc
		parameters["appColor"] = appriseColor(notifyType)
		parameters["appImageUrl"] = appriseImageURL(notifyType, "72x72")
		parameters["appUrl"] = appriseAppURL
		payload["parameters"] = parameters
	} else {
		// Acquire the text version of the body when it arrived as HTML.
		textBody := body
		if n.notifyFormat == "html" {
			textBody = htmlToText(body)
		}
		for _, channel := range n.channels {
			switch channel {
			case "sms", "call":
				message := textBody
				if title != "" {
					message = title + "\n" + textBody
				}
				payload[channel] = map[string]any{
					"message": message,
				}
			case "email":
				subject := title
				if subject == "" {
					subject = "Apprise"
				}
				htmlBody := body
				if n.notifyFormat != "html" {
					htmlBody = textToHTML(body)
				}
				payload["email"] = map[string]any{
					"subject": subject,
					"html":    htmlBody,
				}
				if n.fromAddr != "" {
					payload["email"].(map[string]any)["senderEmail"] = n.fromAddr
					payload["email"].(map[string]any)["senderName"] = n.fromName
				}
			case "inapp":
				fallback := title
				if fallback == "" {
					fallback = "Apprise"
				}
				payload["inapp"] = map[string]any{
					"title": fallback,
					"image": appriseImageURL(notifyType, "72x72"),
				}
			case "web_push":
				fallback := title
				if fallback == "" {
					fallback = "Apprise"
				}
				payload["web_push"] = map[string]any{
					"title":   fallback,
					"message": textBody,
					"icon":    appriseImageURL(notifyType, "72x72"),
				}
			case "mobile_push":
				fallback := title
				if fallback == "" {
					fallback = "Apprise"
				}
				payload["mobile_push"] = map[string]any{
					"title":   fallback,
					"message": textBody,
				}
			case "slack":
				message := textBody
				if title != "" {
					message = title + "\n" + textBody
				}
				payload["slack"] = map[string]any{
					"text": message,
				}
			}
		}
	}

	if n.fromAddr != "" {
		payload["options"] = map[string]any{
			"email": map[string]any{
				"fromAddress": n.fromAddr,
				"fromName":    n.fromName,
			},
		}
	} else if len(n.cc) > 0 || len(n.bcc) > 0 {
		payload["options"] = map[string]any{
			"email": map[string]any{},
		}
	}

	to := map[string]any{}
	if target.id != "" {
		to["id"] = target.id
	}
	if target.email != "" {
		to["email"] = target.email
	}
	if target.number != "" {
		to["number"] = target.number
	}
	payload["to"] = to

	if len(n.cc) > 0 || len(n.bcc) > 0 {
		ccSet := n.cc
		bccSet := n.bcc
		if target.email != "" {
			ccSet = subtractSet(ccSet, n.bcc, target.email)
			bccSet = subtractSet(bccSet, nil, target.email)
		}
		if len(ccSet) > 0 {
			payload["options"].(map[string]any)["email"].(map[string]any)["ccAddresses"] = setToList(ccSet)
		}
		if len(bccSet) > 0 {
			payload["options"].(map[string]any)["email"].(map[string]any)["bccAddresses"] = setToList(bccSet)
		}
	}

	return payload
}

// parsePingramTargets mirrors upstream: a recipient id is always optional, so
// a bare email or phone number is enough to identify a target on its own.
// Invalid entries are dropped rather than failing the URL.
func parsePingramTargets(entries []string, channels map[string]struct{}) []pingramTargetEntry {
	targets := []pingramTargetEntry{}
	current := pingramTargetEntry{}

	for _, raw := range entries {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}

		if isSimpleEmail(trimmed) {
			if current.email == "" {
				current.email = trimmed
				if len(channels) == 0 {
					channels["email"] = struct{}{}
				}
				continue
			}
			targets = append(targets, current)
			current = pingramTargetEntry{email: trimmed}
			continue
		}

		if number, ok := normalizePhoneWithPlus(trimmed); ok {
			if current.number == "" {
				current.number = number
				if len(channels) == 0 {
					channels["sms"] = struct{}{}
				}
				continue
			}
			targets = append(targets, current)
			current = pingramTargetEntry{number: number}
			continue
		}

		if match := pingramIDRe.FindStringSubmatch(trimmed); match != nil {
			id := match[1]
			if current.id == "" {
				current.id = id
				continue
			}
			targets = append(targets, current)
			current = pingramTargetEntry{id: id}
			continue
		}
	}

	if current != (pingramTargetEntry{}) {
		targets = append(targets, current)
	}

	return targets
}

func setToList(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	return out
}

func subtractSet(input map[string]struct{}, remove map[string]struct{}, item string) map[string]struct{} {
	out := map[string]struct{}{}
	for value := range input {
		if value == item {
			continue
		}
		if remove != nil {
			if _, ok := remove[value]; ok {
				continue
			}
		}
		out[value] = struct{}{}
	}
	return out
}

var pingramIDRe = regexp.MustCompile(`^\s*(?:@|%40)?([\w_-]+)\s*$`)

func init() {
	RegisterSchemaEntryOrdered(32, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"apikey": map[string]any{
					"alias_of": "apikey",
				},
				"bcc": map[string]any{
					"delim":    []string{",", " "},
					"group":    []any{},
					"map_to":   "bcc",
					"name":     "Blind Carbon Copy",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"cc": map[string]any{
					"delim":    []string{",", " "},
					"group":    []any{},
					"map_to":   "cc",
					"name":     "Carbon Copy",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"channels": map[string]any{
					"delim":    []string{",", " "},
					"group":    []any{},
					"map_to":   "channels",
					"name":     "Channels",
					"private":  false,
					"required": false,
					"type":     "list:string",
					"values":   []string{"call", "email", "inapp", "mobile_push", "slack", "sms", "web_push"},
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
					"map_to":   "from_addr",
					"name":     "From Email",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"mode": map[string]any{
					"map_to":   "mode",
					"name":     "Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"message", "template"},
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
					"map_to":   "region",
					"name":     "Region Name",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"us", "ca", "eu"},
				},
				"reply": map[string]any{
					"map_to":   "reply_to",
					"name":     "Reply To",
					"private":  false,
					"required": false,
					"type":     "string",
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
				"to": map[string]any{
					"alias_of": "targets",
					"delim":    []string{",", " "},
				},
				"type": map[string]any{
					"alias_of": "type",
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
				"tokens": map[string]any{
					"map_to":   "tokens",
					"name":     "Template Tokens",
					"prefix":   ":",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{apikey}/{targets}", "{schema}://{type}@{apikey}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"regex":    []string{"^pingram_(sk|pk)_[\\w-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "pingram",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"pingram"},
				},
				"target_email": map[string]any{
					"map_to":   "targets",
					"name":     "Target Email",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_id": map[string]any{
					"map_to":   "targets",
					"name":     "Target ID",
					"prefix":   "@",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_sms": map[string]any{
					"map_to":   "targets",
					"name":     "Target SMS",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_email", "target_id", "target_sms"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"type": map[string]any{
					"map_to":   "message_type",
					"name":     "Message Type",
					"private":  false,
					"regex":    []string{"^[A-Z0-9_-]+$", "i"},
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
		"secure_protocols": []string{"pingram"},
		"service_name":     "Pingram",
		"service_url":      "https://www.pingram.io/",
		"setup_url":        "https://appriseit.com/services/pingram/",
	})
}
