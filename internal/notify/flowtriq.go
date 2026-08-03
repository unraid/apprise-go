package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

// flowtriqSeverity maps an Apprise notification type onto the severity
// Flowtriq expects; failures are reported as critical rather than failure.
var flowtriqSeverity = map[NotifyType]string{
	NotifyInfo:    "info",
	NotifySuccess: "success",
	NotifyWarning: "warning",
	NotifyFailure: "critical",
}

type FlowtriqTarget struct {
	apikey      string
	host        string
	port        int
	secure      bool
	webhookPath string
}

func NewFlowtriqTarget(target *ParsedURL) (*FlowtriqTarget, error) {
	apikey := strings.TrimSpace(target.User)
	if apikey == "" {
		return nil, fmt.Errorf("missing api key")
	}

	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	webhookPath := strings.Trim(strings.TrimSpace(target.Path), "/")
	if webhookPath == "" {
		return nil, fmt.Errorf("missing webhook path")
	}

	return &FlowtriqTarget{
		apikey:      apikey,
		host:        host,
		port:        target.Port,
		secure:      strings.EqualFold(target.Scheme, "flowtriqs"),
		webhookPath: webhookPath,
	}, nil
}

func (f *FlowtriqTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	severity, ok := flowtriqSeverity[notifyType]
	if !ok {
		severity = "info"
	}

	payload := map[string]any{
		"title":    title,
		"body":     body,
		"severity": severity,
		"type":     string(notifyType),
		"source":   "apprise",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	scheme := "http"
	if f.secure {
		scheme = "https"
	}

	url := fmt.Sprintf("%s://%s", scheme, f.host)
	if f.port > 0 {
		url += fmt.Sprintf(":%d", f.port)
	}
	url += "/" + f.webhookPath

	return RequestSpec{
		Method: "POST",
		URL:    url,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
			"X-API-Key":    f.apikey,
		},
		Body: string(data),
	}, nil
}

func (f *FlowtriqTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := f.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func init() {
	RegisterSchemaEntryOrdered(143, SchemaEntry{
		"attachment_support": false,
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
			"templates": []string{"{schema}://{apikey}@{host}/{path}/", "{schema}://{apikey}@{host}:{port}/{path}/"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": true,
					"type":     "string",
				},
				"path": map[string]any{
					"map_to":   "webhook_path",
					"name":     "Webhook Path",
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
					"values":   []string{"flowtriq", "flowtriqs"},
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"flowtriq"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"flowtriqs"},
		"service_name":     "Flowtriq",
		"service_url":      "https://flowtriq.com",
		"setup_url":        "https://appriseit.com/services/flowtriq/",
	})
}
