package notify

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const nextcloudDefaultVersion = 21
const nextcloudDefaultAppDesc = "Apprise Notifications"

type NextcloudTarget struct {
	host      string
	port      int
	secure    bool
	user      string
	password  string
	targets   []string
	version   int
	urlPrefix string
	headers   map[string]string
}

func NewNextcloudTarget(target *ParsedURL) (*NextcloudTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	targets := []string{}
	for _, entry := range splitPath(target.Path) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.HasPrefix(entry, "@") {
			entry = strings.TrimPrefix(entry, "@")
		}
		if entry == "" {
			continue
		}
		if isNextcloudGroup(entry) {
			continue
		}
		targets = append(targets, entry)
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	version := nextcloudDefaultVersion
	if raw := strings.TrimSpace(target.Query["version"]); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			version = parsed
		}
	}

	urlPrefix := strings.Trim(target.Query["url_prefix"], "/")

	return &NextcloudTarget{
		host:      host,
		port:      target.Port,
		secure:    strings.EqualFold(target.Scheme, "nclouds"),
		user:      strings.TrimSpace(target.User),
		password:  target.Password,
		targets:   targets,
		version:   version,
		urlPrefix: urlPrefix,
		headers:   cloneMap(target.QueryAdd),
	}, nil
}

func (n *NextcloudTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(n.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	spec := n.buildSpec(body, title, n.targets[0])
	_ = notifyType
	return spec, nil
}

func (n *NextcloudTarget) Send(body, title string, notifyType NotifyType) error {
	if len(n.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	for _, target := range n.targets {
		spec := n.buildSpec(body, title, target)
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = notifyType
	return nil
}

func (n *NextcloudTarget) buildSpec(body, title, target string) RequestSpec {
	values := url.Values{}
	if title == "" {
		values.Set("shortMessage", nextcloudDefaultAppDesc)
	} else {
		values.Set("shortMessage", title)
	}
	if strings.TrimSpace(body) != "" {
		values.Set("longMessage", body)
	}

	headers := map[string]string{
		"User-Agent":     "Apprise",
		"OCS-APIREQUEST": "true",
		"Accept":         "application/json",
		"Content-Type":   "application/x-www-form-urlencoded",
	}
	for key, value := range n.headers {
		headers[key] = value
	}
	if n.user != "" {
		headers["Authorization"] = basicAuthHeader(n.user, n.password)
	}

	return RequestSpec{
		Method:  "POST",
		URL:     n.buildURL(target),
		Headers: headers,
		Body:    values.Encode(),
	}
}

func (n *NextcloudTarget) buildURL(target string) string {
	scheme := "http"
	if n.secure {
		scheme = "https"
	}

	host := n.host
	if n.port > 0 {
		host = fmt.Sprintf("%s:%d", host, n.port)
	}

	base := fmt.Sprintf("%s://%s", scheme, host)
	if n.urlPrefix != "" {
		base = base + "/" + n.urlPrefix
	}

	escaped := url.PathEscape(target)
	if n.version < 21 {
		return fmt.Sprintf("%s/ocs/v2.php/apps/admin_notifications/api/v1/notifications/%s", base, escaped)
	}
	return fmt.Sprintf("%s/ocs/v2.php/apps/notifications/api/v2/admin_notifications/%s", base, escaped)
}

func isNextcloudGroup(entry string) bool {
	trimmed := strings.TrimSpace(entry)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(strings.TrimPrefix(trimmed, "#"))
	switch lower {
	case "all", "everyone", "*":
		return true
	}
	return strings.HasPrefix(trimmed, "#")
}

func init() {
	RegisterSchemaEntryOrdered(46, SchemaEntry{
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
				"to": map[string]any{
					"alias_of": "targets",
					"delim":    []string{",", " "},
				},
				"tz": map[string]any{
					"default":  nil,
					"map_to":   "tz",
					"name":     "Timezone",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"url_prefix": map[string]any{
					"map_to":   "url_prefix",
					"name":     "URL Prefix",
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
				"version": map[string]any{
					"default":  21,
					"map_to":   "version",
					"min":      1,
					"name":     "Version",
					"private":  false,
					"required": false,
					"type":     "int",
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
			"templates": []string{"{schema}://{host}/{targets}", "{schema}://{host}:{port}/{targets}", "{schema}://{user}:{password}@{host}/{targets}", "{schema}://{user}:{password}@{host}:{port}/{targets}"},
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
					"values":   []string{"ncloud", "nclouds"},
				},
				"target_group": map[string]any{
					"map_to":   "targets",
					"name":     "Target Group",
					"prefix":   "#",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_user": map[string]any{
					"map_to":   "targets",
					"name":     "Target User",
					"prefix":   "@",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_group", "target_user"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": true,
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
		"protocols": []string{"ncloud"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"nclouds"},
		"service_name":     "Nextcloud",
		"service_url":      "https://nextcloud.com/",
		"setup_url":        "https://appriseit.com/services/nextcloud/",
	})
}
