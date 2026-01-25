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
