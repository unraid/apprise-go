package notify

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultMaxAttachmentBytes is the default cap used when fetching remote attachments.
const DefaultMaxAttachmentBytes int64 = 25 * 1024 * 1024

var attachmentHTTPClient = &http.Client{Timeout: 30 * time.Second}

type Attachment struct {
	Source   string
	Name     string
	MIMEType string
	Data     []byte
}

func ParseAttachments(rawValues []string) ([]Attachment, error) {
	return ParseAttachmentsWithMaxBytes(rawValues, DefaultMaxAttachmentBytes)
}

// ParseAttachmentsWithMaxBytes parses attachments using maxBytes for each remote attachment fetch.
func ParseAttachmentsWithMaxBytes(rawValues []string, maxBytes int64) ([]Attachment, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("maximum attachment size must be positive")
	}
	attachments := make([]Attachment, 0, len(rawValues))
	for _, raw := range rawValues {
		attachment, err := ParseAttachmentWithMaxBytes(raw, maxBytes)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}

func ParseAttachment(raw string) (Attachment, error) {
	return ParseAttachmentWithMaxBytes(raw, DefaultMaxAttachmentBytes)
}

// ParseAttachmentWithMaxBytes parses an attachment using maxBytes for a remote attachment fetch.
func ParseAttachmentWithMaxBytes(raw string, maxBytes int64) (Attachment, error) {
	source := strings.TrimSpace(raw)
	if source == "" {
		return Attachment{}, fmt.Errorf("empty attachment")
	}
	if maxBytes <= 0 {
		return Attachment{}, fmt.Errorf("maximum attachment size must be positive")
	}

	location, params := splitAttachmentParams(source)
	name := strings.TrimSpace(params.Get("name"))
	mimeType := strings.TrimSpace(params.Get("mime"))
	if mimeType == "" {
		mimeType = strings.TrimSpace(params.Get("mimetype"))
	}

	var data []byte
	var err error
	switch strings.ToLower(attachmentScheme(location)) {
	case "http", "https":
		data, err = readHTTPAttachment(location, maxBytes)
	case "file":
		data, location, err = readFileURLAttachment(location)
	default:
		// #nosec G304 -- attachment paths are explicitly supplied by the caller.
		data, err = os.ReadFile(location)
	}
	if err != nil {
		return Attachment{}, err
	}

	if name == "" {
		name = filepath.Base(location)
	}
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "apprise-attachment"
	}

	if mimeType == "" {
		mimeType = detectMIMEType(name, data)
	}

	return Attachment{
		Source:   source,
		Name:     name,
		MIMEType: mimeType,
		Data:     data,
	}, nil
}

func (a Attachment) Base64() string {
	return base64.StdEncoding.EncodeToString(a.Data)
}

func attachmentPayloads(attachments []Attachment) []map[string]any {
	if len(attachments) == 0 {
		return []map[string]any{}
	}
	payloads := make([]map[string]any, 0, len(attachments))
	for i, attachment := range attachments {
		filename := attachment.Name
		if strings.TrimSpace(filename) == "" {
			filename = fmt.Sprintf("file%03d.dat", i+1)
		}
		payloads = append(payloads, map[string]any{
			"filename": filename,
			"base64":   attachment.Base64(),
			"mimetype": attachment.MIMEType,
		})
	}
	return payloads
}

func splitAttachmentParams(raw string) (string, url.Values) {
	parsed, err := url.Parse(raw)
	if err == nil && parsed.Scheme != "" {
		params := parsed.Query()
		parsed.RawQuery = ""
		return parsed.String(), params
	}

	parts := strings.SplitN(raw, "?", 2)
	if len(parts) == 1 {
		return raw, url.Values{}
	}
	params, err := url.ParseQuery(parts[1])
	if err != nil {
		return parts[0], url.Values{}
	}
	return parts[0], params
}

func attachmentScheme(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Scheme
}

func readHTTPAttachment(raw string, maxBytes int64) ([]byte, error) {
	return readHTTPAttachmentWithClient(raw, attachmentHTTPClient, maxBytes)
}

func readHTTPAttachmentWithClient(raw string, client *http.Client, maxBytes int64) ([]byte, error) {
	// Attachment URLs are explicit caller input, matching Python Apprise's
	// trusted-URL behavior. Do not pass untrusted end-user URLs here without
	// applying application-level SSRF policy before calling WithAttachments.
	req, err := http.NewRequest(http.MethodGet, raw, nil) // #nosec G107 -- attachment URLs are explicitly supplied by the caller.
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("attachment exceeds maximum size of %d bytes", maxBytes)
	}
	return data, nil
}

func readFileURLAttachment(raw string) ([]byte, string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, "", err
	}
	path := parsed.Path
	if path == "" && parsed.Host != "" {
		path = parsed.Host
	}
	// #nosec G304 -- file:// attachment paths are explicitly supplied by the caller.
	data, err := os.ReadFile(path)
	return data, path, err
}

func detectMIMEType(name string, data []byte) string {
	if ext := filepath.Ext(name); ext != "" {
		if value := mime.TypeByExtension(ext); value != "" {
			if mediaType, _, err := mime.ParseMediaType(value); err == nil && mediaType != "" {
				return mediaType
			}
			return value
		}
	}
	if len(data) > 0 {
		return http.DetectContentType(data)
	}
	return "application/octet-stream"
}

func escapeMultipartParam(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return strings.ReplaceAll(value, `"`, `\"`)
}
