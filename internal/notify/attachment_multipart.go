package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"regexp"
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
			fmt.Sprintf("files[%d]", index), attachment, true))
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
//
// withType is false for services that hand requests a filename and a handle
// without a type. Those parts carry no Content-Type at all, and adding one
// would not match what the service receives.
func attachmentPartHeader(field string, attachment Attachment, withType bool) textproto.MIMEHeader {
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(
		`form-data; name="%s"; filename="%s"`,
		escapeMultipartValue(field), escapeMultipartValue(attachment.Name)))

	if !withType {
		return header
	}

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
		// Mailgun is handed a filename and a handle with no type, so its
		// parts carry no Content-Type.
		part, err := writer.CreatePart(attachmentPartHeader(
			fmt.Sprintf("attachment[%d]", index), attachment, false))
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

// formAttachmentBody builds the multipart body the generic form:// webhook
// sends. The field name comes from ?attach_as=; a wildcard in it is replaced
// by a two-digit counter so several files get distinct names, and without one
// every file reuses the same field.
func formAttachmentBody(values url.Values, attachAs string, attachments []Attachment) (body string, contentType string, err error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

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
			formAttachmentField(attachAs, index), attachment, true))
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

// formAttachmentWildcard matches the placeholder characters upstream accepts
// in an attach_as value.
var formAttachmentWildcard = regexp.MustCompile(`[*?+$:.%]+`)

func formAttachmentField(attachAs string, index int) string {
	name := strings.TrimSpace(attachAs)
	if name == "" {
		name = "file*"
	}

	counter := fmt.Sprintf("%02d", index+1)
	if formAttachmentWildcard.MatchString(name) {
		return formAttachmentWildcard.ReplaceAllString(name, counter)
	}

	return name
}

// singleFileAttachmentBody builds a multipart body carrying form fields plus
// one file under the given field name. Pushover and the services shaped like
// it send exactly one attachment per request.
func singleFileAttachmentBody(
	values url.Values,
	field string,
	attachment Attachment,
	withType bool,
) (body string, contentType string, err error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

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

	part, err := writer.CreatePart(attachmentPartHeader(field, attachment, withType))
	if err != nil {
		return "", "", err
	}
	if _, err := part.Write(attachment.Data); err != nil {
		return "", "", err
	}

	if err := writer.Close(); err != nil {
		return "", "", err
	}

	return buffer.String(), writer.FormDataContentType(), nil
}

// appriseAPIFormAttachmentBody numbers its file fields fileNN, which is what
// the Apprise API expects in form mode.
func appriseAPIFormAttachmentBody(values url.Values, attachments []Attachment) (body string, contentType string, err error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

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
			fmt.Sprintf("file%02d", index+1),
			Attachment{
				Name:     attachment.FileName(index, ".dat"),
				MimeType: attachment.MimeType,
				Data:     attachment.Data,
			}, true))
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

// indexedFileAttachmentBody builds a multipart body whose file fields are
// named by a template taking the file's index — SMSC uses mes1, mes2 and
// 800.com repeats media[].
func indexedFileAttachmentBody(
	values url.Values,
	fieldFor func(index int) string,
	attachments []Attachment,
	withType bool,
) (body string, contentType string, err error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

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
			fieldFor(index),
			Attachment{
				Name:     attachment.FileName(index, ".dat"),
				MimeType: attachment.MimeType,
				Data:     attachment.Data,
			}, withType))
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
