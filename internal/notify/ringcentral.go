package notify

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	ringCentralAccessTokenTTL  = 3600
	ringCentralRefreshTokenTTL = 604800

	ringCentralModeBasic = "basic"
	ringCentralModeJWT   = "jwt"

	// A token longer than this is a JWT rather than a password.
	ringCentralJWTMinLen = 60
)

// The environment selects an infix in the API hostname.
var ringCentralEnvironments = map[string]string{
	"production": "",
	"sandbox":    ".devtest",
}

var (
	ringCentralClientPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	ringCentralJWTPattern    = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

type RingCentralTarget struct {
	clientID     string
	clientSecret string
	token        string
	source       string
	mode         string
	environment  string
	targets      []string
}

func NewRingCentralTarget(target *ParsedURL) (*RingCentralTarget, error) {
	clientID := strings.TrimSpace(target.Host)

	// The first path segment is the client secret; what follows is the
	// source phone number when the URL carries no password, then targets.
	entries := splitPath(target.Path)
	clientSecret := ""
	if len(entries) > 0 {
		clientSecret, entries = entries[0], entries[1:]
	}

	source, token := "", ""
	if target.HasPassword && target.Password != "" {
		// ringc://source:token@client_id/secret[/targets]
		source = strings.TrimSpace(target.User)
		token = strings.TrimSpace(target.Password)
	} else {
		// ringc://token@client_id/secret/source[/targets]
		token = strings.TrimSpace(target.User)
		if len(entries) > 0 {
			source, entries = entries[0], entries[1:]
		}
	}

	if secret := strings.TrimSpace(target.Query["secret"]); secret != "" {
		clientSecret = secret
	}
	// The token override has to land before the mode is detected, so that a
	// JWT supplied this way is still recognised as one.
	if override := strings.TrimSpace(target.Query["token"]); override != "" {
		token = override
	}
	if from := strings.TrimSpace(target.Query["from"]); from != "" {
		source = from
	}
	if override := strings.TrimSpace(target.Query["source"]); override != "" {
		source = override
	}

	mode, err := ringCentralMode(target.Query["mode"], token)
	if err != nil {
		return nil, err
	}

	environment, err := ringCentralEnvironment(target.Query["env"])
	if err != nil {
		return nil, err
	}

	if !ringCentralClientPattern.MatchString(clientID) {
		return nil, fmt.Errorf("invalid client id: %s", clientID)
	}
	if !ringCentralClientPattern.MatchString(clientSecret) {
		return nil, fmt.Errorf("invalid client secret: %s", clientSecret)
	}
	if mode == ringCentralModeJWT && !ringCentralJWTPattern.MatchString(token) {
		return nil, fmt.Errorf("invalid jwt token")
	}
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	normalizedSource, ok := normalizePhone(source)
	if !ok {
		return nil, fmt.Errorf("invalid source phone number: %s", source)
	}

	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		entries = append(entries, parseDelimitedList(to)...)
	}

	targets := make([]string, 0, len(entries))
	for _, entry := range entries {
		if normalized, ok := normalizePhone(entry); ok {
			targets = append(targets, normalized)
		}
	}

	return &RingCentralTarget{
		clientID:     clientID,
		clientSecret: clientSecret,
		token:        token,
		source:       normalizedSource,
		mode:         mode,
		environment:  environment,
		targets:      targets,
	}, nil
}

// BuildRequest cannot describe this provider: the send carries a bearer token
// that only exists after the OAuth request.
func (r *RingCentralTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	_, _, _ = body, title, notifyType

	return RequestSpec{}, fmt.Errorf("multi-step request")
}

