package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const pushbulletURL = "https://api.pushbullet.com/v2/pushes"

// Uploading is a three step exchange: ask where to put the file, post it
// there, then push a message referencing the resulting URL.
const pushbulletUploadRequestURL = "https://api.pushbullet.com/v2/upload-request"

type PushbulletTarget struct {
	accessToken string
	target      string
}

func NewPushbulletTarget(target *ParsedURL) (*PushbulletTarget, error) {
	accessToken := target.Host
	if accessToken == "" {
		return nil, fmt.Errorf("missing access token")
	}

	targets := splitPath(target.Path)
	if rawTargets, ok := target.Query["to"]; ok && rawTargets != "" {
		targets = append(targets, splitList(rawTargets)...)
	}

	selected := ""
	for _, entry := range targets {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		selected = trimmed
		break
	}

	return &PushbulletTarget{
		accessToken: accessToken,
		target:      selected,
	}, nil
}

func (p *PushbulletTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload := map[string]any{
		"type":  "note",
		"title": title,
		"body":  body,
	}

	p.applyRecipient(payload)

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Accept":        "*/*",
		"Content-Type":  "application/json",
		"Authorization": basicAuthHeader(p.accessToken, ""),
	}

	_ = notifyType

	return RequestSpec{
		Method:  "POST",
		URL:     pushbulletURL,
		Headers: headers,
		Body:    string(data),
	}, nil
}

func (p *PushbulletTarget) Send(body, title string, notifyType NotifyType) error {
	return p.SendWithAttachments(body, title, notifyType, nil)
}

func (p *PushbulletTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	// Every file is uploaded before anything is pushed, so a failed upload
	// does not leave a message referencing a file that is not there.
	pushes := make([]map[string]any, 0, len(attachments))
	for index, attachment := range attachments {
		push, err := p.uploadAttachment(attachment, index)
		if err != nil {
			return err
		}
		pushes = append(pushes, push)
	}

	spec, err := p.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}
	if err := SendRequest(spec); err != nil {
		return err
	}

	for _, push := range pushes {
		spec, err := p.pushSpec(push)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

// uploadAttachment asks Pushbullet where to put a file, posts it there, and
// returns the push describing it.
func (p *PushbulletTarget) uploadAttachment(attachment Attachment, index int) (map[string]any, error) {
	name := attachment.FileName(index, ".dat")

	request, err := json.Marshal(map[string]any{
		"file_name": name,
		"file_type": attachment.MimeType,
	})
	if err != nil {
		return nil, err
	}

	var upload struct {
		FileName  string `json:"file_name"`
		FileType  string `json:"file_type"`
		FileURL   string `json:"file_url"`
		UploadURL string `json:"upload_url"`
	}
	if err := doJSONRequest(RequestSpec{
		Method:  "POST",
		URL:     pushbulletUploadRequestURL,
		Headers: p.headers("application/json"),
		Body:    string(request),
	}, &upload); err != nil {
		return nil, err
	}
	if upload.UploadURL == "" || upload.FileURL == "" {
		return nil, fmt.Errorf("pushbullet upload request returned no url")
	}

	// The upload itself is multipart and carries no auth of its own.
	uploadBody, contentType, err := singleFileAttachmentBody(
		url.Values{}, "file", Attachment{
			Name:     upload.FileName,
			MimeType: upload.FileType,
			Data:     attachment.Data,
		}, false)
	if err != nil {
		return nil, err
	}

	if err := SendRequest(RequestSpec{
		Method:  "POST",
		URL:     upload.UploadURL,
		Headers: p.headers(contentType),
		Body:    uploadBody,
	}); err != nil {
		return nil, err
	}

	push := map[string]any{
		"type":      "file",
		"file_name": upload.FileName,
		"file_type": upload.FileType,
		"file_url":  upload.FileURL,
	}
	// An image is shown inline rather than as a link.
	if strings.HasPrefix(strings.ToLower(upload.FileType), "image/") {
		push["image_url"] = upload.FileURL
	}

	return push, nil
}

func (p *PushbulletTarget) pushSpec(push map[string]any) (RequestSpec, error) {
	payload := map[string]any{}
	for key, value := range push {
		payload[key] = value
	}
	p.applyRecipient(payload)

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method:  "POST",
		URL:     pushbulletURL,
		Headers: p.headers("application/json"),
		Body:    string(data),
	}, nil
}

func (p *PushbulletTarget) headers(contentType string) map[string]string {
	headers := map[string]string{
		"User-Agent":    "Apprise",
		"Accept":        "*/*",
		"Authorization": basicAuthHeader(p.accessToken, ""),
	}
	if contentType != "" {
		headers["Content-Type"] = contentType
	}

	return headers
}

// applyRecipient names who a push is for, however the target was written.
func (p *PushbulletTarget) applyRecipient(payload map[string]any) {
	if p.target == "" {
		return
	}

	switch {
	case strings.HasPrefix(p.target, "#") && len(p.target) > 1:
		payload["channel_tag"] = p.target[1:]
	case looksLikeEmail(p.target):
		payload["email"] = p.target
	default:
		payload["device_iden"] = p.target
	}
}

func looksLikeEmail(value string) bool {
	at := strings.Index(value, "@")
	if at <= 0 || at == len(value)-1 {
		return false
	}
	return strings.Contains(value[at+1:], ".")
}

func init() {
	RegisterSchemaEntryOrdered(113, SchemaEntry{
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
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{accesstoken}", "{schema}://{accesstoken}/{targets}"},
			"tokens": map[string]any{
				"accesstoken": map[string]any{
					"map_to":   "accesstoken",
					"name":     "Access Token",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "pbul",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"pbul"},
				},
				"target_channel": map[string]any{
					"map_to":   "targets",
					"name":     "Target Channel",
					"prefix":   "#",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_device": map[string]any{
					"map_to":   "targets",
					"name":     "Target Device",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_email": map[string]any{
					"map_to":   "targets",
					"name":     "Target Email",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_channel", "target_device", "target_email"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
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
		"secure_protocols": []string{"pbul"},
		"service_name":     "Pushbullet",
		"service_url":      "https://www.pushbullet.com/",
		"setup_url":        "https://appriseit.com/services/pushbullet/",
	})
}
