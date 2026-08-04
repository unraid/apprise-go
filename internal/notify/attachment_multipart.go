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
// services modeled on it expect: the ordinary JSON payload under
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

	mimeType := attachment.MIMEType
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
func mailgunAttachmentBody(fields formFields, attachments []Attachment) (body string, contentType string, err error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	if err := writeFormFields(writer, fields); err != nil {
		return "", "", err
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
// sends. The field name is the template parseFormAttachAs produced: numbered
// per file when it carries a placeholder, and reused as-is when it does not.
func formAttachmentBody(
	fields formFields,
	attachAs string,
	numbered bool,
	attachments []Attachment,
) (body string, contentType string, err error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	if err := writeFormFields(writer, fields); err != nil {
		return "", "", err
	}

	for index, attachment := range attachments {
		part, err := writer.CreatePart(attachmentPartHeader(
			formAttachmentField(attachAs, numbered, index), attachment, true))
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

// formAttachmentField names the part for one file.
func formAttachmentField(attachAs string, numbered bool, index int) string {
	if !numbered {
		return attachAs
	}

	return fmt.Sprintf(attachAs, index+1)
}

// singleFileAttachmentBody builds a multipart body carrying form fields plus
// one file under the given field name. Pushover and the services shaped like
// it send exactly one attachment per request.
func singleFileAttachmentBody(
	fields formFields,
	field string,
	attachment Attachment,
	withType bool,
) (body string, contentType string, err error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	if err := writeFormFields(writer, fields); err != nil {
		return "", "", err
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
func appriseAPIFormAttachmentBody(fields formFields, attachments []Attachment) (body string, contentType string, err error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	if err := writeFormFields(writer, fields); err != nil {
		return "", "", err
	}

	for index, attachment := range attachments {
		part, err := writer.CreatePart(attachmentPartHeader(
			fmt.Sprintf("file%02d", index+1),
			Attachment{
				Name:     attachment.FileName(index, ".dat"),
				MIMEType: attachment.MIMEType,
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
	fields formFields,
	fieldFor func(index int) string,
	attachments []Attachment,
	withType bool,
) (body string, contentType string, err error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	if err := writeFormFields(writer, fields); err != nil {
		return "", "", err
	}

	for index, attachment := range attachments {
		part, err := writer.CreatePart(attachmentPartHeader(
			fieldFor(index),
			Attachment{
				Name:     attachment.FileName(index, ".dat"),
				MIMEType: attachment.MIMEType,
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

// ringCentralMMSBody builds RingCentral's MMS body: the same JSON metadata an
// SMS would carry, but as a named form part rather than the request body,
// followed by one part per file all sharing the field name attachment.
func ringCentralMMSBody(metadata map[string]any, attachments []Attachment) (body string, contentType string, err error) {
	data, err := json.Marshal(metadata)
	if err != nil {
		return "", "", err
	}

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	// The metadata part is handed over without a filename, so it is an
	// ordinary field that happens to declare its type.
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="json"`)
	header.Set("Content-Type", "application/json")

	part, err := writer.CreatePart(header)
	if err != nil {
		return "", "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", "", err
	}

	for _, attachment := range attachments {
		part, err := writer.CreatePart(attachmentPartHeader("attachment", attachment, true))
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

// writeFormFields writes the fields in the order they were added, which is the
// order upstream's payload dictionary declares them.
func writeFormFields(writer *multipart.Writer, fields formFields) error {
	for i, name := range fields.names {
		if err := writer.WriteField(name, fields.values[i]); err != nil {
			return err
		}
	}

	return nil
}
