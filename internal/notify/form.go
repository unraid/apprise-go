package notify

import (
	"fmt"
	"net/url"
	"strings"
)

var formMethods = map[string]struct{}{
	"POST":   {},
	"GET":    {},
	"DELETE": {},
	"PUT":    {},
	"HEAD":   {},
	"PATCH":  {},
}

type FormTarget struct {
	target        *ParsedURL
	method        string
	headers       map[string]string
	params        map[string]string
	payloadExtras map[string]string
	payloadMap    map[string]string
}

func NewFormTarget(target *ParsedURL) (*FormTarget, error) {
	method := "POST"
	if rawMethod, ok := target.Query["method"]; ok && rawMethod != "" {
		method = strings.ToUpper(rawMethod)
	}
	if _, ok := formMethods[method]; !ok {
		return nil, fmt.Errorf("invalid method: %s", method)
	}

	payloadExtras := cloneMap(target.QueryPayload)
	payloadMap := map[string]string{
		"version": "version",
		"title":   "title",
		"message": "message",
		"type":    "type",
	}

	for key, value := range payloadExtras {
		if _, ok := payloadMap[key]; !ok {
			continue
		}

		payloadMap[key] = value
		delete(payloadExtras, key)
	}

	return &FormTarget{
		target:        target,
		method:        method,
		headers:       cloneMap(target.QueryAdd),
		params:        cloneMap(target.QueryDel),
		payloadExtras: payloadExtras,
		payloadMap:    payloadMap,
	}, nil
}

func (f *FormTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := f.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (f *FormTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload := map[string]string{}

	base := map[string]string{
		"version": "1.0",
		"title":   title,
		"message": body,
		"type":    string(notifyType),
	}

	for key, value := range base {
		mapped := f.payloadMap[key]
		if mapped == "" {
			continue
		}
		payload[mapped] = value
	}

	for key, value := range f.payloadExtras {
		payload[key] = value
	}

	scheme := "http"
	if strings.ToLower(f.target.Scheme) == "forms" {
		scheme = "https"
	}

	host := f.target.Host
	if f.target.Port != 0 {
		host = fmt.Sprintf("%s:%d", host, f.target.Port)
	}

	u := url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   f.target.Path,
	}
	if u.Path == "" {
		u.Path = "/"
	}

	if f.method == "GET" {
		values := url.Values{}
		for key, value := range payload {
			values.Set(key, value)
		}
		for key, value := range f.params {
			values.Set(key, value)
		}
		u.RawQuery = values.Encode()
	}

	bodyPayload := ""
	if f.method != "GET" {
		values := url.Values{}
		for key, value := range payload {
			values.Set(key, value)
		}
		bodyPayload = values.Encode()
	}

	if f.method != "GET" && len(f.params) > 0 {
		values := url.Values{}
		for key, value := range f.params {
			values.Set(key, value)
		}
		u.RawQuery = values.Encode()
	}

	headers := map[string]string{
		"User-Agent": "Apprise",
		"Accept":     "*/*",
	}
	if f.method != "GET" {
		headers["Content-Type"] = "application/x-www-form-urlencoded"
	}
	for key, value := range f.headers {
		headers[key] = value
	}
	if f.target.User != "" {
		password := f.target.Password
		if !f.target.HasPassword {
			password = "None"
		}
		headers["Authorization"] = basicAuthHeader(f.target.User, password)
	}

	return RequestSpec{
		Method:  f.method,
		URL:     u.String(),
		Headers: headers,
		Body:    bodyPayload,
	}, nil
}

func init() {
	RegisterSchemaEntryOrdered(67, SchemaEntry{
		"attachment_support": true,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"attach-as": map[string]any{
					"default":  "file*",
					"map_to":   "attach_as",
					"name":     "Attach File As",
					"private":  false,
					"required": false,
					"type":     "string",
				},
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
				"method": map[string]any{
					"default":  "POST",
					"map_to":   "method",
					"name":     "Fetch Method",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"POST", "GET", "DELETE", "PUT", "HEAD", "PATCH"},
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
				"params": map[string]any{
					"map_to":   "params",
					"name":     "GET Params",
					"prefix":   "-",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"payload": map[string]any{
					"map_to":   "payload",
					"name":     "Payload Extras",
					"prefix":   ":",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{host}", "{schema}://{host}:{port}", "{schema}://{user}@{host}", "{schema}://{user}@{host}:{port}", "{schema}://{user}:{password}@{host}", "{schema}://{user}:{password}@{host}:{port}"},
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
					"values":   []string{"form", "forms"},
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
		"protocols": []string{"form"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"forms"},
		"service_name":     "Form",
		"service_url":      nil,
		"setup_url":        "https://appriseit.com/services/form/",
	})
}
