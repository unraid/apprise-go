package testutil

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"sort"
	"strings"
	"testing"
)

type NormalizedSMTP struct {
	MailFrom    string
	RcptTo      []string
	From        string
	To          []string
	Cc          []string
	ReplyTo     []string
	Subject     string
	Body        string
	ContentType string
	AppID       string
}

func AssertSMTPMessagesMatch(t *testing.T, pythonMsg, goMsg SMTPMessage) {
	t.Helper()

	pythonNormalized := NormalizeSMTPMessage(t, pythonMsg)
	goNormalized := NormalizeSMTPMessage(t, goMsg)

	if !equalNormalizedSMTP(pythonNormalized, goNormalized) {
		pyJSON, _ := json.Marshal(pythonNormalized)
		goJSON, _ := json.Marshal(goNormalized)
		t.Fatalf("smtp parity mismatch\npython=%s\ngo=%s", pyJSON, goJSON)
	}
}

func AssertSMTPMessageSequencesMatch(t *testing.T, pythonMsgs, goMsgs []SMTPMessage) {
	t.Helper()

	if len(pythonMsgs) != len(goMsgs) {
		t.Fatalf("smtp message count mismatch: python=%d go=%d", len(pythonMsgs), len(goMsgs))
	}
	for i := range pythonMsgs {
		AssertSMTPMessagesMatch(t, pythonMsgs[i], goMsgs[i])
	}
}

func NormalizeSMTPMessage(t *testing.T, msg SMTPMessage) NormalizedSMTP {
	t.Helper()

	parsed, err := mail.ReadMessage(strings.NewReader(msg.Data))
	if err != nil {
		t.Fatalf("parse smtp message failed: %v", err)
	}

	return NormalizedSMTP{
		MailFrom:    normalizeSMTPAddress(msg.MailFrom),
		RcptTo:      normalizeSMTPAddressList(msg.RcptTo),
		From:        normalizeSMTPSingleHeaderAddress(parsed.Header.Get("From")),
		To:          normalizeSMTPHeaderAddressList(parsed.Header.Get("To")),
		Cc:          normalizeSMTPHeaderAddressList(parsed.Header.Get("Cc")),
		ReplyTo:     normalizeSMTPHeaderAddressList(parsed.Header.Get("Reply-To")),
		Subject:     decodeSMTPHeader(parsed.Header.Get("Subject")),
		Body:        decodeSMTPBody(t, parsed.Header, parsed.Body),
		ContentType: normalizeSMTPContentType(parsed.Header.Get("Content-Type")),
		AppID:       strings.TrimSpace(parsed.Header.Get("X-Application")),
	}
}

func normalizeSMTPAddress(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.Trim(value, "<>")))
}

func normalizeSMTPAddressList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		normalized := normalizeSMTPAddress(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func normalizeSMTPSingleHeaderAddress(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	addr, err := mail.ParseAddress(value)
	if err != nil {
		return normalizeSMTPAddress(value)
	}
	return normalizeSMTPAddress(addr.Address)
}

func normalizeSMTPHeaderAddressList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	list, err := mail.ParseAddressList(value)
	if err != nil {
		return normalizeSMTPAddressList(strings.Split(value, ","))
	}
	out := make([]string, 0, len(list))
	for _, addr := range list {
		out = append(out, normalizeSMTPAddress(addr.Address))
	}
	sort.Strings(out)
	return out
}

func decodeSMTPHeader(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	decoder := mime.WordDecoder{}
	decoded, err := decoder.DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

func normalizeSMTPContentType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return strings.ToLower(value)
	}
	return strings.ToLower(mediaType)
}

func decodeSMTPBody(t *testing.T, header mail.Header, body io.Reader) string {
	t.Helper()

	raw, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read smtp body failed: %v", err)
	}

	contentType := header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err == nil && strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
		boundary := params["boundary"]
		if boundary != "" {
			return decodeMultipartBody(t, boundary, raw)
		}
	}

	raw = decodeSMTPTransfer(t, header, raw)
	out := string(raw)
	out = strings.ReplaceAll(out, "\r\n", "\n")
	out = strings.TrimRight(out, "\n")
	return out
}

func decodeMultipartBody(t *testing.T, boundary string, raw []byte) string {
	t.Helper()

	reader := multipart.NewReader(bytes.NewReader(raw), boundary)
	parts := []string{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("parse smtp multipart failed: %v", err)
		}

		partBody := decodeSMTPPartBody(t, mail.Header(part.Header), part)
		partType := normalizeSMTPContentType(part.Header.Get("Content-Type"))
		parts = append(parts, partType+"\n"+partBody)
	}

	return strings.Join(parts, "\n--PART--\n")
}

func decodeSMTPPartBody(t *testing.T, header mail.Header, body io.Reader) string {
	t.Helper()

	raw, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read smtp part body failed: %v", err)
	}
	raw = decodeSMTPTransfer(t, header, raw)

	out := string(raw)
	out = strings.ReplaceAll(out, "\r\n", "\n")
	out = strings.TrimRight(out, "\n")
	return out
}

func decodeSMTPTransfer(t *testing.T, header mail.Header, raw []byte) []byte {
	t.Helper()

	encoding := strings.ToLower(strings.TrimSpace(header.Get("Content-Transfer-Encoding")))
	switch encoding {
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(stripSMTPWhitespace(string(raw)))
		if err == nil {
			return decoded
		}
	case "quoted-printable":
		reader := quotedprintable.NewReader(bytes.NewReader(raw))
		decoded, err := io.ReadAll(reader)
		if err == nil {
			return decoded
		}
	}
	return raw
}

func stripSMTPWhitespace(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case ' ', '\t', '\r', '\n':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func equalNormalizedSMTP(a, b NormalizedSMTP) bool {
	if a.MailFrom != b.MailFrom {
		return false
	}
	if !equalStringSlice(a.RcptTo, b.RcptTo) {
		return false
	}
	if a.From != b.From {
		return false
	}
	if !equalStringSlice(a.To, b.To) {
		return false
	}
	if !equalStringSlice(a.Cc, b.Cc) {
		return false
	}
	if !equalStringSlice(a.ReplyTo, b.ReplyTo) {
		return false
	}
	if a.Subject != b.Subject {
		return false
	}
	if a.Body != b.Body {
		return false
	}
	if a.ContentType != b.ContentType {
		return false
	}
	if a.AppID != b.AppID {
		return false
	}
	return true
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
