package parity

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

// smtpPart is one leaf of a message's MIME tree, reduced to the fields that
// decide how a mail client treats it.
type smtpPart struct {
	ContentType string
	Disposition string
	ContentID   string
	Content     string

	// raw is the part's bytes as decoded, before newline normalization. The
	// normalized form is what two implementations are compared on; this is
	// what proves a file arrived intact, and for a binary file the two are
	// not the same string.
	raw []byte
}

// TestMailtoAttachmentParity compares the message mailto builds for a file
// against the one upstream builds.
//
// mailto declares attachment support and, until this test, sent nothing: the
// coverage guard walks the HTTP parity providers, and mailto is SMTP, so it
// was never in the count that reached "41 of 41".
//
// Attachments change the message's shape rather than adding to it. The body
// is demoted into the first part, and with inline images the whole message
// becomes multipart/related rather than multipart/mixed — a difference a
// client acts on, rendering the image in place instead of offering it as a
// download.
func TestMailtoAttachmentParity(t *testing.T) {
	dir := t.TempDir()

	pixel, err := os.ReadFile(filepath.Join(
		testutil.RepoRoot(t), "internal", "parity", "fixtures", "pixel.png"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	imagePath := filepath.Join(dir, "pixel.png")
	if err := os.WriteFile(imagePath, pixel, 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	docPath := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(docPath, []byte("plain text attachment\n"), 0o600); err != nil {
		t.Fatalf("write doc: %v", err)
	}

	cases := []struct {
		name        string
		query       string
		body        string
		attachments []string
	}{
		{
			name:        "mixed",
			body:        "plain body",
			attachments: []string{docPath},
		},
		{
			name:        "several files",
			body:        "plain body",
			attachments: []string{docPath, imagePath},
		},
		{
			// An image with inline on turns the message into related and
			// appends an anchor referencing it.
			name:        "inline image is embedded",
			query:       "&inline=yes&format=html",
			body:        "<b>see below</b>",
			attachments: []string{imagePath},
		},
		{
			// A cid: the caller wrote is honoured as-is, with no anchor
			// appended for an image already referenced.
			name:        "explicit cid reference",
			query:       "&inline=yes&format=html",
			body:        `<img src="cid:pixel.png"> done`,
			attachments: []string{imagePath},
		},
		{
			// Plain text cannot embed anything, so upstream names the
			// images in the body instead. The format has to be stated:
			// mailto defaults to HTML, so leaving it off tests the HTML
			// path a second time rather than this one.
			name:        "inline on a text body names the image",
			query:       "&inline=yes&format=text",
			body:        "plain body",
			attachments: []string{imagePath},
		},
		{
			// A non-image is a download even with inline on.
			name:        "inline leaves a non-image alone",
			query:       "&inline=yes&format=html",
			body:        "<b>body</b>",
			attachments: []string{docPath},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capture := testutil.StartSMTPCapture(t)
			defer func() {
				_ = capture.Close()
			}()

			host, port, err := net.SplitHostPort(capture.Addr())
			if err != nil {
				t.Fatalf("smtp host split: %v", err)
			}

			url := fmt.Sprintf(
				"mailto://%s:%s?from=sender@example.com&to=recipient@example.com&mode=insecure%s",
				host, port, tc.query)

			args := []string{"--url", url, "--body", tc.body, "--title", "title"}
			for _, path := range tc.attachments {
				args = append(args, "--attach", path)
			}

			t.Setenv("PYTHONPATH", testutil.AppriseSourceRoot(t))
			script := filepath.Join(testutil.RepoRoot(t),
				"internal", "testutil", "scripts", "capture_smtp.py")
			stdout, stderr, err := testutil.RunPythonScript(t, script, args...)
			if err != nil {
				t.Fatalf("python smtp send failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
			}

			var result smtpSendResult
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatalf("parse python result: %v (stdout: %s)", err, stdout)
			}
			if !result.Success {
				t.Fatalf("python smtp send reported failure: %s", strings.TrimSpace(stdout))
			}

			pythonMsgs := capture.Messages()
			if len(pythonMsgs) == 0 {
				t.Fatal("no smtp message captured from python")
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

			attachments := make([]notify.Attachment, 0, len(tc.attachments))
			for _, path := range tc.attachments {
				attachment, err := notify.LoadAttachment(path)
				if err != nil {
					t.Fatalf("load attachment: %v", err)
				}
				attachments = append(attachments, attachment)
			}

			if err := notify.SendWithAttachments(
				target, tc.body, "title", notify.NotifyInfo, attachments); err != nil {
				t.Fatalf("go mailto send failed: %v", err)
			}

			goMsgs := capture.Messages()
			if len(goMsgs) == 0 {
				t.Fatal("no smtp message captured from go")
			}
			goMsg := goMsgs[len(goMsgs)-1]

			pythonType, pythonParts := smtpStructure(t, pythonMsg.Data)
			goType, goParts := smtpStructure(t, goMsg.Data)

			if pythonType != goType {
				t.Fatalf("top level content type mismatch: python=%s go=%s", pythonType, goType)
			}
			assertSMTPPartsEqual(t, pythonParts, goParts)

			// The structure could match with an empty file in it, so check
			// the bytes actually arrived.
			for _, path := range tc.attachments {
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("read attachment: %v", err)
				}
				if !smtpPartsCarry(goParts, want) {
					t.Fatalf("the message does not carry %s", filepath.Base(path))
				}
			}
		})
	}
}

func assertSMTPPartsEqual(t *testing.T, python, goParts []smtpPart) {
	t.Helper()

	if len(python) != len(goParts) {
		t.Fatalf("part count mismatch: python has %d, go has %d\npython=%s\ngo=%s",
			len(python), len(goParts), smtpPartSummary(python), smtpPartSummary(goParts))
	}

	for i := range python {
		if python[i].ContentType != goParts[i].ContentType ||
			python[i].Disposition != goParts[i].Disposition ||
			python[i].ContentID != goParts[i].ContentID ||
			python[i].Content != goParts[i].Content {
			t.Fatalf("part %d mismatch:\npython=%s\ngo=%s",
				i, describeSMTPPart(python[i]), describeSMTPPart(goParts[i]))
		}
	}
}

func describeSMTPPart(part smtpPart) string {
	return fmt.Sprintf("type=%q disposition=%q cid=%q content=%q",
		part.ContentType, part.Disposition, part.ContentID, part.Content)
}

func smtpPartSummary(parts []smtpPart) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, part.ContentType+"/"+part.Disposition)
	}

	return strings.Join(out, ", ")
}

