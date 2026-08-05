package notify

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type HumHubTarget struct {
	host     string
	port     int
	secure   bool
	user     string
	password string
	targets  []string
}

func NewHumHubTarget(target *ParsedURL) (*HumHubTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	user := strings.TrimSpace(target.User)
	if user == "" {
		return nil, fmt.Errorf("missing token or username")
	}

	targets := splitPath(target.Path)
	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		targets = append(targets, parseDelimitedList(to)...)
	}
	// Container IDs are positive integers. Upstream drops anything else with a
	// warning and fails only when nothing usable is left, so a URL naming one
	// good container and one typo still posts to the good one -- and the
	// canonical form of each id is the parsed integer, not the text.
	valid := make([]string, 0, len(targets))
	for _, entry := range targets {
		id, err := strconv.Atoi(strings.TrimSpace(entry))
		if err != nil || id <= 0 {
			continue
		}
		valid = append(valid, strconv.Itoa(id))
	}
	targets = valid

	if len(targets) == 0 {
		return nil, fmt.Errorf("missing container ids")
	}

	return &HumHubTarget{
		host:     host,
		port:     target.Port,
		secure:   strings.EqualFold(target.Scheme, "humhubs"),
		user:     user,
		password: target.Password,
		targets:  targets,
	}, nil
}

func (h *HumHubTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	specs, err := h.buildRequests(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}

	return specs[0], nil
}

func (h *HumHubTarget) Send(body, title string, notifyType NotifyType) error {
	return h.SendWithAttachments(body, title, notifyType, nil)
}

// SendWithAttachments creates the post first, then uploads each file to it.
// The upload URL names the post, so the id has to come back from the create
// before anything can be attached.
func (h *HumHubTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	// Upstream keeps going after a failed target; see sendOutcome.
	var outcome sendOutcome
	specs, err := h.buildRequests(body, title, notifyType)
	if err != nil {
		return err
	}

	for _, spec := range specs {
		if len(attachments) == 0 {
			outcome.record(SendRequest(spec))
			continue
		}

		var created struct {
			ID json.Number `json:"id"`
		}
		if err := doJSONRequest(spec, &created); err != nil {
			return err
		}
		postID := created.ID.String()
		if postID == "" {
			return fmt.Errorf("humhub post creation returned no id")
		}

		uploadURL := fmt.Sprintf("%s/api/v1/post/%s/upload-files", h.baseURL(), postID)
		for _, attachment := range attachments {
			// HumHub is handed a filename and a handle with no type, so the
			// part carries no content type.
			requestBody, contentType, err := singleFileAttachmentBody(
				formFields{}, "files[]", attachment, false)
			if err != nil {
				return err
			}

			uploadHeaders := map[string]string{}
			for key, value := range spec.Headers {
				uploadHeaders[key] = value
			}
			uploadHeaders["Content-Type"] = contentType

			if err := SendRequest(RequestSpec{
				Method:  "POST",
				URL:     uploadURL,
				Headers: uploadHeaders,
				Body:    requestBody,
			}); err != nil {
				return err
			}
		}
	}

	return outcome.err()
}

func (h *HumHubTarget) baseURL() string {
	scheme := "http"
	if h.secure {
		scheme = "https"
	}
	base := fmt.Sprintf("%s://%s", scheme, h.host)
	if h.port > 0 {
		base += fmt.Sprintf(":%d", h.port)
	}

	return base
}

// buildRequests posts once per container, since HumHub addresses a single
// container per call.
func (h *HumHubTarget) buildRequests(body, title string, notifyType NotifyType) ([]RequestSpec, error) {
	_ = notifyType

	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/json",
	}
	// A password means basic auth; otherwise the user field is a bearer token.
	if h.password != "" {
		headers["Authorization"] = basicAuthHeader(h.user, h.password)
	} else {
		headers["Authorization"] = "Bearer " + h.user
	}

	data, err := json.Marshal(map[string]any{
		"data": map[string]any{"message": mergeTitleBody(title, body)},
	})
	if err != nil {
		return nil, err
	}

	scheme := "http"
	if h.secure {
		scheme = "https"
	}
	base := fmt.Sprintf("%s://%s", scheme, h.host)
	if h.port > 0 {
		base += fmt.Sprintf(":%d", h.port)
	}

	specs := make([]RequestSpec, 0, len(h.targets))
	for _, container := range h.targets {
		specs = append(specs, RequestSpec{
			Method:  "POST",
			URL:     fmt.Sprintf("%s/api/v1/post/container/%s", base, container),
			Headers: headers,
			Body:    string(data),
		})
	}

	return specs, nil
}

func init() {
	RegisterSchemaEntryOrdered(149, SchemaEntry{
		"attachment_support": true,
		"category":           "native",
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
				"optional": map[string]any{
					"default":  false,
					"map_to":   "optional",
					"name":     "Optional Service",
					"private":  false,
					"required": false,
					"type":     "bool",
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
				"redirect": map[string]any{
					"default":  true,
					"map_to":   "redirect",
					"name":     "Follow Redirects",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"retry": map[string]any{
					"default":  0,
					"map_to":   "retry",
					"max":      10,
					"min":      0,
					"name":     "Service Retry",
					"private":  false,
					"required": false,
					"type":     "int",
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
				"verify": map[string]any{
					"default":  true,
					"map_to":   "verify",
					"name":     "Verify SSL",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"wait": map[string]any{
					"default":  0.0,
					"map_to":   "wait",
					"max":      20.0,
					"min":      0.0,
					"name":     "Inter-Retry Wait",
					"private":  false,
					"required": false,
					"type":     "float",
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{user}@{host}/{targets}", "{schema}://{user}@{host}:{port}/{targets}", "{schema}://{user}:{password}@{host}/{targets}", "{schema}://{user}:{password}@{host}:{port}/{targets}"},
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
					"values":   []string{"humhub", "humhubs"},
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []any{},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"user": map[string]any{
					"map_to":   "user",
					"name":     "Token or Username",
					"private":  true,
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"humhub"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"humhubs"},
		"service_name":     "HumHub",
		"service_url":      "https://www.humhub.com/",
		"setup_url":        "https://appriseit.com/services/humhub/",
	})
}
