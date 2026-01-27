package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	workflowsVersionLegacy = "2016-06-01"
	workflowsVersionPA     = "2022-03-01-preview"
	workflowsAdaptiveVer   = "1.4"
)

var workflowsWorkflowRe = regexp.MustCompile(`(?i)^[A-Z0-9_-]+$`)
var workflowsSignatureRe = regexp.MustCompile(`(?i)^[a-z0-9_-]+$`)
var workflowsTemplateTokenRe = regexp.MustCompile(`(?i)\{\{\s*([a-z0-9_]+)\s*\}\}`)

type WorkflowsTarget struct {
	host           string
	port           int
	workflowID     string
	signature      string
	includeImage   bool
	powerAutomate  bool
	wrap           bool
	apiVersion     string
	templatePath   string
	templateTokens map[string]string
}

func NewWorkflowsTarget(target *ParsedURL) (*WorkflowsTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	entries := splitPath(target.Path)

	workflowID := strings.TrimSpace(target.Query["workflow"])
	if workflowID == "" {
		workflowID = strings.TrimSpace(target.Query["id"])
	}
	if workflowID == "" && len(entries) > 0 {
		workflowID = entries[0]
		entries = entries[1:]
	}

	signature := strings.TrimSpace(target.Query["signature"])
	if signature == "" {
		signature = strings.TrimSpace(target.Query["sig"])
	}
	if signature == "" && len(entries) > 0 {
		signature = entries[0]
	}

	if workflowID == "" || !workflowsWorkflowRe.MatchString(workflowID) {
		return nil, fmt.Errorf("invalid workflow")
	}
	if signature == "" || !workflowsSignatureRe.MatchString(signature) {
		return nil, fmt.Errorf("invalid signature")
	}

	includeImage := parseBoolWithDefault(target.Query["image"], true)

	powerAutomate := parseBoolWithDefault(target.Query["powerautomate"], false)
	if raw := strings.TrimSpace(target.Query["pa"]); raw != "" {
		powerAutomate = parseBoolWithDefault(raw, powerAutomate)
	}

	wrap := parseBoolWithDefault(target.Query["wrap"], true)

	apiVersion := strings.TrimSpace(target.Query["api-version"])
	if apiVersion == "" {
		apiVersion = strings.TrimSpace(target.Query["ver"])
	}
	if apiVersion == "" {
		if powerAutomate {
			apiVersion = workflowsVersionPA
		} else {
			apiVersion = workflowsVersionLegacy
		}
	}

	templatePath := strings.TrimSpace(target.Query["template"])

	templateTokens := map[string]string{}
	for key, value := range target.QueryPayload {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		templateTokens[key] = value
	}

	return &WorkflowsTarget{
		host:           host,
		port:           target.Port,
		workflowID:     workflowID,
		signature:      signature,
		includeImage:   includeImage,
		powerAutomate:  powerAutomate,
		wrap:           wrap,
		apiVersion:     apiVersion,
		templatePath:   templatePath,
		templateTokens: templateTokens,
	}, nil
}

func (w *WorkflowsTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload, err := w.buildPayload(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	requestURL := w.buildURL()

	return RequestSpec{
		Method: "POST",
		URL:    requestURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/json",
		},
		Body: string(data),
	}, nil
}

func (w *WorkflowsTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := w.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (w *WorkflowsTarget) buildURL() string {
	base := fmt.Sprintf("https://%s", w.host)
	if w.port > 0 {
		base += fmt.Sprintf(":%d", w.port)
	}

	path := ""
	if w.powerAutomate {
		path = "/powerautomate/automations/direct"
	}
	path += fmt.Sprintf("/workflows/%s/triggers/manual/paths/invoke", w.workflowID)

	query := url.Values{}
	query.Set("api-version", w.apiVersion)
	query.Set("sp", "/triggers/manual/run")
	query.Set("sv", "1.0")
	query.Set("sig", w.signature)

	return base + path + "?" + query.Encode()
}

func (w *WorkflowsTarget) buildPayload(body, title string, notifyType NotifyType) (map[string]any, error) {
	if strings.TrimSpace(w.templatePath) != "" {
		payload, err := w.buildTemplatePayload(body, title, notifyType)
		if err != nil {
			return nil, err
		}
		return payload, nil
	}

	bodyContent := []map[string]any{}
	if w.includeImage {
		bodyContent = append(bodyContent, map[string]any{
			"type":    "Image",
			"url":     appriseImageURL(notifyType, "32x32"),
			"height":  "32px",
			"altText": string(notifyType),
		})
	}

	if title != "" {
		bodyContent = append(bodyContent, map[string]any{
			"type":   "TextBlock",
			"text":   title,
			"style":  "heading",
			"weight": "Bolder",
			"size":   "Large",
			"id":     "title",
		})
	}

	bodyContent = append(bodyContent, map[string]any{
		"type":  "TextBlock",
		"text":  body,
		"style": "default",
		"wrap":  w.wrap,
		"id":    "body",
	})

	return map[string]any{
		"type": "message",
		"attachments": []map[string]any{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"contentUrl":  nil,
				"content": map[string]any{
					"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
					"type":    "AdaptiveCard",
					"version": workflowsAdaptiveVer,
					"body":    bodyContent,
					"msteams": map[string]any{
						"width": "full",
					},
				},
			},
		},
	}, nil
}

