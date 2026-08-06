package notify

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

const (
	appriseMethodForm = "form"
	appriseMethodJSON = "json"
)

type AppriseTarget struct {
	target *ParsedURL

	method  string
	token   string
	path    string
	tags    []string
	headers map[string]string
}

// appriseTokenRe mirrors upstream's token regex.
var appriseTokenRe = regexp.MustCompile(`(?i)^[A-Z0-9_-]{1,128}$`)

func NewAppriseTarget(target *ParsedURL) (*AppriseTarget, error) {
	segments := splitPath(target.Path)
	if len(segments) == 0 {
		return nil, fmt.Errorf("missing token")
	}

	// Upstream validates the token's shape and raises; a URL whose last path
	// segment is punctuation names no configuration that can exist.
	token := segments[len(segments)-1]
	if !appriseTokenRe.MatchString(token) {
		return nil, fmt.Errorf("invalid apprise api token: %q", token)
	}
	pathSegments := segments[:len(segments)-1]
	pathPrefix := ""
	if len(pathSegments) > 0 {
		pathPrefix = "/" + strings.Join(pathSegments, "/")
	}

	method := appriseMethodForm
	if rawMethod, ok := target.Query["method"]; ok && rawMethod != "" {
		method = strings.ToLower(rawMethod)
	}
	if method != appriseMethodForm && method != appriseMethodJSON {
		return nil, fmt.Errorf("invalid method: %s", method)
	}

	tags := []string{}
	if rawTags, ok := target.Query["tags"]; ok && rawTags != "" {
		for _, tag := range strings.Split(rawTags, ",") {
			trimmed := strings.TrimSpace(tag)
			if trimmed != "" {
				tags = append(tags, trimmed)
			}
		}
	}
	sort.Strings(tags)

	return &AppriseTarget{
		target:  target,
		method:  method,
		token:   token,
		path:    pathPrefix,
		tags:    tags,
		headers: cloneMap(target.QueryAdd),
	}, nil
}

func (a *AppriseTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	return a.buildRequest(body, title, notifyType, nil)
}

func (a *AppriseTarget) buildRequest(body, title string, notifyType NotifyType, attachments []Attachment) (RequestSpec, error) {
	scheme := "http"
	if strings.ToLower(a.target.Scheme) == "apprises" {
		scheme = "https"
	}

	host := a.target.Host
	if a.target.Port != 0 {
		host = fmt.Sprintf("%s:%d", host, a.target.Port)
	}

	endpoint := fmt.Sprintf("%s/notify/%s", a.path, a.token)
	u := url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   endpoint,
	}

	headers := cloneMap(a.headers)
	headers["User-Agent"] = "Apprise"
	headers["Accept"] = "application/json"
	if a.target.HasUser {
		password := a.target.Password
		if !a.target.HasPassword {
			password = "None"
		}
		credentials := a.target.User + ":" + password
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials))
	}

	if a.method == appriseMethodForm {
		values := formFields{}
		values.Set("title", title)
		values.Set("body", body)
		values.Set("type", string(notifyType))
		values.Set("format", "text")
		for _, tag := range a.tags {
			values.Add("tag", tag)
		}

		if len(attachments) > 0 {
			// Form mode uploads the files, numbered fileNN.
			requestBody, contentType, err := appriseAPIFormAttachmentBody(values, attachments)
			if err != nil {
				return RequestSpec{}, err
			}
			headers["Content-Type"] = contentType

			return RequestSpec{
				Method:  "POST",
				URL:     u.String(),
				Headers: headers,
				Body:    requestBody,
			}, nil
		}

		headers["Content-Type"] = "application/x-www-form-urlencoded"
		return RequestSpec{
			Method:  "POST",
			URL:     u.String(),
			Headers: headers,
			Body:    values.Encode(),
		}, nil
	}

	payload := map[string]any{
		"title":  title,
		"body":   body,
		"type":   string(notifyType),
		"format": "text",
	}
	if len(a.tags) > 0 {
		payload["tag"] = a.tags
	}
	if len(attachments) > 0 {
		// JSON mode carries the files base64 encoded in the body.
		payload["attachments"] = attachmentsCustomJSONStyle(attachments)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}
	headers["Content-Type"] = "application/json"

	return RequestSpec{
		Method:  "POST",
		URL:     u.String(),
		Headers: headers,
		Body:    string(data),
	}, nil
}

func (a *AppriseTarget) Send(body, title string, notifyType NotifyType) error {
	return a.SendWithAttachments(body, title, notifyType, nil)
}

func (a *AppriseTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	spec, err := a.buildRequest(body, title, notifyType, attachments)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func splitPath(pathValue string) []string {
	if pathValue == "" {
		return nil
	}

	trimmed := strings.TrimLeft(pathValue, "/")
	if trimmed == "" {
		return nil
	}

	parts := splitPathDelims.Split(trimmed, -1)
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		decoded, err := url.PathUnescape(part)
		if err == nil {
			part = decoded
		}
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		segments = append(segments, part)
	}
	if len(segments) == 0 {
		return nil
	}
	return segments
}

var splitPathDelims = regexp.MustCompile(`[ \t\r\n,\\/]+`)
