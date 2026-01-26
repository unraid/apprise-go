package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const sfrURL = "https://www.dmc.sfr-sh.fr/DmcWS/1.5.8/JsonService/MessagesUnitairesWS/addSingleCall"

type SFRTarget struct {
	user     string
	password string
	spaceID  string
	lang     string
	sender   string
	media    string
	timeout  int
	voice    string
	targets  []string
}

type sfrAuthPayload struct {
	ServiceID       string `json:"serviceId"`
	ServicePassword string `json:"servicePassword"`
	SpaceID         string `json:"spaceId"`
	Lang            string `json:"lang"`
}

type sfrMessagePayload struct {
	Media    string `json:"media"`
	TextMsg  string `json:"textMsg"`
	To       string `json:"to"`
	From     string `json:"from"`
	Timeout  int    `json:"timeout"`
	TTSVoice string `json:"ttsVoice"`
}

func NewSFRTarget(target *ParsedURL) (*SFRTarget, error) {
	user := strings.TrimSpace(target.User)
	password := target.Password
	if user == "" || password == "" {
		return nil, fmt.Errorf("missing credentials")
	}

	spaceID := strings.TrimSpace(target.Host)
	if spaceID == "" {
		return nil, fmt.Errorf("missing space id")
	}

	lang := strings.TrimSpace(target.Query["lang"])
	if lang == "" {
		lang = "fr_FR"
	}

	sender := strings.TrimSpace(target.Query["sender"])
	if sender == "" {
		sender = strings.TrimSpace(target.Query["from"])
	}

	media := strings.TrimSpace(target.Query["media"])
	if media == "" {
		media = "SMSUnicode"
	}

	timeout := 2880
	if raw := strings.TrimSpace(target.Query["timeout"]); raw != "" {
		value, err := strconv.Atoi(raw)
		if err == nil {
			timeout = value
		}
	}

	voice := strings.TrimSpace(target.Query["voice"])
	if voice == "" {
		voice = "claire08s"
	}

	targets := []string{}
	for _, entry := range splitPath(target.Path) {
		if normalized, ok := normalizePhone(entry); ok {
			targets = append(targets, normalized)
		}
	}
	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		for _, entry := range parseDelimitedList(toValue) {
			if normalized, ok := normalizePhone(entry); ok {
				targets = append(targets, normalized)
			}
		}
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	return &SFRTarget{
		user:     user,
		password: password,
		spaceID:  spaceID,
		lang:     lang,
		sender:   sender,
		media:    media,
		timeout:  timeout,
		voice:    voice,
		targets:  targets,
	}, nil
}

func (s *SFRTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(s.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	payload, err := s.buildPayload(s.targets[0], message)
	if err != nil {
		return RequestSpec{}, err
	}

	requestURL := sfrURL
	if encoded := payload.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    requestURL,
		Headers: map[string]string{
			"Accept": "*/*",
		},
	}, nil
}

func (s *SFRTarget) Send(body, title string, notifyType NotifyType) error {
	if len(s.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	message := mergeTitleBody(title, body)
	for _, target := range s.targets {
		payload, err := s.buildPayload(target, message)
		if err != nil {
			return err
		}

		requestURL := sfrURL
		if encoded := payload.Encode(); encoded != "" {
			requestURL += "?" + encoded
		}

		spec := RequestSpec{
			Method: "POST",
			URL:    requestURL,
			Headers: map[string]string{
				"Accept": "*/*",
			},
		}

		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = notifyType

	return nil
}

func (s *SFRTarget) buildPayload(target, message string) (url.Values, error) {
	authPayload := sfrAuthPayload{
		ServiceID:       s.user,
		ServicePassword: s.password,
		SpaceID:         s.spaceID,
		Lang:            s.lang,
	}
	authData, err := json.Marshal(authPayload)
	if err != nil {
		return nil, err
	}
	authText := addJSONSpaces(authData)

	messagePayload := sfrMessagePayload{
		Media:    s.media,
		TextMsg:  message,
		To:       target,
		From:     s.sender,
		Timeout:  s.timeout,
		TTSVoice: s.voice,
	}
	messageData, err := json.Marshal(messagePayload)
	if err != nil {
		return nil, err
	}
	messageText := addJSONSpaces(messageData)

	payload := url.Values{}
	payload.Set("authenticate", authText)
	payload.Set("messageUnitaire", messageText)
	return payload, nil
}

func addJSONSpaces(input []byte) string {
	var b strings.Builder
	b.Grow(len(input))

	inString := false
	escape := false
	for _, c := range input {
		if inString {
			b.WriteByte(c)
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
			} else if c == '"' {
				inString = false
			}
			continue
		}

		switch c {
		case '"':
			inString = true
			b.WriteByte(c)
		case ':':
			b.WriteByte(':')
			b.WriteByte(' ')
		case ',':
			b.WriteByte(',')
			b.WriteByte(' ')
		default:
			b.WriteByte(c)
		}
	}

	return b.String()
}

func init() {
	RegisterSchemaEntryOrdered(1, SchemaEntry{
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
					"alias_of": "sender",
				},
				"lang": map[string]any{
					"default":  "fr_FR",
					"map_to":   "lang",
					"name":     "Language",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"media": map[string]any{
					"default":  "SMSUnicode",
					"map_to":   "media",
					"name":     "Media Type",
					"private":  false,
					"required": true,
					"type":     "string",
					"values":   []string{"SMS", "SMSLong", "SMSUnicode", "SMSUnicodeLong"},
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
				"sender": map[string]any{
					"default":  "",
					"map_to":   "sender",
					"name":     "Sender Name",
					"private":  false,
					"required": true,
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
				"timeout": map[string]any{
					"default":  2880,
					"map_to":   "timeout",
					"name":     "Timeout",
					"private":  false,
					"required": false,
					"type":     "int",
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
				"voice": map[string]any{
					"default":  "claire08s",
					"map_to":   "voice",
					"name":     "TTS Voice",
					"private":  false,
					"required": false,
					"type":     "string",
					"values":   []string{"claire08s", "laura8k"},
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{user}:{password}@{space_id}/{targets}"},
			"tokens": map[string]any{
				"password": map[string]any{
					"map_to":   "password",
					"name":     "Service Password",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "sfr",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"sfr"},
				},
				"space_id": map[string]any{
					"map_to":   "space_id",
					"name":     "Space ID",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"target": map[string]any{
					"map_to":   "targets",
					"name":     "Recipient Phone Number",
					"private":  false,
					"regex":    []string{"^\\+?[0-9\\s)(+-]+$", "i"},
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": true,
					"type":     "list:string",
				},
				"user": map[string]any{
					"map_to":   "user",
					"name":     "Service ID",
					"private":  false,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"sfr"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": nil,
		"service_name":     "Société Française du Radiotéléphone",
		"service_url":      "https://www.sfr.fr/",
		"setup_url":        "https://appriseit.com/services/sfr/",
	})
}
