package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"sort"
	"strings"
)

// discordStyleAttachmentBody builds the multipart body Discord and the
// services modelled on it expect: the ordinary JSON payload under
// payload_json, then one part per file named files[N].
//
// The content type is returned alongside the body because it carries the
// generated boundary and cannot be written by the caller.
func discordStyleAttachmentBody(payload map[string]any, attachments []Attachment) (body string, contentType string, err error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	if err := writer.WriteField("payload_json", string(data)); err != nil {
		return "", "", err
	}

	for index, attachment := range attachments {
		part, err := writer.CreatePart(attachmentPartHeader(
			fmt.Sprintf("files[%d]", index), attachment))
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

	return buffer.String(), writer.FormDataContentType(), nil
}

// attachmentPartHeader writes the part header by hand so the file's own
// content type survives; CreateFormFile hardcodes application/octet-stream.
func attachmentPartHeader(field string, attachment Attachment) textproto.MIMEHeader {
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(
		`form-data; name="%s"; filename="%s"`,
		escapeMultipartValue(field), escapeMultipartValue(attachment.Name)))

	mimeType := attachment.MimeType
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	header.Set("Content-Type", mimeType)

	return header
}

func escapeMultipartValue(value string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
}

// mailgunAttachmentBody converts a form-encoded payload into the multipart
// body Mailgun needs once files are present: every existing field is carried
// over, then one part per file named attachment[N].
func mailgunAttachmentBody(values url.Values, attachments []Attachment) (body string, contentType string, err error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	// Sorted so the body is reproducible; url.Values iterates a map.
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		for _, value := range values[key] {
			if err := writer.WriteField(key, value); err != nil {
				return "", "", err
			}
		}
	}

	for index, attachment := range attachments {
		part, err := writer.CreatePart(attachmentPartHeader(
			fmt.Sprintf("attachment[%d]", index), attachment))
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

	return buffer.String(), writer.FormDataContentType(), nil
}
