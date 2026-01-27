package notify

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const enigma2DefaultTimeout = 13
const enigma2MinTimeout = -1

type Enigma2Target struct {
	host     string
	port     int
	secure   bool
	user     string
	password string
	hasPass  bool
	fullpath string
	timeout  int
	headers  map[string]string
}

func NewEnigma2Target(target *ParsedURL) (*Enigma2Target, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	secure := strings.EqualFold(target.Scheme, "enigma2s")

	fullpath := target.Path
	if strings.TrimSpace(fullpath) == "" {
		fullpath = "/"
	}

	timeout := enigma2DefaultTimeout
	if rawTimeout := strings.TrimSpace(target.Query["timeout"]); rawTimeout != "" {
		if value, err := strconv.Atoi(rawTimeout); err == nil {
			if value < enigma2MinTimeout {
				value = enigma2MinTimeout
			}
			timeout = value
		}
	}

	headers := map[string]string{}
	for key, value := range target.QueryAdd {
		headers[key] = value
	}

	return &Enigma2Target{
		host:     host,
		port:     target.Port,
		secure:   secure,
		user:     target.User,
		password: target.Password,
		hasPass:  target.HasPassword,
		fullpath: fullpath,
		timeout:  timeout,
		headers:  headers,
	}, nil
}

func (e *Enigma2Target) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	message := mergeTitleBody(title, body)

	params := url.Values{}
	params.Set("text", message)
	params.Set("type", strconv.Itoa(enigma2MessageType(notifyType)))
	params.Set("timeout", strconv.Itoa(e.timeout))

	requestURL := e.buildURL() + "?" + params.Encode()
	headers := map[string]string{
		"User-Agent": "Apprise",
	}
	for key, value := range e.headers {
		headers[key] = value
	}
	if e.user != "" {
		password := e.password
		if !e.hasPass {
			password = "None"
		}
		headers["Authorization"] = basicAuthHeader(e.user, password)
	}

	return RequestSpec{
		Method:  "GET",
		URL:     requestURL,
		Headers: headers,
	}, nil
}

func (e *Enigma2Target) Send(body, title string, notifyType NotifyType) error {
	spec, err := e.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (e *Enigma2Target) buildURL() string {
	scheme := "http"
	if e.secure {
		scheme = "https"
	}

	base := fmt.Sprintf("%s://%s", scheme, e.host)
	if e.port > 0 {
		base += fmt.Sprintf(":%d", e.port)
	}

	trimmed := strings.TrimRight(e.fullpath, "/")
	if trimmed == "" {
		trimmed = ""
	}

	return base + trimmed + "/api/message"
}

func enigma2MessageType(notifyType NotifyType) int {
	switch notifyType {
	case NotifyWarning:
		return 2
	case NotifyFailure:
		return 3
	default:
		return 1
	}
}

func init() {
	RegisterSchemaEntryOrdered(5, SchemaEntry{
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
				"timeout": map[string]any{
					"default":  13,
					"map_to":   "timeout",
					"min":      -1,
					"name":     "Server Timeout",
					"private":  false,
					"required": false,
					"type":     "int",
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
			"kwargs": map[string]any{
				"headers": map[string]any{
					"map_to":   "headers",
					"name":     "HTTP Header",
					"prefix":   "+",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{host}", "{schema}://{host}:{port}", "{schema}://{user}@{host}", "{schema}://{user}@{host}:{port}", "{schema}://{user}:{password}@{host}", "{schema}://{user}:{password}@{host}:{port}", "{schema}://{host}/{fullpath}", "{schema}://{host}:{port}/{fullpath}", "{schema}://{user}@{host}/{fullpath}", "{schema}://{user}@{host}:{port}/{fullpath}", "{schema}://{user}:{password}@{host}/{fullpath}", "{schema}://{user}:{password}@{host}:{port}/{fullpath}"},
			"tokens": map[string]any{
				"fullpath": map[string]any{
					"map_to":   "fullpath",
					"name":     "Path",
					"private":  false,
					"required": false,
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
					"values":   []string{"enigma2", "enigma2s"},
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
		"protocols": []string{"enigma2"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"enigma2s"},
		"service_name":     "Enigma2",
		"service_url":      "https://dreambox.de/",
		"setup_url":        "https://appriseit.com/services/enigma2/",
	})
}
