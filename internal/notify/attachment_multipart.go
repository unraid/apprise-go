package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/textproto"
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
