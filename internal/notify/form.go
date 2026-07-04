package notify

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/textproto"
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
	attachAs      string
	attachMulti   bool
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

	attachAs, attachMulti, err := parseFormAttachAs(target.Query["attach-as"])
	if err != nil {
		return nil, err
	}

	return &FormTarget{
		target:        target,
		method:        method,
		headers:       cloneMap(target.QueryAdd),
		params:        cloneMap(target.QueryDel),
		payloadExtras: payloadExtras,
		payloadMap:    payloadMap,
		attachAs:      attachAs,
		attachMulti:   attachMulti,
	}, nil
}

func (f *FormTarget) Send(body, title string, notifyType NotifyType) error {
	return f.SendWithAttachments(body, title, notifyType, nil)
}

func (f *FormTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	spec, err := f.BuildRequestWithAttachments(body, title, notifyType, attachments)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (f *FormTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	return f.BuildRequestWithAttachments(body, title, notifyType, nil)
}

func (f *FormTarget) BuildRequestWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) (RequestSpec, error) {
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
	contentType := "application/x-www-form-urlencoded"
	if f.method != "GET" {
		if len(attachments) > 0 {
			var err error
			bodyPayload, contentType, err = f.buildMultipartPayload(payload, attachments)
			if err != nil {
				return RequestSpec{}, err
			}
		} else {
			values := url.Values{}
			for key, value := range payload {
				values.Set(key, value)
			}
			bodyPayload = values.Encode()
		}
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

func (f *FormTarget) buildMultipartPayload(payload map[string]string, attachments []Attachment) (string, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range payload {
		if err := writer.WriteField(key, value); err != nil {
			return "", "", err
		}
	}
	for i, attachment := range attachments {
		fieldName := f.attachAs
		if f.attachMulti {
			fieldName = fmt.Sprintf(f.attachAs, i+1)
		}
		if strings.TrimSpace(fieldName) == "" {
			fieldName = "file"
		}
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeMultipartParam(fieldName), escapeMultipartParam(attachment.Name)))
		header.Set("Content-Type", attachment.MIMEType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return "", "", err
		}
		if _, err := part.Write(attachment.Data); err != nil {
			return "", "", err
		}
	}
	if err := writer.Close(); err != nil {
		return "", "", err
	}
	return body.String(), writer.FormDataContentType(), nil
}

func parseFormAttachAs(raw string) (string, bool, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "file%02d", true, nil
	}
	if strings.Count(value, "%02d") > 1 {
		return "", false, fmt.Errorf("attach-as supports only one placeholder")
	}
	if strings.Contains(value, "%02d") {
		return value, true, nil
	}

	wildcard := -1
	for i, ch := range value {
		if !strings.ContainsRune("*?+$:.%", ch) {
			continue
		}
		if wildcard >= 0 {
			return "", false, fmt.Errorf("attach-as supports only one wildcard")
		}
		wildcard = i
	}
	if wildcard < 0 {
		return value, false, nil
	}
	return value[:wildcard] + "%02d" + value[wildcard+1:], true, nil
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
