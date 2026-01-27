package notify

import (
	"fmt"
	"strings"
)

const noticaOfficialURL = "https://notica.us/?%s"

type NoticaTarget struct {
	mode    string
	scheme  string
	host    string
	port    int
	path    string
	token   string
	headers map[string]string
	user    string
	pass    string
}

func NewNoticaTarget(target *ParsedURL) (*NoticaTarget, error) {
	segments := splitPath(target.Path)
	mode := "official"
	token := ""
	host := ""
	path := ""

	if len(segments) == 0 {
		mode = "official"
		token = target.Host
	} else {
		mode = "selfhosted"
		token = segments[len(segments)-1]
		host = target.Host
		if len(segments) > 1 {
			path = "/" + strings.Join(segments[:len(segments)-1], "/") + "/"
		} else {
			path = "/"
		}
	}

	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	scheme := "http"
	if strings.ToLower(target.Scheme) == "noticas" {
		scheme = "https"
	}

	return &NoticaTarget{
		mode:    mode,
		scheme:  scheme,
		host:    host,
		port:    target.Port,
		path:    path,
		token:   token,
		headers: cloneMap(target.QueryAdd),
		user:    target.User,
		pass:    target.Password,
	}, nil
}

func (n *NoticaTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/x-www-form-urlencoded",
	}

	if n.mode == "selfhosted" {
		for key, value := range n.headers {
			headers[key] = value
		}
		if n.user != "" {
			pass := n.pass
			if pass == "" {
				pass = "None"
			}
			headers["Authorization"] = basicAuthHeader(n.user, pass)
		}
	}

	url := ""
	if n.mode == "official" {
		url = fmt.Sprintf(noticaOfficialURL, n.token)
	} else {
		host := n.host
		if n.port != 0 {
			host = fmt.Sprintf("%s:%d", host, n.port)
		}
		url = fmt.Sprintf("%s://%s%s?token=%s", n.scheme, host, n.path, n.token)
	}

	message := mergeTitleBody(title, body)
	_ = notifyType

	return RequestSpec{
		Method:  "POST",
		URL:     url,
		Headers: headers,
		Body:    "d:" + message,
	}, nil
}

func (n *NoticaTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := n.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func init() {
	RegisterSchemaEntryOrdered(34, SchemaEntry{
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
			"templates": []string{"{schema}://{token}", "{schema}://{host}/{token}", "{schema}://{host}:{port}/{token}", "{schema}://{user}@{host}/{token}", "{schema}://{user}@{host}:{port}/{token}", "{schema}://{user}:{password}@{host}/{token}", "{schema}://{user}:{password}@{host}:{port}/{token}", "{schema}://{host}{path}/{token}", "{schema}://{host}:{port}/{path}/{token}", "{schema}://{user}@{host}/{path}/{token}", "{schema}://{user}@{host}:{port}{path}/{token}", "{schema}://{user}:{password}@{host}{path}/{token}", "{schema}://{user}:{password}@{host}:{port}/{path}/{token}"},
			"tokens": map[string]any{
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"password": map[string]any{
					"map_to":   "password",
					"name":     "Password",
					"private":  true,
					"required": false,
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
					"values":   []string{"notica", "noticas"},
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Token",
					"private":  true,
					"regex":    []any{"^\\?*(?P<token>[^/]+)\\s*$", nil},
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
		"protocols": []string{"notica"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"noticas"},
		"service_name":     "Notica",
		"service_url":      "https://notica.us/",
		"setup_url":        "https://appriseit.com/services/notica/",
	})
}
