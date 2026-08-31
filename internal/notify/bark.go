package notify

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
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
	notifyFormat string

	// encryptionKey switches the wire payload to Bark's AES-GCM envelope.
	encryptionKey string
}

func NewBarkTarget(target *ParsedURL) (*BarkTarget, error) {
	if target.Host == "" {
		return nil, fmt.Errorf("missing host")
	}

	targets := parseTargets(target.Path)
	if toValue, ok := target.Query["to"]; ok && toValue != "" {
		targets = append(targets, parseList(toValue)...)
	}
	includeImage := parseBool(target.Query["image"], true)

	sound := matchBarkSound(target.Query["sound"])
	level := matchBarkLevel(target.Query["level"])

	badge := parseIntInRange(target.Query["badge"], 0, 1<<31-1)
	volume := parseIntInRange(target.Query["volume"], 0, 10)

	// Encryption is off unless an encryption key was explicitly given.
	// AES-GCM only accepts 128/192/256-bit (16/24/32 byte) ASCII keys.
	encryptionKey := target.Query["key"]
	if encryptionKey != "" {
		if !isASCII(encryptionKey) {
			return nil, fmt.Errorf("bark encryption key must contain only ascii characters")
		}
		switch len(encryptionKey) {
		case 16, 24, 32:
		default:
			return nil, fmt.Errorf("bark encryption key must contain exactly 16, 24, or 32 ascii characters")
		}
	}

	// An empty target list is not refused here. Upstream builds the object
	// and reports the failure when the send is attempted; both make no
	// request and both report failure, so matching upstream keeps the rest
	// of a configuration file behaving identically either way. The guard
	// lives on the send path instead.
	return &BarkTarget{
		targets:       targets,
		host:          target.Host,
		port:          target.Port,
		secure:        strings.ToLower(target.Scheme) == "barks",
		user:          target.User,
		password:      target.Password,
		includeImage:  includeImage,
		sound:         sound,
		category:      strings.TrimSpace(target.Query["category"]),
		group:         strings.TrimSpace(target.Query["group"]),
		level:         level,
		click:         strings.TrimSpace(target.Query["click"]),
		icon:          strings.TrimSpace(target.Query["icon"]),
		call:          parseBool(target.Query["call"], false),
		badge:         badge,
		volume:        volume,
		notifyFormat:  normalizeNotifyFormat(target.Query["format"]),
		encryptionKey: encryptionKey,
	}, nil
}

func (b *BarkTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(b.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	return b.buildRequestForTarget(b.targets[len(b.targets)-1], body, title, notifyType)
}

func (b *BarkTarget) Send(body, title string, notifyType NotifyType) error {
	// Upstream keeps going after a failed target; see sendOutcome.
	var outcome sendOutcome
	if len(b.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	for i := len(b.targets) - 1; i >= 0; i-- {
		spec, err := b.buildRequestForTarget(b.targets[i], body, title, notifyType)
		if err != nil {
			return err
		}
		outcome.record(SendRequest(spec))
	}

	return outcome.err()
}

func (b *BarkTarget) buildRequestForTarget(deviceKey, body, title string, notifyType NotifyType) (RequestSpec, error) {
	resolvedTitle := title
	if resolvedTitle == "" {
		resolvedTitle = barkDefaultTitle
	}

	// Upstream builds the parameter object in a fixed order; the encrypted
	// envelope serializes it verbatim, so the order is part of the wire
	// format there.
	pairs := []jsonPair{{"title", resolvedTitle}}
	if b.notifyFormat == "markdown" {
		pairs = append(pairs, jsonPair{"markdown", body})
	} else {
		pairs = append(pairs, jsonPair{"body", body})
	}

	icon := ""
	if b.icon != "" {
		icon = b.icon
	} else if b.includeImage {
		icon = barkImageURL(notifyType)
	}
	if icon != "" {
		pairs = append(pairs, jsonPair{"icon", icon})
	}

	if b.sound != "" {
		pairs = append(pairs, jsonPair{"sound", b.sound})
	}
	if b.click != "" {
		pairs = append(pairs, jsonPair{"url", b.click})
	}
	if b.badge > 0 {
		pairs = append(pairs, jsonPair{"badge", b.badge})
	}
	if b.level != "" {
		pairs = append(pairs, jsonPair{"level", b.level})
	}
	if b.category != "" {
		pairs = append(pairs, jsonPair{"category", b.category})
	}
	if b.group != "" {
		pairs = append(pairs, jsonPair{"group", b.group})
	}
	if b.volume > 0 {
		pairs = append(pairs, jsonPair{"volume", b.volume})
	}
	if b.call {
		pairs = append(pairs, jsonPair{"call", 1})
	}

	var data []byte
	if b.encryptionKey != "" {
		// Encrypt the payload; the plaintext fields are replaced by a
		// device_key/ciphertext/iv wire payload for this target.
		ciphertext, iv, err := barkEncryptPayload(b.encryptionKey, pairs)
		if err != nil {
			return RequestSpec{}, err
		}
		encrypted, err := json.Marshal(map[string]any{
			"device_key": deviceKey,
			"ciphertext": ciphertext,
			"iv":         iv,
		})
		if err != nil {
			return RequestSpec{}, err
		}
		data = encrypted
	} else {
		payload := map[string]any{"device_key": deviceKey}
		for _, pair := range pairs {
			payload[pair.key] = pair.value
		}
		plain, err := json.Marshal(payload)
		if err != nil {
			return RequestSpec{}, err
		}
		data = plain
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
				"key": map[string]any{
					"map_to":   "encryption_key",
					"name":     "Encryption Key",
					"private":  true,
					"required": false,
					"type":     "string",
				},
				"level": map[string]any{
					"map_to":   "level",
					"name":     "Level",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"active", "timeSensitive", "passive", "critical"},
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
					"private":  true,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_device"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  true,
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
			"packages_recommended": []any{"cryptography"},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"barks"},
		"service_name":     "Bark",
		"service_url":      "https://github.com/Finb/Bark",
		"setup_url":        "https://appriseit.com/services/bark/",
	})
}

type jsonPair struct {
	key   string
	value any
}

// barkCompactJSON serializes the ordered parameter object exactly the way
// upstream's json.dumps(..., ensure_ascii=False, separators=(",", ":")) does;
// the bytes are encrypted, so any difference changes the ciphertext.
func barkCompactJSON(pairs []jsonPair) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, pair := range pairs {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := encodeJSONValue(pair.key)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		value, err := encodeJSONValue(pair.value)
		if err != nil {
			return nil, err
		}
		buf.Write(value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func encodeJSONValue(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// barkEncryptPayload encrypts one Bark parameter object with a fresh AES-GCM
// IV, mirroring upstream's secrets.token_urlsafe(9): 12 URL-safe ASCII
// characters used directly as the 96-bit nonce.
func barkEncryptPayload(key string, pairs []jsonPair) (ciphertext, iv string, err error) {
	iv = strings.TrimSpace(os.Getenv("APPRISE_BARK_TEST_IV"))
	if len(iv) != 12 || !isASCII(iv) {
		random := make([]byte, 9)
		if _, err = rand.Read(random); err != nil {
			return "", "", err
		}
		iv = base64.RawURLEncoding.EncodeToString(random)
	}

	plaintext, err := barkCompactJSON(pairs)
	if err != nil {
		return "", "", err
	}

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}

	sealed := gcm.Seal(nil, []byte(iv), plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), iv, nil
}
