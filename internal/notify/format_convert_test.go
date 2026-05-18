package notify_test

import (
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

func TestConvertMessageFormatMarkdownToHTML(t *testing.T) {
	assertConvertMessageFormatRequestParity(t, "_This is Italics Text_", "markdown", "html")
}

func TestConvertMessageFormatHTMLToText(t *testing.T) {
	assertConvertMessageFormatRequestParity(t, "<b>This is Bold Text</b>", "html", "text")
}

func TestConvertMessageFormatHTMLToMarkdownMatchesPythonFallback(t *testing.T) {
	assertConvertMessageFormatRequestParity(t, "<b>This is Bold Text</b>", "html", "markdown")
}

func TestConvertMessageFormatRejectsInvalidFormats(t *testing.T) {
	if _, err := notify.ConvertMessageFormat("body", "bad", "html"); err == nil {
		t.Fatalf("expected invalid input format error")
	}
	if _, err := notify.ConvertMessageFormat("body", "text", "bad"); err == nil {
		t.Fatalf("expected invalid output format error")
	}
}

func assertConvertMessageFormatRequestParity(t *testing.T, body, inputFormat, outputFormat string) {
	t.Helper()
	testutil.RequirePythonApprise(t)

	url := "json://example.com/notify?format=" + outputFormat
	pythonSpecs := testutil.CapturePythonRequestsWithFormat(t, url, body, "", inputFormat)

	converted, err := notify.ConvertMessageFormat(body, inputFormat, outputFormat)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if outputFormat == "html" && !strings.Contains(converted, "<em>This is Italics Text</em>") {
		t.Fatalf("expected markdown converted to HTML, got %q", converted)
	}

	parsed, err := notify.ParseURL(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	target, err := notify.NewJSONTarget(parsed)
	if err != nil {
		t.Fatalf("new json target: %v", err)
	}
	goSpecs := testutil.CaptureGoRequests(t, func() error {
		return target.Send(converted, "", notify.NotifyInfo)
	})

	testutil.AssertRequestSpecSequenceMatches(t, pythonSpecs, goSpecs)
}
