package notify

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Attachment is a file to send alongside a notification.
//
// The contents are read once, up front, rather than streamed. A notification
// goes to every target of every URL, so a stream would be consumed by the
// first one and arrive empty at the rest.
type Attachment struct {
	Name     string
	MimeType string
	Data     []byte
}

// AttachmentSender is implemented by providers that can transmit attachments.
//
// It is deliberately separate from Sender: a provider that cannot carry a file
// should fail to satisfy this interface rather than accept an attachment and
// quietly drop it. Callers check for it and report the gap.
type AttachmentSender interface {
	Sender

	SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error
}

// LoadAttachment reads a local file. A file:// prefix is accepted so the same
// syntax works whether or not the caller strips it.
func LoadAttachment(location string) (Attachment, error) {
	path := strings.TrimSpace(location)
	path = strings.TrimPrefix(path, "file://")
	if path == "" {
		return Attachment{}, fmt.Errorf("empty attachment path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Attachment{}, fmt.Errorf("read attachment %s: %w", path, err)
	}

	name := filepath.Base(path)

	return Attachment{
		Name:     name,
		MimeType: attachmentMimeType(name, data),
		Data:     data,
	}, nil
}

// LoadAttachments reads every location, stopping at the first failure so a
// notification never goes out with only some of what was asked for.
func LoadAttachments(locations []string) ([]Attachment, error) {
	attachments := make([]Attachment, 0, len(locations))
	for _, location := range locations {
		if strings.TrimSpace(location) == "" {
			continue
		}
		attachment, err := LoadAttachment(location)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, attachment)
	}

	return attachments, nil
}

// attachmentMimeType prefers the extension and falls back to sniffing the
// contents, which is what a receiver would do with an unhelpful name.
func attachmentMimeType(name string, data []byte) string {
	if byExtension := mime.TypeByExtension(filepath.Ext(name)); byExtension != "" {
		return strings.SplitN(byExtension, ";", 2)[0]
	}
	if len(data) > 0 {
		return strings.SplitN(http.DetectContentType(data), ";", 2)[0]
	}

	return "application/octet-stream"
}

// SendWithAttachments delivers a notification, including attachments when
// there are any.
//
// A target that cannot carry them is an error rather than a silent downgrade:
// someone attaching a file and seeing a success has every reason to believe it
// arrived.
func SendWithAttachments(target Sender, body, title string, notifyType NotifyType, attachments []Attachment) error {
	if len(attachments) == 0 {
		return target.Send(body, title, notifyType)
	}

	sender, ok := target.(AttachmentSender)
	if !ok {
		return fmt.Errorf("this service does not support attachments")
	}

	return sender.SendWithAttachments(body, title, notifyType, attachments)
}