func smtpPartsCarry(parts []smtpPart, data []byte) bool {
	for _, part := range parts {
		if bytes.Contains(part.raw, data) {
			return true
		}
	}

	return false
}

// smtpStructure returns the message's top-level type and its leaf parts.
func smtpStructure(t *testing.T, raw string) (string, []smtpPart) {
	t.Helper()

	parsed, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parse smtp message: %v", err)
	}

	body, err := io.ReadAll(parsed.Body)
	if err != nil {
		t.Fatalf("read smtp body: %v", err)
	}

	header := textprotoHeader(parsed.Header)
	mediaType, _, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
	if err != nil {
		mediaType = strings.TrimSpace(parsed.Header.Get("Content-Type"))
	}

	return mediaType, flattenSMTPParts(t, header, body)
}

// textprotoHeader reduces a mail header to the three fields the walk needs.
func textprotoHeader(header mail.Header) map[string]string {
	return map[string]string{
		"Content-Type":              header.Get("Content-Type"),
		"Content-Transfer-Encoding": header.Get("Content-Transfer-Encoding"),
		"Content-Disposition":       header.Get("Content-Disposition"),
		"Content-ID":                header.Get("Content-ID"),
	}
}

// flattenSMTPParts walks a MIME tree depth first, returning its leaves. The
// boundaries themselves are generated per message and carry no information, so
// only the parts they delimit are compared.
func flattenSMTPParts(t *testing.T, header map[string]string, body []byte) []smtpPart {
	t.Helper()

	mediaType, params, err := mime.ParseMediaType(header["Content-Type"])
	if err == nil && strings.HasPrefix(mediaType, "multipart/") && params["boundary"] != "" {
		var parts []smtpPart
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("read multipart: %v", err)
			}

			content, err := io.ReadAll(part)
			if err != nil {
				t.Fatalf("read part: %v", err)
			}

			nested := map[string]string{
				"Content-Type":              part.Header.Get("Content-Type"),
				"Content-Transfer-Encoding": part.Header.Get("Content-Transfer-Encoding"),
				"Content-Disposition":       part.Header.Get("Content-Disposition"),
				"Content-ID":                part.Header.Get("Content-ID"),
			}
			parts = append(parts, flattenSMTPParts(t, nested, content)...)
		}

		return parts
	}

	decoded := decodeSMTPPart(header["Content-Transfer-Encoding"], body)

	return []smtpPart{{
		ContentType: normalizeContentType(header["Content-Type"]),
		Disposition: normalizeDisposition(header["Content-Disposition"]),
		ContentID:   strings.TrimSpace(header["Content-ID"]),
		Content:     strings.TrimRight(strings.ReplaceAll(string(decoded), "\r\n", "\n"), "\n"),
		raw:         decoded,
	}}
}

// normalizeDisposition keeps the disposition and its filename, which is what
// decides whether a client shows the file or offers it, and under what name.
func normalizeDisposition(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	mediaType, params, err := mime.ParseMediaType(value)
	if err != nil {
		return value
	}
	if params["filename"] == "" {
		return mediaType
	}

	return mediaType + `; filename="` + params["filename"] + `"`
}

func decodeSMTPPart(encoding string, raw []byte) []byte {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		if decoded, err := base64.StdEncoding.DecodeString(stripWhitespace(string(raw))); err == nil {
			raw = decoded
		}
	case "quoted-printable":
		if decoded, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(raw))); err == nil {
			raw = decoded
		}
	}

	return raw
}
