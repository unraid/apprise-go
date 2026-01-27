package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type SynologyTarget struct {
	host     string
	port     int
	secure   bool
	user     string
	password string
	token    string
	fullpath string
	fileURL  string
	headers  map[string]string
}

func NewSynologyTarget(target *ParsedURL) (*SynologyTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	token := strings.TrimSpace(target.Query["token"])
	fullpath := ""
	if token == "" {
		entries := splitPath(target.Path)
		if len(entries) == 0 {
			return nil, fmt.Errorf("missing token")
		}
		token = entries[0]
		entries = entries[1:]
		if len(entries) > 0 {
			fullpath = "/" + strings.Join(entries, "/")
		}
	}
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	fileURL := strings.TrimSpace(target.Query["file_url"])

	headers := map[string]string{}
	for key, value := range target.QueryAdd {
		headers[key] = value
	}

	return &SynologyTarget{
		host:     host,
		port:     target.Port,
		secure:   strings.EqualFold(target.Scheme, "synologys"),
		user:     strings.TrimSpace(target.User),
		password: target.Password,
		token:    token,
		fullpath: fullpath,
		fileURL:  fileURL,
		headers:  headers,
	}, nil
}

func (s *SynologyTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	message := mergeTitleBody(title, body)

	textValue, err := json.Marshal(message)
	if err != nil {
		return RequestSpec{}, err
	}

	payloadData := []byte{}
	if s.fileURL == "" {
		payloadData = []byte(fmt.Sprintf("{\"text\": %s}", textValue))
	} else {
		fileValue, err := json.Marshal(s.fileURL)
		if err != nil {
			return RequestSpec{}, err
		}
		payloadData = []byte(fmt.Sprintf("{\"text\": %s, \"file_url\": %s}", textValue, fileValue))
	}

	params := url.Values{}
	params.Set("api", "SYNO.Chat.External")
	params.Set("method", "incoming")
	params.Set("version", "2")
	params.Set("token", s.token)

	requestURL := s.buildURL() + "?" + params.Encode()

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "*/*",
	}
	for key, value := range s.headers {
		headers[key] = value
	}
	if s.user != "" {
		pass := s.password
		if pass == "" {
			pass = "None"
		}
		headers["Authorization"] = basicAuthHeader(s.user, pass)
	}

	_ = notifyType

	return RequestSpec{
		Method:  "POST",
		URL:     requestURL,
		Headers: headers,
		Body:    "payload=" + string(payloadData),
	}, nil
}

func (s *SynologyTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := s.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (s *SynologyTarget) buildURL() string {
	scheme := "http"
	if s.secure {
		scheme = "https"
	}

	base := fmt.Sprintf("%s://%s", scheme, s.host)
	if s.port > 0 {
		base += fmt.Sprintf(":%d", s.port)
	}

	path := strings.TrimRight(s.fullpath, "/")
	return base + path + "/webapi/entry.cgi"
}

func init() {
	RegisterSchemaEntryOrdered(21, SchemaEntry{
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
				"file_url": map[string]any{
					"map_to":   "file_url",
					"name":     "Upload",
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
			"templates": []string{"{schema}://{host}/{token}", "{schema}://{host}:{port}/{token}", "{schema}://{user}@{host}/{token}", "{schema}://{user}@{host}:{port}/{token}", "{schema}://{user}:{password}@{host}/{token}", "{schema}://{user}:{password}@{host}:{port}/{token}"},
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
					"values":   []string{"synology", "synologys"},
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Token",
					"private":  true,
					"required": true,
					"type":     "string",
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
		"protocols": []string{"synology"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"synologys"},
		"service_name":     "Synology Chat",
		"service_url":      "https://www.synology.com/",
		"setup_url":        "https://appriseit.com/services/synology_chat/",
	})
}
