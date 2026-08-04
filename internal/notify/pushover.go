package notify

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const (
	pushoverURL              = "https://api.pushover.net/1/messages.json"
	pushoverDefaultSound     = "pushover"
	pushoverDefaultPriority  = 0
	pushoverDefaultAppDesc   = "Apprise Notifications"
	pushoverSendToAllDevices = "ALL_DEVICES"
)

var pushoverPriorityMap = map[string]int{
	"l":  -2,
	"m":  -1,
	"n":  0,
	"h":  1,
	"e":  2,
	"-2": -2,
	"-1": -1,
	"0":  0,
	"1":  1,
	"2":  2,
}

// pushoverGroupPattern matches a group key target, which upstream distinguishes
// from a device by its leading hash. The hash may arrive percent encoded.
var pushoverGroupPattern = regexp.MustCompile(`(?i)^\s*(?:%23|#)([a-z0-9]+)\s*$`)

// pushoverEncryptionKeyPattern matches the 256 bit E2EE key as hex.
var pushoverEncryptionKeyPattern = regexp.MustCompile(`(?i)^[0-9a-f]{64}$`)

type PushoverTarget struct {
	userKey              string
	token                string
	devices              []string
	groups               []string
	sound                string
	priority             int
	supplementalURL      string
	supplementalURLTitle string
	interval             int
	expire               int
	format               string
	encryptionKey        string
	e2ee                 bool
}

func NewPushoverTarget(target *ParsedURL) (*PushoverTarget, error) {
	userKey := target.User
	token := target.Host
	if userKey == "" || token == "" {
		return nil, fmt.Errorf("missing user key or token")
	}

	targets := splitPath(target.Path)
	if rawTargets, ok := target.Query["to"]; ok && rawTargets != "" {
		targets = append(targets, splitList(rawTargets)...)
	}

	var devices, groups []string
	if len(targets) == 0 {
		devices = []string{pushoverSendToAllDevices}
	} else {
		for _, entry := range targets {
			if match := pushoverGroupPattern.FindStringSubmatch(entry); match != nil {
				groups = append(groups, match[1])
				continue
			}
			devices = append(devices, entry)
		}
	}

	priority := pushoverDefaultPriority
	if rawPriority := strings.TrimSpace(target.Query["priority"]); rawPriority != "" {
		priority = parsePushoverPriority(rawPriority)
	}

	interval := 0
	expire := 0
	if priority == 2 {
		// Upstream renamed the emergency retry interval to interval; retry is
		// now the inherited service retry argument.
		interval = 900
		if rawInterval := strings.TrimSpace(target.Query["interval"]); rawInterval != "" {
			if parsed, err := strconv.Atoi(rawInterval); err == nil {
				interval = parsed
			}
		}
		if interval < 30 {
			return nil, fmt.Errorf("pushover interval must be at least 30 seconds")
		}

		expire = 3600
		if rawExpire := strings.TrimSpace(target.Query["expire"]); rawExpire != "" {
			if parsed, err := strconv.Atoi(rawExpire); err == nil {
				expire = parsed
			}
		}
		if expire < 0 || expire > 10800 {
			return nil, fmt.Errorf("pushover expire must be between 0 and 10800 seconds")
		}
	}

	sound := pushoverDefaultSound
	if rawSound := strings.TrimSpace(target.Query["sound"]); rawSound != "" {
		sound = strings.ToLower(rawSound)
	}

	encryptionKey := strings.TrimSpace(target.Query["key"])
	if encryptionKey != "" && !pushoverEncryptionKeyPattern.MatchString(encryptionKey) {
		return nil, fmt.Errorf("pushover encryption key must be 64 hexadecimal characters")
	}

	// Encryption is on by default once a key is supplied, and can only be
	// turned off explicitly.
	e2ee := encryptionKey != ""
	if raw := strings.TrimSpace(target.Query["e2ee"]); raw != "" {
		e2ee = encryptionKey != "" && parseBool(raw, true)
	}

	return &PushoverTarget{
		userKey:              userKey,
		token:                token,
		devices:              devices,
		groups:               groups,
		sound:                sound,
		priority:             priority,
		supplementalURL:      strings.TrimSpace(target.Query["url"]),
		supplementalURLTitle: strings.TrimSpace(target.Query["url_title"]),
		interval:             interval,
		expire:               expire,
		format:               normalizeNotifyFormat(target.Query["format"]),
		encryptionKey:        encryptionKey,
		e2ee:                 e2ee,
	}, nil
}

