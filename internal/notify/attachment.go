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
	"regexp"
	"strings"
	"time"
)

// DefaultMaxAttachmentBytes matches Python Apprise's default attachment cap.
const DefaultMaxAttachmentBytes int64 = 1048576000

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
			"type": attachment.MIMEType,
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
			"mimetype": attachment.MIMEType,
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
			"ContentType": attachment.MIMEType,
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
			"content_type": attachment.MIMEType,
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
			"mimetype": attachment.MIMEType,
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
		fmt.Fprintf(&builder, `<Attachment filename="%s" mimetype="%s">`,
			escapeXML(attachment.FileName(index, ".dat")), escapeXML(attachment.MIMEType))
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
