package parity

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

type smtpSendResult struct {
	Success bool `json:"success"`
}

type normalizedSMTP struct {
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

func TestMailtoSMTPParity(t *testing.T) {
	capture := testutil.StartSMTPCapture(t)
	defer func() {
		_ = capture.Close()
	}()

	host, port, err := net.SplitHostPort(capture.Addr())
	if err != nil {
		t.Fatalf("smtp host split failed: %v", err)
	}

	url := fmt.Sprintf(
		"mailto://%s:%s?from=sender@example.com&to=recipient@example.com&cc=cc@example.com&bcc=bcc@example.com&reply=reply@example.com&mode=insecure&format=text",
		host,
		port,
	)

	body := "apprise parity body"
	title := "apprise parity title"

	appriseRoot := testutil.AppriseSourceRoot(t)
	t.Setenv("PYTHONPATH", appriseRoot)

	script := filepath.Join(testutil.RepoRoot(t), "internal", "testutil", "scripts", "capture_smtp.py")
	stdout, stderr, err := testutil.RunPythonScript(t, script, "--url", url, "--body", body, "--title", title)
	if err != nil {
		t.Fatalf("python smtp send failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	var result smtpSendResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse python smtp result failed: %v (stdout: %s)", err, strings.TrimSpace(stdout))
	}
	if !result.Success {
		t.Fatalf("python smtp send reported failure: %s", strings.TrimSpace(stdout))
	}

	pythonMsgs := capture.Messages()
	if len(pythonMsgs) == 0 {
		t.Fatalf("no smtp message captured from python")
	}
	pythonMsg := pythonMsgs[len(pythonMsgs)-1]

	capture.Reset()

	parsed, err := notify.ParseURL(url)
	if err != nil {
		t.Fatalf("parse mailto url: %v", err)
	}
	target, err := notify.NewMailtoTarget(parsed)
	if err != nil {
		t.Fatalf("build mailto target: %v", err)
	}
	if err := target.Send(body, title, notify.NotifyInfo); err != nil {
		t.Fatalf("go mailto send failed: %v", err)
	}

	goMsgs := capture.Messages()
	if len(goMsgs) == 0 {
		t.Fatalf("no smtp message captured from go")
	}
	goMsg := goMsgs[len(goMsgs)-1]

	pythonNormalized := normalizeSMTPMessage(t, pythonMsg)
	goNormalized := normalizeSMTPMessage(t, goMsg)

	if !equalNormalizedSMTP(pythonNormalized, goNormalized) {
		pyJSON, _ := json.Marshal(pythonNormalized)
		goJSON, _ := json.Marshal(goNormalized)
		t.Fatalf("smtp parity mismatch\npython=%s\ngo=%s", pyJSON, goJSON)
	}
}

func normalizeSMTPMessage(t *testing.T, msg testutil.SMTPMessage) normalizedSMTP {
	t.Helper()

	parsed, err := mail.ReadMessage(strings.NewReader(msg.Data))
	if err != nil {
		t.Fatalf("parse smtp message failed: %v", err)
	}

	return normalizedSMTP{
		MailFrom:    normalizeAddress(msg.MailFrom),
		RcptTo:      normalizeAddressList(msg.RcptTo),
		From:        normalizeSingleHeaderAddress(parsed.Header.Get("From")),
		To:          normalizeHeaderAddressList(parsed.Header.Get("To")),
		Cc:          normalizeHeaderAddressList(parsed.Header.Get("Cc")),
		ReplyTo:     normalizeHeaderAddressList(parsed.Header.Get("Reply-To")),
		Subject:     decodeHeader(parsed.Header.Get("Subject")),
		Body:        decodeSMTPBody(t, parsed.Header, parsed.Body),
		ContentType: normalizeContentType(parsed.Header.Get("Content-Type")),
		AppID:       strings.TrimSpace(parsed.Header.Get("X-Application")),
	}
}

func normalizeAddress(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.Trim(value, "<>")))
}

func normalizeAddressList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		normalized := normalizeAddress(value)
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

func normalizeSingleHeaderAddress(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	addr, err := mail.ParseAddress(value)
	if err != nil {
		return normalizeAddress(value)
	}
	return normalizeAddress(addr.Address)
}

func normalizeHeaderAddressList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	list, err := mail.ParseAddressList(value)
	if err != nil {
		return normalizeAddressList(strings.Split(value, ","))
	}
	out := make([]string, 0, len(list))
	for _, addr := range list {
		out = append(out, normalizeAddress(addr.Address))
	}
	sort.Strings(out)
	return out
}

func decodeHeader(value string) string {
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

func normalizeContentType(value string) string {
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

	encoding := strings.ToLower(strings.TrimSpace(header.Get("Content-Transfer-Encoding")))
	switch encoding {
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(stripWhitespace(string(raw)))
		if err == nil {
			raw = decoded
		}
	case "quoted-printable":
		reader := quotedprintable.NewReader(bytes.NewReader(raw))
		decoded, err := io.ReadAll(reader)
		if err == nil {
			raw = decoded
		}
	}

	out := string(raw)
	out = strings.ReplaceAll(out, "\r\n", "\n")
	out = strings.TrimRight(out, "\n")
	return out
}

func stripWhitespace(value string) string {
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

func equalNormalizedSMTP(a, b normalizedSMTP) bool {
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
