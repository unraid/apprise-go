package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const barkDefaultTitle = "Apprise Notifications"

var barkSounds = []string{
	"alarm.caf",
	"anticipate.caf",
	"bell.caf",
	"birdsong.caf",
	"bloom.caf",
	"calypso.caf",
	"chime.caf",
	"choo.caf",
	"descent.caf",
	"electronic.caf",
	"fanfare.caf",
	"glass.caf",
	"gotosleep.caf",
	"healthnotification.caf",
	"horn.caf",
	"ladder.caf",
	"mailsent.caf",
	"minuet.caf",
	"multiwayinvitation.caf",
	"newmail.caf",
	"newsflash.caf",
	"noir.caf",
	"paymentsuccess.caf",
	"shake.caf",
	"sherwoodforest.caf",
	"silence.caf",
	"spell.caf",
	"suspense.caf",
	"telegraph.caf",
	"tiptoes.caf",
	"typewriters.caf",
	"update.caf",
}

var barkLevels = []string{
	"active",
	"timeSensitive",
	"passive",
	"critical",
}

var barkListDelimiters = regexp.MustCompile(`[\[\];,\s]+`)

type BarkTarget struct {
	targets      []string
	host         string
	port         int
	secure       bool
	user         string
	password     string
	includeImage bool
	sound        string
	category     string
	group        string
	level        string
	click        string
	icon         string
	call         bool
	badge        int
	volume       int
}

func NewBarkTarget(target *ParsedURL) (*BarkTarget, error) {
	if target.Host == "" {
		return nil, fmt.Errorf("missing host")
	}

	targets := parseTargets(target.Path)
	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		targets = append(targets, parseList(toValue)...)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	includeImage := parseBool(target.Query["image"], true)

	sound := matchBarkSound(target.Query["sound"])
	level := matchBarkLevel(target.Query["level"])

	badge := parseIntInRange(target.Query["badge"], 0, 1<<31-1)
	volume := parseIntInRange(target.Query["volume"], 0, 10)

	return &BarkTarget{
		targets:      targets,
		host:         target.Host,
		port:         target.Port,
		secure:       strings.ToLower(target.Scheme) == "barks",
		user:         target.User,
		password:     target.Password,
		includeImage: includeImage,
		sound:        sound,
		category:     strings.TrimSpace(target.Query["category"]),
		group:        strings.TrimSpace(target.Query["group"]),
		level:        level,
		click:        strings.TrimSpace(target.Query["click"]),
		icon:         strings.TrimSpace(target.Query["icon"]),
		call:         parseBool(target.Query["call"], false),
		badge:        badge,
		volume:       volume,
	}, nil
}