func (w *WorkflowsTarget) buildTemplatePayload(body, title string, notifyType NotifyType) (map[string]any, error) {
	path := strings.TrimSpace(w.templatePath)
	if strings.HasPrefix(path, "file://") {
		path = strings.TrimPrefix(path, "file://")
	}
	if path != "" && !filepath.IsAbs(path) {
		if moduleRoot, ok := findModuleRoot(); ok {
			path = filepath.Join(moduleRoot, path)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	tokens := map[string]string{}
	for key, value := range w.templateTokens {
		tokens[key] = value
	}
	tokens["app_body"] = body
	tokens["app_title"] = title
	tokens["app_type"] = string(notifyType)
	tokens["app_id"] = "Apprise"
	tokens["app_desc"] = "Apprise Notifications"
	tokens["app_color"] = appriseColor(notifyType)
	tokens["app_image_url"] = appriseImageURL(notifyType, "32x32")
	tokens["app_url"] = appriseAppURL
	tokens["app_mode"] = "json"

	rendered := applyTemplateTokens(string(data), tokens)

	var payload map[string]any
	if err := json.Unmarshal([]byte(rendered), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func applyTemplateTokens(template string, tokens map[string]string) string {
	if template == "" || len(tokens) == 0 {
		return template
	}
	lookup := map[string]string{}
	for key, value := range tokens {
		lookup[strings.ToLower(key)] = value
	}
	return workflowsTemplateTokenRe.ReplaceAllStringFunc(template, func(match string) string {
		matches := workflowsTemplateTokenRe.FindStringSubmatch(match)
		if len(matches) < 2 {
			return match
		}
		key := strings.ToLower(matches[1])
		value, ok := lookup[key]
		if !ok {
			return match
		}
		encoded, err := json.Marshal(value)
		if err != nil || len(encoded) < 2 {
			return value
		}
		return string(encoded[1 : len(encoded)-1])
	})
}

func init() {
	RegisterSchemaEntryOrdered(92, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"api-version": map[string]any{
					"alias_of": "ver",
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
					"default":  "markdown",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"id": map[string]any{
					"alias_of": "workflow",
				},
				"image": map[string]any{
					"default":  true,
					"map_to":   "include_image",
					"name":     "Include Image",
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
				"pa": map[string]any{
					"default":  false,
					"map_to":   "power_automate",
					"name":     "Use Power Automate URL",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"powerautomate": map[string]any{
					"alias_of": "pa",
				},
				"rto": map[string]any{
					"default":  4,
					"map_to":   "rto",
					"name":     "Socket Read Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"sig": map[string]any{
					"alias_of": "signature",
				},
				"store": map[string]any{
					"default":  true,
					"map_to":   "store",
					"name":     "Persistent Storage",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"template": map[string]any{
					"map_to":   "template",
					"name":     "Template Path",
					"private":  true,
					"required": false,
					"type":     "string",
				},
				"tz": map[string]any{
					"default":  nil,
					"map_to":   "tz",
					"name":     "Timezone",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"ver": map[string]any{
					"map_to":   "version",
					"name":     "API Version",
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
				"wrap": map[string]any{
					"default":  true,
					"map_to":   "wrap",
					"name":     "Wrap Text",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
			},
			"kwargs": map[string]any{
				"tokens": map[string]any{
					"map_to":   "tokens",
					"name":     "Template Tokens",
					"prefix":   ":",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{host}/{workflow}/{signature}", "{schema}://{host}:{port}/{workflow}/{signature}"},
			"tokens": map[string]any{
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": true,
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
					"values":   []string{"workflow", "workflows"},
				},
				"signature": map[string]any{
					"map_to":   "signature",
					"name":     "Signature",
					"private":  true,
					"regex":    []string{"^[a-z0-9_-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"workflow": map[string]any{
					"map_to":   "workflow",
					"name":     "Workflow ID",
					"private":  true,
					"regex":    []string{"^[A-Z0-9_-]+$", "i"},
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": nil,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"workflow", "workflows"},
		"service_name":     "Power Automate / Workflows (for MSTeams)",
		"service_url":      "https://www.microsoft.com/power-platform/products/power-automate",
		"setup_url":        "https://appriseit.com/services/workflows/",
	})
}
