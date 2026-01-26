package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

var gotifyPriorityOrder = []struct {
	prefix string
	value  int
}{
	{"10", 10},
	{"0", 0},
	{"1", 0},
	{"2", 0},
	{"3", 3},
	{"4", 3},
	{"5", 5},
	{"6", 5},
	{"7", 5},
	{"8", 8},
	{"9", 8},
	{"l", 0},
	{"m", 3},
	{"n", 5},
	{"h", 8},
	{"e", 10},
}

type GotifyTarget struct {
	target   *ParsedURL
	token    string
	fullpath string
	priority int
}

func NewGotifyTarget(target *ParsedURL) (*GotifyTarget, error) {
	segments := splitPath(target.Path)
	if len(segments) == 0 {
		return nil, fmt.Errorf("missing token")
	}

	token := segments[len(segments)-1]
	pathSegments := segments[:len(segments)-1]
	fullpath := "/"
	if len(pathSegments) > 0 {
		fullpath = "/" + strings.Join(pathSegments, "/") + "/"
	}

	priority := 5
	if rawPriority, ok := target.Query["priority"]; ok && rawPriority != "" {
		priority = parseGotifyPriority(rawPriority, priority)
	}

	return &GotifyTarget{
		target:   target,
		token:    token,
		fullpath: fullpath,
		priority: priority,
	}, nil
}

func (g *GotifyTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	scheme := "http"
	if strings.ToLower(g.target.Scheme) == "gotifys" {
		scheme = "https"
	}

	host := g.target.Host
	if g.target.Port != 0 {
		host = fmt.Sprintf("%s:%d", host, g.target.Port)
	}

	u := url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   fmt.Sprintf("%smessage", g.fullpath),
	}

	payload := map[string]any{
		"priority": g.priority,
		"title":    title,
		"message":  body,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/json",
		"X-Gotify-Key": g.token,
	}

	return RequestSpec{
		Method:  "POST",
		URL:     u.String(),
		Headers: headers,
		Body:    string(data),
	}, nil
}

func (g *GotifyTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := g.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func parseGotifyPriority(raw string, fallback int) int {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	for _, entry := range gotifyPriorityOrder {
		if strings.HasPrefix(normalized, entry.prefix) {
			return entry.value
		}
	}

	if value, err := strconv.Atoi(normalized); err == nil {
		return value
	}

	return fallback
}

func init() {
	RegisterSchemaEntryOrdered(52, SchemaEntry{
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
					"default":  5,
					"map_to":   "priority",
					"name":     "Priority",
					"private":  false,
					"required": false,
					"type":     "choice:int",
					"values":   []any{0, 3, 5, 8, 10},
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
			"templates": []string{"{schema}://{host}/{token}", "{schema}://{host}:{port}/{token}", "{schema}://{host}{path}{token}", "{schema}://{host}:{port}{path}{token}"},
			"tokens": map[string]any{
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"path": map[string]any{
					"default":  "/",
					"map_to":   "fullpath",
					"name":     "Path",
					"private":  false,
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
					"values":   []string{"gotify", "gotifys"},
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Token",
					"private":  true,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"gotify"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"gotifys"},
		"service_name":     "Gotify",
		"service_url":      "https://github.com/gotify/server",
		"setup_url":        "https://appriseit.com/services/gotify/",
	})
}