func (b *BarkTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(b.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	return b.buildRequestForTarget(b.targets[len(b.targets)-1], body, title, notifyType)
}

func (b *BarkTarget) Send(body, title string, notifyType NotifyType) error {
	if len(b.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	for i := len(b.targets) - 1; i >= 0; i-- {
		spec, err := b.buildRequestForTarget(b.targets[i], body, title, notifyType)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func (b *BarkTarget) buildRequestForTarget(deviceKey, body, title string, notifyType NotifyType) (RequestSpec, error) {
	resolvedTitle := title
	if resolvedTitle == "" {
		resolvedTitle = barkDefaultTitle
	}

	payload := map[string]any{
		"title":      resolvedTitle,
		"body":       body,
		"device_key": deviceKey,
	}

	icon := ""
	if b.icon != "" {
		icon = b.icon
	} else if b.includeImage {
		icon = barkImageURL(notifyType)
	}
	if icon != "" {
		payload["icon"] = icon
	}

	if b.sound != "" {
		payload["sound"] = b.sound
	}
	if b.click != "" {
		payload["url"] = b.click
	}
	if b.badge > 0 {
		payload["badge"] = b.badge
	}
	if b.level != "" {
		payload["level"] = b.level
	}
	if b.category != "" {
		payload["category"] = b.category
	}
	if b.group != "" {
		payload["group"] = b.group
	}
	if b.volume > 0 {
		payload["volume"] = b.volume
	}
	if b.call {
		payload["call"] = 1
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	scheme := "http"
	if b.secure {
		scheme = "https"
	}

	host := b.host
	if b.port != 0 {
		host = fmt.Sprintf("%s:%d", host, b.port)
	}

	requestURL := url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   "/push",
	}

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/json; charset=utf-8",
	}
	if b.user != "" {
		headers["Authorization"] = basicAuthHeader(b.user, b.password)
	}

	return RequestSpec{
		Method:  "POST",
		URL:     requestURL.String(),
		Headers: headers,
		Body:    string(data),
	}, nil
}

func barkImageURL(notifyType NotifyType) string {
	return appriseImageURL(notifyType, "128x128")
}

func matchBarkSound(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	for _, sound := range barkSounds {
		if strings.HasPrefix(sound, value) {
			return sound
		}
	}
	return ""
}

func matchBarkLevel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	initial := strings.ToLower(value[:1])
	for _, level := range barkLevels {
		if strings.HasPrefix(strings.ToLower(level), initial) {
			return level
		}
	}
	return ""
}

func parseTargets(rawPath string) []string {
	path := strings.Trim(rawPath, "/")
	if path == "" {
		return nil
	}
	parts := strings.Split(path, "/")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		decoded, err := url.PathUnescape(part)
		if err != nil {
			decoded = part
		}
		decoded = strings.TrimSpace(decoded)
		if decoded != "" {
			result = append(result, decoded)
		}
	}
	return result
}

func parseList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := barkListDelimiters.Split(raw, -1)
	values := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values[part] = struct{}{}
	}

	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func parseIntInRange(raw string, min, max int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	if value < min || value > max {
		return 0
	}
	return value
}

func init() {
	RegisterSchemaEntryOrdered(87, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"badge": map[string]any{
					"map_to":   "badge",
					"min":      0,
					"name":     "Badge",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"call": map[string]any{
					"default":  false,
					"map_to":   "call",
					"name":     "Call",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"category": map[string]any{
					"map_to":   "category",
					"name":     "Category",
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
				"icon": map[string]any{
					"map_to":   "icon",
					"name":     "Icon URL",
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
				"level": map[string]any{
					"map_to":   "level",
					"name":     "Level",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"active", "timeSensitive", "passive", "critical"},
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
				"sound": map[string]any{
					"map_to":   "sound",
					"name":     "Sound",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"alarm.caf", "anticipate.caf", "bell.caf", "birdsong.caf", "bloom.caf", "calypso.caf", "chime.caf", "choo.caf", "descent.caf", "electronic.caf", "fanfare.caf", "glass.caf", "gotosleep.caf", "healthnotification.caf", "horn.caf", "ladder.caf", "mailsent.caf", "minuet.caf", "multiwayinvitation.caf", "newmail.caf", "newsflash.caf", "noir.caf", "paymentsuccess.caf", "shake.caf", "sherwoodforest.caf", "silence.caf", "spell.caf", "suspense.caf", "telegraph.caf", "tiptoes.caf", "typewriters.caf", "update.caf"},
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
				"volume": map[string]any{
					"map_to":   "volume",
					"max":      10,
					"min":      0,
					"name":     "Volume",
					"private":  false,
					"required": false,
					"type":     "int",
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{host}/{targets}", "{schema}://{host}:{port}/{targets}", "{schema}://{user}:{password}@{host}/{targets}", "{schema}://{user}:{password}@{host}:{port}/{targets}"},
			"tokens": map[string]any{
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"password": map[string]any{
					"map_to":   "password",
					"name":     "Password",
					"private":  true,
					"required": false,
					"type":     "string",
				},
				"port": map[string]any{
					"map_to":   "port",
					"max":      65535,
					"min":      1,
					"name":     "Port",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"bark", "barks"},
				},
				"target_device": map[string]any{
					"map_to":   "targets",
					"name":     "Target Device",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_device"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": true,
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
		"protocols": []string{"bark"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"barks"},
		"service_name":     "Bark",
		"service_url":      "https://github.com/Finb/Bark",
		"setup_url":        "https://appriseit.com/services/bark/",
	})
}
