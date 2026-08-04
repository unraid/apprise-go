package notify

import (
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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

// Base64 returns the attachment encoded the way a JSON email API expects it.
func (a Attachment) Base64() string {
	return base64.StdEncoding.EncodeToString(a.Data)
}

// FileName returns the attachment's name, falling back to a generated one so
// a nameless attachment still arrives with something usable.
func (a Attachment) FileName(index int, extension string) string {
	if name := strings.TrimSpace(a.Name); name != "" {
		return name
	}

	return fmt.Sprintf("file%03d%s", index+1, extension)
}

// The JSON email APIs all base64 the file and differ only in what they call
// the fields. Each encoder below matches one service exactly; the differences
// are not cosmetic, since a wrong key means the attachment is dropped by the
// receiver without complaint.

// attachmentsSendGridStyle is used by SendGrid and Resend.
func attachmentsSendGridStyle(attachments []Attachment) []any {
	out := make([]any, 0, len(attachments))
	for index, attachment := range attachments {
		out = append(out, map[string]any{
			"content":  attachment.Base64(),
			"filename": attachment.FileName(index, ".dat"),
			// Upstream sends a fixed type here rather than the detected one.
			"type":        "application/octet-stream",
			"disposition": "attachment",
		})
	}

	return out
}

// attachmentsMailerSendStyle omits the type field SendGrid sends.
func attachmentsMailerSendStyle(attachments []Attachment) []any {
	out := make([]any, 0, len(attachments))
	for index, attachment := range attachments {
		out = append(out, map[string]any{
			"content":     attachment.Base64(),
			"filename":    attachment.FileName(index, ".dat"),
			"disposition": "attachment",
		})
	}

	return out
}

// brevoValidExtensions are the only extensions Brevo accepts. It ignores the
// content type entirely and decides from the filename, so anything else has
// to be renamed or it is rejected outright.
var brevoValidExtensions = map[string]struct{}{}

func init() {
	for _, ext := range strings.Fields(
		"aif aifc aiff avi bmp cgm css csv doc docm docx eps ez flac gif htm " +
			"html ics jpeg jpg m4a m4v mkv mobi mov mp3 mp4 mpeg mpg msg ods " +
			"odt ogg pdf pkpass png ppt pptx pub rtf shtml tar tif tiff txt " +
			"wav wma wmv xls xlsx xml zip") {
		brevoValidExtensions[ext] = struct{}{}
	}
}

func attachmentsBrevoStyle(attachments []Attachment) []any {
	out := make([]any, 0, len(attachments))
	for index, attachment := range attachments {
		// .txt rather than .dat, which Brevo rejects.
		name := attachment.FileName(index, ".txt")
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
		if _, ok := brevoValidExtensions[ext]; !ok {
			name += ".txt"
		}

		out = append(out, map[string]any{
			"content": attachment.Base64(),
			"name":    name,
		})
	}

	return out
}

func attachmentsSparkPostStyle(attachments []Attachment) []any {
	out := make([]any, 0, len(attachments))
	for index, attachment := range attachments {
		out = append(out, map[string]any{
			"name": attachment.FileName(index, ".dat"),
			"type": attachment.MimeType,
			"data": attachment.Base64(),
		})
	}

	return out
}

func attachmentsSMTP2GoStyle(attachments []Attachment) []any {
	out := make([]any, 0, len(attachments))
	for index, attachment := range attachments {
		out = append(out, map[string]any{
			"filename": attachment.FileName(index, ".dat"),
			"fileblob": attachment.Base64(),
			"mimetype": attachment.MimeType,
		})
	}

	return out
}

// attachmentsPostmarkStyle capitalises its keys, unlike every other service
// here.
func attachmentsPostmarkStyle(attachments []Attachment) []any {
	out := make([]any, 0, len(attachments))
	for index, attachment := range attachments {
		out = append(out, map[string]any{
			"Name":        attachment.FileName(index, ".dat"),
			"Content":     attachment.Base64(),
			"ContentType": attachment.MimeType,
		})
	}

	return out
}

// attachmentsSMSEagleStyle carries no filename at all; the content type is the
// only thing describing the file.
func attachmentsSMSEagleStyle(attachments []Attachment) []any {
	out := make([]any, 0, len(attachments))
	for _, attachment := range attachments {
		out = append(out, map[string]any{
			"content_type": attachment.MimeType,
			"content":      attachment.Base64(),
		})
	}

	return out
}

// attachmentsCustomJSONStyle is the shape the generic json:// webhook sends.
func attachmentsCustomJSONStyle(attachments []Attachment) []any {
	out := make([]any, 0, len(attachments))
	for index, attachment := range attachments {
		out = append(out, map[string]any{
			"filename": attachment.FileName(index, ".dat"),
			"base64":   attachment.Base64(),
			"mimetype": attachment.MimeType,
		})
	}

	return out
}

// attachmentsCustomXMLStyle renders the Attachments element the generic
// xml:// webhook sends.
func attachmentsCustomXMLStyle(attachments []Attachment) string {
	if len(attachments) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString(`<Attachments format="base64">`)
	for index, attachment := range attachments {
		builder.WriteString(fmt.Sprintf(`<Attachment filename="%s" mimetype="%s">`,
			escapeXML(attachment.FileName(index, ".dat")), escapeXML(attachment.MimeType)))
		builder.WriteString(attachment.Base64())
		builder.WriteString("</Attachment>")
	}
	builder.WriteString("</Attachments>")

	return builder.String()
}

// telegramMediaRoute names the endpoint and form field Telegram wants for a
// given file. The table is ordered and scanned top to bottom, since the gif
// rule has to win before the general image rule sees it.
type telegramMediaRoute struct {
	pattern *regexp.Regexp
	method  string
	field   string
}

var telegramMediaRoutes = []telegramMediaRoute{
	// Animations are documented to support gif or H.264 only.
	{regexp.MustCompile(`(?i)^(image/gif|video/H264)`), "sendAnimation", "animation"},
	// Catches every remaining image type.
	{regexp.MustCompile(`(?i)^image/.*`), "sendPhoto", "photo"},
	{regexp.MustCompile(`(?i)^video/mp4`), "sendVideo", "video"},
	{regexp.MustCompile(`(?i)^(application|audio)/ogg`), "sendVoice", "voice"},
	{regexp.MustCompile(`(?i)^audio/(mpeg|mp4a-latm)`), "sendAudio", "audio"},
	// Everything else.
	{regexp.MustCompile(`.*`), "sendDocument", "document"},
}

func telegramRouteFor(mimeType string) telegramMediaRoute {
	for _, route := range telegramMediaRoutes {
		if route.pattern.MatchString(mimeType) {
			return route
		}
	}

	return telegramMediaRoutes[len(telegramMediaRoutes)-1]
}
