package notify

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

const simplepushURL = "https://api.simplepush.io/send"

const simplepushBlockSize = aes.BlockSize

type SimplePushTarget struct {
	apiKey   string
	event    string
	user     string
	password string
	iv       []byte
	ivHex    string
	key      []byte
}

func NewSimplePushTarget(target *ParsedURL) (*SimplePushTarget, error) {
	apiKey := strings.TrimSpace(target.Host)
	if apiKey == "" || strings.ContainsAny(apiKey, " \t\n\r") {
		return nil, fmt.Errorf("missing api key")
	}

	event := strings.TrimSpace(target.Query["event"])

	return &SimplePushTarget{
		apiKey:   apiKey,
		event:    event,
		user:     target.User,
		password: target.Password,
	}, nil
}

func (s *SimplePushTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload := map[string]string{
		"key":   s.apiKey,
		"msg":   body,
		"title": title,
	}

	if s.user != "" && s.password != "" {
		encryptedBody, err := s.encrypt(body)
		if err != nil {
			return RequestSpec{}, err
		}
		encryptedTitle, err := s.encrypt(title)
		if err != nil {
			return RequestSpec{}, err
		}
		payload["msg"] = encryptedBody
		payload["title"] = encryptedTitle
		payload["encrypted"] = "true"
		payload["iv"] = s.ivHex
	}

	if s.event != "" {
		payload["event"] = s.event
	}

	values := url.Values{}
	for key, value := range payload {
		values.Set(key, value)
	}

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    simplepushURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/x-www-form-urlencoded",
		},
		Body: values.Encode(),
	}, nil
}

func (s *SimplePushTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := s.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (s *SimplePushTarget) encrypt(content string) (string, error) {
	if err := s.ensureEncryption(); err != nil {
		return "", err
	}

	padded := pkcs7Pad([]byte(content), simplepushBlockSize)
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}

	ciphertext := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, s.iv)
	mode.CryptBlocks(ciphertext, padded)

	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

func (s *SimplePushTarget) ensureEncryption() error {
	if s.iv != nil && s.key != nil {
		return nil
	}

	iv := make([]byte, simplepushBlockSize)
	override := strings.TrimSpace(os.Getenv("APPRISE_SIMPLEPUSH_TEST_IV"))
	if override != "" {
		if decoded, err := hex.DecodeString(override); err == nil && len(decoded) == simplepushBlockSize {
			copy(iv, decoded)
		}
	}
	if isZeroBlock(iv) {
		if _, err := io.ReadFull(rand.Reader, iv); err != nil {
			return err
		}
	}

	hash := sha1.Sum([]byte(s.password + s.user))
	hexDigest := hex.EncodeToString(hash[:])
	keyBytes, err := hex.DecodeString(hexDigest[:32])
	if err != nil {
		return err
	}

	s.iv = iv
	s.ivHex = strings.ToUpper(hex.EncodeToString(iv))
	s.key = keyBytes
	return nil
}

func pkcs7Pad(input []byte, blockSize int) []byte {
	pad := blockSize - (len(input) % blockSize)
	if pad == 0 {
		pad = blockSize
	}
	padded := make([]byte, 0, len(input)+pad)
	padded = append(padded, input...)
	for i := 0; i < pad; i++ {
		padded = append(padded, byte(pad))
	}
	return padded
}

func isZeroBlock(input []byte) bool {
	for _, b := range input {
		if b != 0 {
			return false
		}
	}
	return true
}

func init() {
	RegisterSchemaEntryOrdered(16, SchemaEntry{
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
				"event": map[string]any{
					"map_to":   "event",
					"name":     "Event",
					"private":  false,
					"required": false,
					"type":     "string",
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
			"templates": []string{"{schema}://{apikey}", "{schema}://{salt}:{password}@{apikey}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
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
				"salt": map[string]any{
					"map_to":   "user",
					"name":     "Salt",
					"private":  true,
					"required": false,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "spush",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"spush"},
				},
			},
		},
		"enabled":   true,
		"protocols": nil,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []string{"cryptography"},
		},
		"secure_protocols": []string{"spush"},
		"service_name":     "SimplePush",
		"service_url":      "https://simplepush.io/",
		"setup_url":        "https://appriseit.com/services/simplepush/",
	})
}
