package notify

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/blake2b"
)

var (
	// The server's Curve25519 public key and the bot's Ed25519 seed are both
	// 32 bytes written as hex.
	sogsKeyPattern  = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
	sogsRoomPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
)

// sogsNow and sogsNonce are indirected so a test can pin them; the signature
// covers both, and nothing else about a request is random.
var (
	sogsNow   = func() int64 { return time.Now().Unix() }
	sogsNonce = func() ([]byte, error) {
		nonce := make([]byte, 16)
		_, err := rand.Read(nonce)

		return nonce, err
	}
)

type SOGSTarget struct {
	publicKey  []byte
	signingKey ed25519.PrivateKey
	host       string
	port       int
	secure     bool
	rooms      []string
}

func NewSOGSTarget(target *ParsedURL) (*SOGSTarget, error) {
	publicKeyHex := strings.TrimSpace(target.User)
	if value := strings.TrimSpace(target.Query["key"]); value != "" {
		publicKeyHex = value
	}
	// Session's own join links spell it public_key, and it wins over ?key=.
	if value := strings.TrimSpace(target.Query["public_key"]); value != "" {
		publicKeyHex = value
	}
	if !sogsKeyPattern.MatchString(publicKeyHex) {
		return nil, fmt.Errorf("invalid public key")
	}

	seedHex := strings.TrimSpace(target.Password)
	if value := strings.TrimSpace(target.Query["seed"]); value != "" {
		seedHex = value
	}
	if !sogsKeyPattern.MatchString(seedHex) {
		return nil, fmt.Errorf("invalid seed")
	}

	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	publicKey, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}
	seed, err := hex.DecodeString(seedHex)
	if err != nil {
		return nil, fmt.Errorf("invalid seed: %w", err)
	}

	entries := splitPath(target.Path)
	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		entries = append(entries, parseDelimitedList(to)...)
	}

	rooms := make([]string, 0, len(entries))
	for _, entry := range entries {
		if sogsRoomPattern.MatchString(entry) {
			rooms = append(rooms, entry)
		}
	}
	if len(rooms) == 0 {
		return nil, fmt.Errorf("missing rooms")
	}

	scheme := strings.ToLower(target.Scheme)

	return &SOGSTarget{
		publicKey:  publicKey,
		signingKey: ed25519.NewKeyFromSeed(seed),
		host:       host,
		port:       target.Port,
		secure:     scheme == "sessions" || scheme == "sogs",
		rooms:      rooms,
	}, nil
}

func (s *SOGSTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	specs, err := s.buildRequests(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}

	return specs[0], nil
}

func (s *SOGSTarget) Send(body, title string, notifyType NotifyType) error {
	specs, err := s.buildRequests(body, title, notifyType)
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

func (s *SOGSTarget) buildRequests(body, title string, notifyType NotifyType) ([]RequestSpec, error) {
	_ = notifyType

	// Session has no title field, so the title is folded into the message.
	message := buildSessionMessage(mergeTitleBody(title, body))
	messageSignature := ed25519.Sign(s.signingKey, message)

	// The request body is assembled by hand rather than marshaled, because
	// the auth signature covers these exact bytes: upstream produces them
	// with Python's json.dumps, which puts a space after the colon and the
	// comma. Compact JSON would be a valid request with a signature upstream
	// would never have produced.
	requestBody := fmt.Sprintf(
		`{"data": %q, "signature": %q}`,
		base64.StdEncoding.EncodeToString(message),
		base64.StdEncoding.EncodeToString(messageSignature),
	)

	scheme := "http"
	defaultPort := 80
	if s.secure {
		scheme, defaultPort = "https", 443
	}
	hostPort := s.host
	if s.port > 0 && s.port != defaultPort {
		hostPort = fmt.Sprintf("%s:%d", s.host, s.port)
	}

	specs := make([]RequestSpec, 0, len(s.rooms))
	for _, room := range s.rooms {
		path := "/room/" + room + "/message"

		headers := map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
		}
		auth, err := s.authHeaders("POST", path, []byte(requestBody))
		if err != nil {
			return nil, err
		}
		for key, value := range auth {
			headers[key] = value
		}

		specs = append(specs, RequestSpec{
			Method:  "POST",
			URL:     fmt.Sprintf("%s://%s%s", scheme, hostPort, path),
			Headers: headers,
			Body:    requestBody,
		})
	}

	return specs, nil
}