func (p *PushoverTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	specs, err := p.buildRequests(body, title, notifyType, nil)
	if err != nil {
		return RequestSpec{}, err
	}

	return specs[0], nil
}

func (p *PushoverTarget) Send(body, title string, notifyType NotifyType) error {
	return p.SendWithAttachments(body, title, notifyType, nil)
}

func (p *PushoverTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	specs, err := p.buildRequests(body, title, notifyType, attachments)
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

// buildRequests returns one request carrying every device and one request per
// group, since a group key is supplied in place of the user key.
func (p *PushoverTarget) buildRequests(body, title string, notifyType NotifyType, attachments []Attachment) ([]RequestSpec, error) {
	_ = notifyType

	resolvedTitle := title
	if resolvedTitle == "" {
		resolvedTitle = pushoverDefaultAppDesc
	}

	message := body
	html := false
	switch p.format {
	case "html":
		html = true
	case "markdown":
		message = markdownToHTML(body)
		html = true
	}

	supplementalURL := p.supplementalURL
	supplementalURLTitle := p.supplementalURLTitle

	if p.e2ee {
		key, err := hex.DecodeString(p.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("pushover encryption key is not valid hex: %w", err)
		}

		// Encryption failure must never fall back to sending plaintext.
		for _, field := range []*string{&message, &resolvedTitle, &supplementalURL, &supplementalURLTitle} {
			if *field == "" {
				continue
			}
			encrypted, err := pushoverEncryptField(*field, key)
			if err != nil {
				return nil, fmt.Errorf("pushover encryption failed: %w", err)
			}
			*field = encrypted
		}
	}

	base := func() url.Values {
		values := url.Values{}
		values.Set("token", p.token)
		values.Set("priority", fmt.Sprintf("%d", p.priority))
		values.Set("title", resolvedTitle)
		values.Set("message", message)
		values.Set("sound", p.sound)
		if supplementalURL != "" {
			values.Set("url", supplementalURL)
		}
		if supplementalURLTitle != "" {
			values.Set("url_title", supplementalURLTitle)
		}
		if html {
			values.Set("html", "1")
		}
		if p.priority == 2 {
			values.Set("retry", fmt.Sprintf("%d", p.interval))
			values.Set("expire", fmt.Sprintf("%d", p.expire))
		}
		if p.e2ee {
			values.Set("encrypted", "1")
		}
		return values
	}

	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Accept":        "*/*",
		"Content-Type":  "application/x-www-form-urlencoded",
		"Authorization": basicAuthHeader(p.token, ""),
	}

	build := func(values url.Values) RequestSpec {
		requestBody := values.Encode()
		spec := RequestSpec{
			Method:  "POST",
			URL:     pushoverURL,
			Headers: headers,
			Body:    requestBody,
		}

		// Pushover accepts images only, and quietly ignores anything else
		// rather than failing, so a PDF simply never arrives.
		usable := []Attachment{}
		for _, attachment := range attachments {
			if strings.HasPrefix(strings.ToLower(attachment.MimeType), "image/") {
				usable = append(usable, attachment)
			}
		}
		if len(usable) == 0 {
			return spec
		}

		// Pushover takes one file per request, under a fixed field name and
		// with no content type on the part.
		multipartBody, contentType, err := singleFileAttachmentBody(
			values, "attachment", usable[0], false)
		if err != nil {
			return spec
		}

		partHeaders := map[string]string{}
		for key, value := range headers {
			partHeaders[key] = value
		}
		partHeaders["Content-Type"] = contentType
		spec.Headers = partHeaders
		spec.Body = multipartBody

		return spec
	}

	var specs []RequestSpec
	if len(p.devices) > 0 {
		values := base()
		values.Set("user", p.userKey)
		values.Set("device", strings.Join(p.devices, ","))
		specs = append(specs, build(values))
	}

	for _, group := range p.groups {
		values := base()
		values.Set("user", group)
		specs = append(specs, build(values))
	}

	if len(specs) == 0 {
		return nil, fmt.Errorf("no pushover targets")
	}

	return specs, nil
}

// pushoverEncryptField applies the Pushover end-to-end field encryption scheme:
// gzip the plaintext, AES-256-CBC encrypt it under a random IV with PKCS7
// padding, then return base64(IV || ciphertext || HMAC-SHA256(IV || ciphertext)).
// See https://pushover.net/api#e2ee.
func pushoverEncryptField(plaintext string, key []byte) (string, error) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte(plaintext)); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}

	padding := aes.BlockSize - compressed.Len()%aes.BlockSize
	padded := append(compressed.Bytes(), bytes.Repeat([]byte{byte(padding)}, padding)...)

	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)

	mac := hmac.New(sha256.New, key)
	mac.Write(iv)
	mac.Write(ciphertext)

	payload := append(append(append([]byte{}, iv...), ciphertext...), mac.Sum(nil)...)
	return base64.StdEncoding.EncodeToString(payload), nil
}

func parsePushoverPriority(raw string) int {
	normalized := strings.TrimSpace(strings.ToLower(raw))
	if normalized == "" {
		return pushoverDefaultPriority
	}
	if value, err := strconv.Atoi(normalized); err == nil {
		if value >= -2 && value <= 2 {
			return value
		}
		return pushoverDefaultPriority
	}
	for key, value := range pushoverPriorityMap {
		if strings.HasPrefix(normalized, key) {
			return value
		}
	}
	return pushoverDefaultPriority
}

func init() {
	RegisterSchemaEntryOrdered(35, SchemaEntry{
		"attachment_support": true,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
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
				"expire": map[string]any{
					"default":  3600,
					"map_to":   "expire",
					"max":      10800,
					"min":      0,
					"name":     "Expire",
					"private":  false,
					"required": false,
					"type":     "int",
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
					"default":  0,
					"map_to":   "priority",
					"name":     "Priority",
					"private":  false,
					"required": false,
					"type":     "choice:int",
					"values":   []any{-2, -1, 0, 1, 2},
				},
				"e2ee": map[string]any{
					"default":  true,
					"map_to":   "e2ee",
					"name":     "E2EE",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"interval": map[string]any{
					"default":  900,
					"map_to":   "interval",
					"min":      30,
					"name":     "Emergency Retry Interval",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"key": map[string]any{
					"map_to":   "encryption_key",
					"name":     "Encryption Key",
					"private":  true,
					"required": false,
					"type":     "string",
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
					"default":  "pushover",
					"map_to":   "sound",
					"name":     "Sound",
					"private":  false,
					"regex":    []string{"^[a-z]{1,12}$", "i"},
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
				"url": map[string]any{
					"map_to":   "supplemental_url",
					"name":     "URL",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"url_title": map[string]any{
					"map_to":   "supplemental_url_title",
					"name":     "URL Title",
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
			"templates": []string{"{schema}://{user_key}@{token}", "{schema}://{user_key}@{token}/{targets}"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "pover",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"pover"},
				},
				"target_device": map[string]any{
					"map_to":   "targets",
					"name":     "Target Device",
					"private":  false,
					"regex":    []string{"^[a-z0-9_-]{1,25}$", "i"},
					"required": false,
					"type":     "string",
				},
				"target_group": map[string]any{
					"map_to":   "targets",
					"name":     "Target Group",
					"prefix":   "#",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_device", "target_group"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Access Token",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"user_key": map[string]any{
					"map_to":   "user_key",
					"name":     "User Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": nil,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{"cryptography"},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"pover"},
		"service_name":     "Pushover",
		"service_url":      "https://pushover.net/",
		"setup_url":        "https://appriseit.com/services/pushover/",
	})
}
