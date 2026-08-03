package notify

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const (
	homeAssistantDefaultPort   = 8123
	homeAssistantDefaultDomain = "notify"
	homeAssistantBatchSize     = 10
)

// homeAssistantTokenPattern matches a Home Assistant Long-Lived Access Token:
// either the supervisor form (8 hex, dot, 64 hex) or a JWT (three dot separated
// segments). Path elements that do not match are treated as service targets.
var homeAssistantTokenPattern = regexp.MustCompile(`(?i)^([0-9a-f]{8}\.[0-9a-f]{64}|[a-z0-9_-]+\.[a-z0-9_-]+\.[a-z0-9_-]+)$`)

// homeAssistantServicePattern parses a [domain.]service[:target,...] entry.
var homeAssistantServicePattern = regexp.MustCompile(`(?i)^\s*(?:([a-z0-9_-]+)\.)?([a-z0-9_-]+)(?::([a-z0-9_,-]+))?`)

// homeAssistantPathDelims splits the URL path into elements. Upstream escapes
// the path before splitting it, so a comma never separates path elements: it
// binds sub-targets to the service they follow. Query values such as ?to= are
// unescaped first and do split on commas, so they use splitPath instead.
var homeAssistantPathDelims = regexp.MustCompile(`[ \t\r\n\\/]+`)

// homeAssistantService is a resolved [domain.]service[:target,...] entry.
type homeAssistantService struct {
	domain   string
	service  string
	subjects []string
}

type HomeAssistantTarget struct {
	host        string
	port        int
	secure      bool
	user        string
	password    string
	accessToken string
	nid         string
	fullpath    string
	prefix      string
	batch       bool
	services    []homeAssistantService
}

func NewHomeAssistantTarget(target *ParsedURL) (*HomeAssistantTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	secure := strings.EqualFold(target.Scheme, "hassios")
	port := target.Port
	if port == 0 && !secure {
		port = homeAssistantDefaultPort
	}

	accessToken := strings.TrimSpace(target.Query["accesstoken"])
	if accessToken == "" {
		accessToken = strings.TrimSpace(target.Query["token"])
	}

	parts := homeAssistantSplitPath(target.Path)
	fullpath := ""
	var rawTargets []string

	switch {
	case accessToken != "":
		// The token came from the query string, so every path element is a
		// service target.
		rawTargets = parts

	default:
		// Scan forward for the first element shaped like an access token.
		// Elements after it are service targets; elements before it are too,
		// each reversed so the last URL segment is called first.
		tokenIdx := -1
		for i, part := range parts {
			if homeAssistantTokenPattern.MatchString(part) {
				tokenIdx = i
				accessToken = part
				break
			}
		}

		switch {
		case tokenIdx >= 0:
			rawTargets = append(reverseStrings(parts[tokenIdx+1:]), reverseStrings(parts[:tokenIdx])...)

		case len(parts) > 0:
			// Nothing looked like a token, so fall back to the last path
			// element and treat the URL as persistent notification only.
			accessToken = parts[len(parts)-1]
		}
	}

	if accessToken == "" {
		return nil, fmt.Errorf("missing access token")
	}

	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		rawTargets = append(rawTargets, splitPath(to)...)
	}

	nid := strings.TrimSpace(target.Query["nid"])

	prefix := strings.TrimSpace(target.Query["prefix"])
	if prefix != "" && !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}

	return &HomeAssistantTarget{
		host:        host,
		port:        port,
		secure:      secure,
		user:        strings.TrimSpace(target.User),
		password:    target.Password,
		accessToken: accessToken,
		nid:         nid,
		fullpath:    fullpath,
		prefix:      prefix,
		batch:       parseBool(target.Query["batch"], false),
		services:    parseHomeAssistantServices(rawTargets),
	}, nil
}

func homeAssistantSplitPath(pathValue string) []string {
	trimmed := strings.TrimLeft(pathValue, "/")
	if trimmed == "" {
		return nil
	}

	parts := homeAssistantPathDelims.Split(trimmed, -1)
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			segments = append(segments, part)
		}
	}

	return segments
}

func reverseStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for i := len(values) - 1; i >= 0; i-- {
		out = append(out, values[i])
	}
	return out
}

// parseHomeAssistantServices resolves raw path entries into service calls,
// defaulting the domain to "notify" when one is not supplied. Entries that
// cannot be parsed are dropped, matching upstream.
func parseHomeAssistantServices(entries []string) []homeAssistantService {
	var services []homeAssistantService
	for _, entry := range entries {
		// Only whitespace separates entries; a comma binds sub-targets to the
		// service they follow, so "notify_group:alice,bob" stays one entry.
		for _, candidate := range strings.Fields(entry) {
			match := homeAssistantServicePattern.FindStringSubmatch(candidate)
			if match == nil {
				continue
			}

			domain := match[1]
			if domain == "" {
				domain = homeAssistantDefaultDomain
			}

			var subjects []string
			for _, subject := range strings.Split(match[3], ",") {
				if subject = strings.TrimSpace(subject); subject != "" {
					subjects = append(subjects, subject)
				}
			}

			services = append(services, homeAssistantService{
				domain:   domain,
				service:  match[2],
				subjects: subjects,
			})
		}
	}

	return services
}

func (h *HomeAssistantTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	specs, err := h.buildRequests(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}

	return specs[0], nil
}

