package parity

import (
	"encoding/base64"
	"net/url"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

// loadAttachmentBytes reads each attachment a case names.
func loadAttachmentBytes(paths []string) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, path := range paths {
		data, err := os.ReadFile(strings.TrimPrefix(path, "file://"))
		if err != nil {
			return nil, err
		}
		out[path] = data
	}

	return out, nil
}

// TestAttachmentGoldensCarryTheirFiles checks that a golden captured for a
// case with attachments actually contains the file's bytes somewhere.
//
// This is what catches a mock that is too thin. update_golden --check proves
// the golden still matches upstream, but if a mocked response is missing a
// field upstream needs — an upload URL, a resolved channel id — upstream
// quietly skips the upload, the golden records the reduced exchange, and a Go
// implementation that also skips it looks like agreement. Both sides are then
// wrong in the same direction and every other check passes.
//
// Twice during this work a thin mock produced exactly that: Slack's
// chat.postMessage mock had no channel field, and GroupMe's URL carried no
// access token. Asserting the bytes are present is what makes those visible.
func TestAttachmentGoldensCarryTheirFiles(t *testing.T) {
	defs := loadProviderDefinitions(t)

	checked := 0
	for name := range defs {
		def := defs[name]
		golden := loadProviderGolden(t, def.Dir, def.Cases)
		goldenByName := map[string]goldenCase{}
		for _, entry := range golden {
			goldenByName[entry.Name] = entry
		}

		for _, c := range def.Cases {
			if len(c.Attachments) == 0 || c.AttachmentDropped {
				continue
			}

			entry, ok := goldenByName[c.Name]
			if !ok {
				t.Errorf("%s/%s: no golden for an attachment case", name, c.Name)
				continue
			}

			attachments, err := loadAttachmentBytes(c.Attachments)
			if err != nil {
				t.Fatalf("%s/%s: %v", name, c.Name, err)
			}

			checked++
			for path, data := range attachments {
				if !goldenCarries(entry, data) {
					t.Errorf("%s/%s: upstream never transmitted %s; "+
						"the capture is not exercising the attachment path "+
						"(a mocked response is probably missing something "+
						"upstream needs before it will upload)",
						name, c.Name, path)
				}
			}
		}
	}

	if checked == 0 {
		t.Fatal("no attachment case was checked; this guard is not running")
	}
}

// goldenCarries reports whether any request in the case carries the file.
func goldenCarries(entry goldenCase, data []byte) bool {
	for _, request := range entry.Requests {
		body := request.Body
		if request.BodyBase64 != "" {
			if decoded, err := base64.StdEncoding.DecodeString(request.BodyBase64); err == nil {
				body = string(decoded)
			}
		}

		if bodyCarries(body, data) {
			return true
		}

		// A service that builds an email posts the whole message base64'd,
		// the attachment's own base64 included, so the file only turns up
		// one layer down.
		for _, nested := range nestedBodies(body) {
			if bodyCarries(nested, data) {
				return true
			}
		}
	}

	return false
}

// bodyCarries reports whether a single body carries the file. Services
// transmit it as raw bytes, as base64, or as base64 inside a form-encoded
// field, so all three spellings count.
func bodyCarries(body string, data []byte) bool {
	encoded := base64.StdEncoding.EncodeToString(data)
	for _, candidate := range []string{string(data), encoded, url.QueryEscape(encoded)} {
		if strings.Contains(body, candidate) {
			return true
		}
	}

	// Base64 inside a MIME part is broken into fixed-width lines, so it only
	// matches once the line breaks are out of the way.
	return strings.Contains(stripWhitespace(body), encoded)
}

// nestedBodies returns the decoded form of every form field that turns out to
// hold base64-encoded text.
func nestedBodies(body string) []string {
	values, err := url.ParseQuery(body)
	if err != nil {
		return nil
	}

	var nested []string
	for _, list := range values {
		for _, value := range list {
			decoded, err := base64.StdEncoding.DecodeString(value)
			if err != nil || !utf8.Valid(decoded) {
				continue
			}
			nested = append(nested, string(decoded))
		}
	}

	return nested
}
