package notify

import (
	"fmt"
	"net/url"
	"strings"
)

var formMethods = map[string]struct{}{
	"POST":    {},
	"GET":     {},
	"DELETE":  {},
	"PUT":     {},
	"HEAD":    {},
	"PATCH":   {},
	"UPDATE":  {},
	"OPTIONS": {},
}

type FormTarget struct {
	target        *ParsedURL
	method        string
	headers       map[string]string
	params        map[string]string
	payloadExtras map[string]string
	payloadMap    map[string]string

	// The URL's own ordering for payloadExtras and params.
	payloadExtraOrder []string
	paramOrder        []string
	attachAs          string
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
	payloadExtraOrder := target.QueryPayloadOrder
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
		target:            target,
		method:            method,
		headers:           cloneMap(target.QueryAdd),
		params:            cloneMap(target.QueryDel),
		payloadExtras:     payloadExtras,
		payloadExtraOrder: payloadExtraOrder,
		paramOrder:        target.QueryDelOrder,
		payloadMap:        payloadMap,
		// The URL argument is attach-as; attach_as is only the name it maps
		// to internally, and reading that spelling meant the documented one
		// did nothing.
		attachAs: strings.TrimSpace(target.Query["attach-as"]),
	}, nil
}

func (f *FormTarget) Send(body, title string, notifyType NotifyType) error {
	return f.SendWithAttachments(body, title, notifyType, nil)
}

func (f *FormTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	spec, err := f.buildRequest(body, title, notifyType, attachments)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (f *FormTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	return f.buildRequest(body, title, notifyType, nil)
}

func (f *FormTarget) buildRequest(body, title string, notifyType NotifyType, attachments []Attachment) (RequestSpec, error) {
	// Upstream walks these four in this order and the extras follow, which
	// is the order a receiver reads the fields off the wire in.
	base := []struct{ key, value string }{
		{"version", "1.0"},
		{"title", title},
		{"message", body},
		{"type", string(notifyType)},
	}

	payload := formFields{}
	for _, entry := range base {
		mapped := f.payloadMap[entry.key]
		if mapped == "" {
			continue
		}
		payload.Set(mapped, entry.value)
	}

	// In the order the URL named them, which is upstream's order too.
	for _, key := range orderedKeys(f.payloadExtraOrder, f.payloadExtras) {
		payload.Set(key, f.payloadExtras[key])
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
		query := payload.Clone()
		for _, key := range orderedKeys(f.paramOrder, f.params) {
			query.Set(key, f.params[key])
		}
		u.RawQuery = query.Encode()
	}

	bodyPayload := ""
	contentType := ""
	if f.method != "GET" {
		if len(attachments) > 0 {
			// Files turn the form-encoded body multipart; the field name
			// comes from ?attach_as=, numbered when it carries a wildcard.
			var err error
			bodyPayload, contentType, err = formAttachmentBody(payload, f.attachAs, attachments)
			if err != nil {
				return RequestSpec{}, err
			}
		} else {
			bodyPayload = payload.Encode()
		}
	}

	if f.method != "GET" && len(f.params) > 0 {
		query := formFields{}
		for _, key := range orderedKeys(f.paramOrder, f.params) {
			query.Set(key, f.params[key])
		}
		u.RawQuery = query.Encode()
	}

	headers := map[string]string{
		"User-Agent": "Apprise",
		"Accept":     "*/*",
	}
	if f.method != "GET" {
		// A multipart body carries its own boundary in the content type.
		if contentType == "" {
			contentType = "application/x-www-form-urlencoded"
		}
		headers["Content-Type"] = contentType
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
				"method": map[string]any{
					"default":  "POST",
					"map_to":   "method",
					"name":     "Fetch Method",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"POST", "GET", "DELETE", "PUT", "HEAD", "PATCH", "UPDATE", "OPTIONS"},
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
