package notify

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	pushDeerDefaultHost = "api2.pushdeer.com"
	pushDeerPath        = "/message/push"
)

type PushDeerTarget struct {
	scheme  string
	host    string
	port    int
	pushKey string
}

func NewPushDeerTarget(target *ParsedURL) (*PushDeerTarget, error) {
	segments := splitPath(target.Path)
	pushKey := ""
	host := target.Host

	if len(segments) == 0 {
		pushKey = target.Host
		host = ""
	} else {
		pushKey = segments[len(segments)-1]
	}

	if pushKey == "" {
		return nil, fmt.Errorf("missing pushkey")
	}

	scheme := "http"
	if strings.ToLower(target.Scheme) == "pushdeers" {
		scheme = "https"
	}

	resolvedHost := host
	if resolvedHost == "" {
		resolvedHost = pushDeerDefaultHost
	}

	port := target.Port
	if port == 0 {
		if scheme == "https" {
			port = 443
		} else {
			port = 80
		}
	}

	return &PushDeerTarget{
		scheme:  scheme,
		host:    resolvedHost,
		port:    port,
		pushKey: pushKey,
	}, nil
}

func (p *PushDeerTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload := url.Values{}
	payload.Set("text", chooseTitle(body, title))
	payload.Set("type", "text")
	if title == "" {
		payload.Set("desp", "")
	} else {
		payload.Set("desp", body)
	}

	u := url.URL{
		Scheme: p.scheme,
		Host:   fmt.Sprintf("%s:%d", p.host, p.port),
		Path:   pushDeerPath,
	}
	q := url.Values{}
	q.Set("pushkey", p.pushKey)
	u.RawQuery = q.Encode()

	headers := map[string]string{
		"Accept":       "*/*",
		"Content-Type": "application/x-www-form-urlencoded",
	}

	_ = notifyType

	return RequestSpec{
		Method:  "POST",
		URL:     u.String(),
		Headers: headers,
		Body:    payload.Encode(),
	}, nil
}

func (p *PushDeerTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := p.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func chooseTitle(body, title string) string {
	if title != "" {
		return title
	}
	return body
}

func init() {
	RegisterSchemaEntryOrdered(58, SchemaEntry{
		"service_name":       "PushDeer",
		"service_url":        "https://www.pushdeer.com/",
		"setup_url":          "https://appriseit.com/services/pushdeer/",
		"attachment_support": false,
		"category":           "native",
		"enabled":            true,
		"protocols":          []string{"pushdeer"},
		"secure_protocols":   []string{"pushdeers"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []string{},
			"packages_required":    []string{},
		},
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
			"kwargs": map[string]any{},
			"templates": []string{
				"{schema}://{pushkey}",
				"{schema}://{host}/{pushkey}",
				"{schema}://{host}:{port}/{pushkey}",
			},
			"tokens": map[string]any{
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
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
				"pushkey": map[string]any{
					"map_to":   "pushkey",
					"name":     "Pushkey",
					"private":  true,
					"regex":    []string{"^[a-z0-9]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"pushdeer", "pushdeers"},
				},
			},
		},
	})
}
