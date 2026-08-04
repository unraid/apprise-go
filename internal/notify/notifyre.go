package notify

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// Notifyre pins its API version in the path.
const (
	notifyreSMSURL = "https://api.notifyre.com/20220711/sms/send"
	notifyreFaxURL = "https://api.notifyre.com/20220711/fax/send"

	notifyreModeSMS = "sms"
	notifyreModeFax = "fax"
)

var notifyreModes = []string{notifyreModeSMS, notifyreModeFax}

type NotifyreTarget struct {
	apikey   string
	targets  []string
	mode     string
	source   string
	campaign string
	template string
	ref      string
	hq       bool
	header   string
}

func NewNotifyreTarget(target *ParsedURL) (*NotifyreTarget, error) {
	apikey := strings.TrimSpace(target.Host)
	if apikey == "" {
		return nil, fmt.Errorf("missing api key")
	}

	entries := splitPath(target.Path)
	if to := strings.TrimSpace(target.Query["to"]); to != "" {
		entries = append(entries, parseDelimitedList(to)...)
	}

	targets := make([]string, 0, len(entries))
	for _, entry := range entries {
		if normalized, ok := normalizePhone(entry); ok {
			targets = append(targets, "+"+normalized)
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	source := ""
	if raw := strings.TrimSpace(target.Query["from"]); raw != "" {
		if normalized, ok := normalizePhone(raw); ok {
			source = "+" + normalized
		}
	}

	// A mode is matched by prefix, so notifyre://...?mode=f reaches fax.
	mode := notifyreModeSMS
	if raw := strings.ToLower(strings.TrimSpace(target.Query["mode"])); raw != "" {
		mode = ""
		for _, candidate := range notifyreModes {
			if strings.HasPrefix(candidate, raw) {
				mode = candidate
				break
			}
		}
		if mode == "" {
			return nil, fmt.Errorf("invalid mode: %s", target.Query["mode"])
		}
	}

	// The campaign name defaults to the application identifier.
	campaign := strings.TrimSpace(target.Query["campaign"])
	if campaign == "" {
		campaign = "Apprise"
	}

	return &NotifyreTarget{
		apikey:   apikey,
		targets:  targets,
		mode:     mode,
		source:   source,
		campaign: campaign,
		template: strings.TrimSpace(target.Query["template"]),
		ref:      strings.TrimSpace(target.Query["ref"]),
		hq:       parseBoolWithDefault(target.Query["hq"], true),
		header:   strings.TrimSpace(target.Query["header"]),
	}, nil
}

func (n *NotifyreTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	return n.buildRequest(body, title, notifyType, nil)
}

func (n *NotifyreTarget) buildRequest(body, title string, notifyType NotifyType, attachments []Attachment) (RequestSpec, error) {
	_ = notifyType

	if n.mode == notifyreModeFax {
		return n.buildFaxRequest(mergeTitleBody(title, body), attachments)
	}

	recipients := make([]any, 0, len(n.targets))
	for _, entry := range n.targets {
		recipients = append(recipients, map[string]any{
			"type":  "mobile_number",
			"value": entry,
		})
	}

	payload := map[string]any{
		"body":         mergeTitleBody(title, body),
		"recipients":   recipients,
		"from":         n.source,
		"campaignName": n.campaign,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    notifyreSMSURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
			"x-api-token":  n.apikey,
		},
		Body: string(data),
	}, nil
}

// buildFaxRequest sends the message as fax pages: the text becomes the cover
// page and each attachment follows it as a further page.
func (n *NotifyreTarget) buildFaxRequest(text string, attachments []Attachment) (RequestSpec, error) {
	documents := make([]any, 0, len(attachments)+1)
	if text != "" {
		documents = append(documents, map[string]any{
			"base64Str":   base64.StdEncoding.EncodeToString([]byte(text)),
			"contentType": "text/plain",
		})
	}

	for _, attachment := range attachments {
		documents = append(documents, map[string]any{
			"base64Str":   attachment.Base64(),
			"contentType": attachment.MimeType,
		})
	}

	if len(documents) == 0 {
		return RequestSpec{}, fmt.Errorf("fax has no content to send")
	}

	recipients := make([]any, 0, len(n.targets))
	for _, entry := range n.targets {
		recipients = append(recipients, map[string]any{
			"type":  "fax_number",
			"value": entry,
		})
	}

	payload := map[string]any{
		"templateName":    n.template,
		"recipients":      recipients,
		"sendFrom":        n.source,
		"isHighQuality":   n.hq,
		"clientReference": n.ref,
		"documents":       documents,
		"header":          n.header,
		"subject":         text,
		"campaignName":    n.campaign,
		"scheduledDate":   nil,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    notifyreFaxURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
			"x-api-token":  n.apikey,
		},
		Body: string(data),
	}, nil
}

func (n *NotifyreTarget) Send(body, title string, notifyType NotifyType) error {
	return n.SendWithAttachments(body, title, notifyType, nil)
}

// SendWithAttachments only carries files in fax mode; upstream warns and drops
// them for SMS rather than failing the send.
func (n *NotifyreTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	spec, err := n.buildRequest(body, title, notifyType, attachments)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func init() {
	RegisterSchemaEntryOrdered(152, SchemaEntry{
		"attachment_support": true,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"campaign": map[string]any{
					"map_to":   "campaign",
					"name":     "Campaign Name",
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
				"from": map[string]any{
					"map_to":   "source",
					"name":     "Source Phone No",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"header": map[string]any{
					"map_to":   "header",
					"name":     "Fax Header",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"hq": map[string]any{
					"default":  true,
					"map_to":   "hq",
					"name":     "High Quality",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"mode": map[string]any{
					"default":  "sms",
					"map_to":   "mode",
					"name":     "Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"sms", "fax"},
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
				"ref": map[string]any{
					"map_to":   "ref",
					"name":     "Client Reference",
					"private":  false,
					"required": false,
					"type":     "string",
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
				"template": map[string]any{
					"map_to":   "template",
					"name":     "Template Name",
					"private":  false,
					"required": false,
					"type":     "string",
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
			"templates": []string{"{schema}://{apikey}/{targets}"},
			"tokens": map[string]any{
				"apikey": map[string]any{
					"map_to":   "apikey",
					"name":     "API Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "notifyre",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"notifyre"},
				},
				"target_phone": map[string]any{
					"map_to":   "targets",
					"name":     "Target Phone No",
					"prefix":   "+",
					"private":  false,
					"regex":    []string{"^[0-9\\s)(+-]+$", "i"},
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_phone"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
			},
		},
		"enabled": true,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"protocols":        nil,
		"secure_protocols": []string{"notifyre"},
		"service_name":     "Notifyre",
		"service_url":      "https://notifyre.com/",
		"setup_url":        "https://appriseit.com/services/notifyre/",
	})
}