func (r *RingCentralTarget) Send(body, title string, notifyType NotifyType) error {
	_ = notifyType

	accessToken, err := r.login()
	if err != nil {
		return err
	}

	for _, spec := range r.messageSpecs(body, title, accessToken) {
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func (r *RingCentralTarget) endpoint(path string) string {
	return fmt.Sprintf("https://platform%s.ringcentral.com%s", ringCentralEnvironments[r.environment], path)
}

// login exchanges the credentials for a bearer token. Upstream revokes the
// token from the notifier's destructor rather than after a send, so no
// revocation request belongs here.
func (r *RingCentralTarget) login() (string, error) {
	form := url.Values{}
	if r.mode == ringCentralModeJWT {
		form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
		form.Set("assertion", r.token)
	} else {
		form.Set("grant_type", "password")
		form.Set("username", "+"+r.source)
		form.Set("password", r.token)
		form.Set("access_token_ttl", fmt.Sprintf("%d", ringCentralAccessTokenTTL))
		form.Set("refresh_token_ttl", fmt.Sprintf("%d", ringCentralRefreshTokenTTL))
	}

	credentials := base64.StdEncoding.EncodeToString([]byte(r.clientID + ":" + r.clientSecret))
	spec := RequestSpec{
		Method: "POST",
		URL:    r.endpoint("/restapi/oauth/token"),
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Accept":        "application/json",
			"Content-Type":  "application/x-www-form-urlencoded",
			"Authorization": "Basic " + credentials,
		},
		Body: form.Encode(),
	}

	var response struct {
		AccessToken string `json:"access_token"`
	}
	if err := doJSONRequest(spec, &response); err != nil {
		return "", err
	}
	if response.AccessToken == "" {
		return "", fmt.Errorf("login response contained no access token")
	}

	return response.AccessToken, nil
}

// messageSpecs builds one SMS per recipient. With no targets the message goes
// back to the source number, which is upstream's loopback test.
func (r *RingCentralTarget) messageSpecs(body, title, accessToken string) []RequestSpec {
	recipients := r.targets
	if len(recipients) == 0 {
		recipients = []string{r.source}
	}

	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Accept":        "application/json",
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + accessToken,
	}

	text := mergeTitleBody(title, body)
	specs := make([]RequestSpec, 0, len(recipients))
	for _, recipient := range recipients {
		payload := map[string]any{
			"from": map[string]string{"phoneNumber": "+" + r.source},
			"to":   []map[string]string{{"phoneNumber": "+" + recipient}},
			"text": text,
		}
		data, err := json.Marshal(payload)
		if err != nil {
			continue
		}

		specs = append(specs, RequestSpec{
			Method:  "POST",
			URL:     r.endpoint("/restapi/v1.0/account/~/extension/~/sms"),
			Headers: headers,
			Body:    string(data),
		})
	}

	return specs
}

// ringCentralMode resolves the auth mode, falling back to guessing from the
// token's length: a JWT is far longer than a password.
func ringCentralMode(raw, token string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		if len(token) > ringCentralJWTMinLen {
			return ringCentralModeJWT, nil
		}
		return ringCentralModeBasic, nil
	}

	for _, candidate := range []string{ringCentralModeBasic, ringCentralModeJWT} {
		if strings.HasPrefix(candidate, normalized) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("invalid mode: %s", raw)
}

func ringCentralEnvironment(raw string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return "production", nil
	}

	for _, candidate := range []string{"production", "sandbox"} {
		if strings.HasPrefix(candidate, normalized) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("invalid environment: %s", raw)
}
func init() {
	RegisterSchemaEntryOrdered(162, SchemaEntry{
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
				"env": map[string]any{
					"default":  "prod",
					"map_to":   "environment",
					"name":     "Environment",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"prod", "sandbox"},
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
				"mode": map[string]any{
					"map_to":   "mode",
					"name":     "Authentication Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"basic", "jwt"},
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
				"secret": map[string]any{
					"alias_of": "secret",
				},
				"source": map[string]any{
					"alias_of": "from_phone",
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
			"templates": []string{"{schema}://{from_phone}:{token}@{client_id}/{secret}/", "{schema}://{from_phone}:{token}@{client_id}/{secret}/{targets}", "{schema}://{token}@{client_id}/{secret}/{from_phone}", "{schema}://{token}@{client_id}/{secret}/{from_phone}/{targets}"},
			"tokens": map[string]any{
				"client_id": map[string]any{
					"map_to":   "client_id",
					"name":     "Client ID",
					"private":  true,
					"regex":    []string{"^[a-z0-9_-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"from_phone": map[string]any{
					"map_to":   "source",
					"name":     "From Phone No",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "ringc",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"ringc"},
				},
				"secret": map[string]any{
					"map_to":   "client_secret",
					"name":     "Client Secret",
					"private":  true,
					"regex":    []string{"^[a-z0-9_-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"target_phone": map[string]any{
					"map_to":   "targets",
					"name":     "Target Phone No",
					"prefix":   "+",
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
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Token / Password",
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
		"protocols":        nil,
		"secure_protocols": []string{"ringc"},
		"service_name":     "RingCentral",
		"service_url":      "https://ringcentral.com/",
		"setup_url":        "https://appriseit.com/services/ringcentral/",
	})
}