// authHeaders builds the four X-SOGS-* headers. The signature covers
//
//	SERVER_KEY ‖ NONCE ‖ TIMESTAMP ‖ METHOD ‖ PATH [‖ BLAKE2B-512(BODY)]
//
// with the body hash appended only when there is a body. Get any part of that
// concatenation wrong and the server rejects the request; nothing in a request
// diff would show it, which is what sogs_vectors_test.go exists to catch.
func (s *SOGSTarget) authHeaders(method, path string, body []byte) (map[string]string, error) {
	nonce, err := sogsNonce()
	if err != nil {
		return nil, err
	}
	timestamp := strconv.FormatInt(sogsNow(), 10)

	toSign := make([]byte, 0, len(s.publicKey)+len(nonce)+len(timestamp)+len(method)+len(path)+blake2b.Size)
	toSign = append(toSign, s.publicKey...)
	toSign = append(toSign, nonce...)
	toSign = append(toSign, timestamp...)
	toSign = append(toSign, strings.ToUpper(method)...)
	toSign = append(toSign, path...)
	if len(body) > 0 {
		digest := blake2b.Sum512(body)
		toSign = append(toSign, digest[:]...)
	}

	signature := ed25519.Sign(s.signingKey, toSign)

	return map[string]string{
		// The 00 prefix marks the key as unblinded.
		"X-SOGS-Pubkey":    "00" + hex.EncodeToString(s.signingKey.Public().(ed25519.PublicKey)),
		"X-SOGS-Nonce":     base64.StdEncoding.EncodeToString(nonce),
		"X-SOGS-Timestamp": timestamp,
		"X-SOGS-Signature": base64.StdEncoding.EncodeToString(signature),
	}, nil
}

// buildSessionMessage encodes the text as a Session protocol Content protobuf
// — Content { dataMessage { body: text } } — followed by the 0x80 padding
// marker the server uses to find the message boundary.
func buildSessionMessage(text string) []byte {
	dataMessage := protobufLengthDelimited(1, []byte(text))
	content := protobufLengthDelimited(1, dataMessage)

	return append(content, 0x80)
}

func protobufLengthDelimited(field int, data []byte) []byte {
	// Wire type 2 is length-delimited.
	out := protobufVarint(uint64(field)<<3 | 2)
	out = append(out, protobufVarint(uint64(len(data)))...)

	return append(out, data...)
}

func protobufVarint(value uint64) []byte {
	out := []byte{}
	for {
		b := byte(value & 0x7F)
		value >>= 7
		if value == 0 {
			return append(out, b)
		}
		out = append(out, b|0x80)
	}
}
func init() {
	RegisterSchemaEntryOrdered(163, SchemaEntry{
		"attachment_support": false,
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
				"format": map[string]any{
					"default":  "text",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"key": map[string]any{
					"alias_of": "user",
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
				"public_key": map[string]any{
					"alias_of": "user",
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
				"seed": map[string]any{
					"alias_of": "password",
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
			"templates": []string{"{schema}://{user}:{password}@{host}/{targets}", "{schema}://{user}:{password}@{host}:{port}/{targets}"},
			"tokens": map[string]any{
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"password": map[string]any{
					"map_to":   "seed",
					"name":     "Seed",
					"private":  true,
					"required": true,
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
					"values":   []string{"session", "sessions", "sogs"},
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []any{},
					"map_to":   "targets",
					"name":     "Rooms",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"user": map[string]any{
					"map_to":   "public_key",
					"name":     "Public Key",
					"private":  false,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"session"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{"cryptography"},
		},
		"secure_protocols": []string{"sessions", "sogs"},
		"service_name":     "Session Open Group Server",
		"service_url":      "https://getsession.org/",
		"setup_url":        "https://appriseit.com/services/sogs/",
	})
}