func (h *HomeAssistantTarget) Send(body, title string, notifyType NotifyType) error {
	specs, err := h.buildRequests(body, title, notifyType)
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

// buildRequests returns one request per service call. Without service targets
// a single persistent notification request is produced.
func (h *HomeAssistantTarget) buildRequests(body, title string, notifyType NotifyType) ([]RequestSpec, error) {
	_ = notifyType

	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + h.accessToken,
	}
	if h.user != "" {
		password := h.password
		if password == "" {
			password = "None"
		}
		headers["Authorization"] = basicAuthHeader(h.user, password)
	}

	build := func(url string, payload map[string]any) (RequestSpec, error) {
		data, err := json.Marshal(payload)
		if err != nil {
			return RequestSpec{}, err
		}

		return RequestSpec{
			Method:  "POST",
			URL:     url,
			Headers: headers,
			Body:    string(data),
		}, nil
	}

	if len(h.services) == 0 {
		// notification_id is only meaningful for persistent notifications;
		// other service domains reject it.
		spec, err := build(h.persistentURL(), map[string]any{
			"title":           title,
			"message":         body,
			"notification_id": h.notificationID(),
		})
		if err != nil {
			return nil, err
		}

		return []RequestSpec{spec}, nil
	}

	batchSize := 1
	if h.batch {
		batchSize = homeAssistantBatchSize
	}

	specs := make([]RequestSpec, 0, len(h.services))
	for _, service := range h.services {
		url := h.serviceURL(service)

		if len(service.subjects) == 0 {
			spec, err := build(url, map[string]any{"title": title, "message": body})
			if err != nil {
				return nil, err
			}
			specs = append(specs, spec)
			continue
		}

		for start := 0; start < len(service.subjects); start += batchSize {
			end := min(start+batchSize, len(service.subjects))
			spec, err := build(url, map[string]any{
				"title":   title,
				"message": body,
				"targets": service.subjects[start:end],
			})
			if err != nil {
				return nil, err
			}
			specs = append(specs, spec)
		}
	}

	return specs, nil
}

func (h *HomeAssistantTarget) baseURL() string {
	scheme := "http"
	if h.secure {
		scheme = "https"
	}

	base := fmt.Sprintf("%s://%s", scheme, h.host)
	if h.port > 0 {
		base += fmt.Sprintf(":%d", h.port)
	}

	return base
}

func (h *HomeAssistantTarget) persistentURL() string {
	return h.baseURL() + strings.TrimRight(h.fullpath, "/") + "/api/services/persistent_notification/create"
}

func (h *HomeAssistantTarget) serviceURL(service homeAssistantService) string {
	return fmt.Sprintf("%s%s/api/services/%s/%s", h.baseURL(), strings.TrimRight(h.prefix, "/"), service.domain, service.service)
}

func (h *HomeAssistantTarget) notificationID() string {
	if h.nid != "" {
		return h.nid
	}
	return newUUIDv4()
}

func newUUIDv4() string {
	if strings.TrimSpace(os.Getenv("APPRISE_FIXED_TIME")) != "" {
		return "00000000-0000-4000-8000-000000000000"
	}

	buf := make([]byte, 16)
	_, err := rand.Read(buf)
	if err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(buf[0:4]),
		hex.EncodeToString(buf[4:6]),
		hex.EncodeToString(buf[6:8]),
		hex.EncodeToString(buf[8:10]),
		hex.EncodeToString(buf[10:16]),
	)
}

func init() {
	RegisterSchemaEntryOrdered(124, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"batch": map[string]any{
					"default":  false,
					"map_to":   "batch",
					"name":     "Batch Mode",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"prefix": map[string]any{
					"map_to":   "prefix",
					"name":     "Path Prefix",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"to": map[string]any{
					"alias_of": "targets",
					"delim":    []string{",", " "},
				},
				"token": map[string]any{
					"alias_of": "accesstoken",
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
				"nid": map[string]any{
					"map_to":   "nid",
					"name":     "Notification ID",
					"private":  false,
					"regex":    []string{"^[a-z0-9_-]+$", "i"},
					"required": false,
					"type":     "string",
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
			"templates": []string{"{schema}://{host}/{accesstoken}", "{schema}://{host}/{accesstoken}/{targets}", "{schema}://{host}:{port}/{accesstoken}", "{schema}://{host}:{port}/{accesstoken}/{targets}", "{schema}://{user}@{host}/{accesstoken}", "{schema}://{user}@{host}/{accesstoken}/{targets}", "{schema}://{user}@{host}:{port}/{accesstoken}", "{schema}://{user}@{host}:{port}/{accesstoken}/{targets}", "{schema}://{user}:{password}@{host}/{accesstoken}", "{schema}://{user}:{password}@{host}/{accesstoken}/{targets}", "{schema}://{user}:{password}@{host}:{port}/{accesstoken}", "{schema}://{user}:{password}@{host}:{port}/{accesstoken}/{targets}"},
			"tokens": map[string]any{
				"accesstoken": map[string]any{
					"map_to":   "accesstoken",
					"name":     "Long-Lived Access Token",
					"private":  true,
					"required": true,
					"type":     "string",
				},
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
					"values":   []string{"hassio", "hassios"},
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
					"required": false,
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
		"protocols": []string{"hassio"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"hassios"},
		"service_name":     "HomeAssistant",
		"service_url":      "https://www.home-assistant.io/",
		"setup_url":        "https://appriseit.com/services/homeassistant/",
	})
}
