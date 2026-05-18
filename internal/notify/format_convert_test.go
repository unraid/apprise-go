package notify

import (
	"strings"
	"testing"
)

func TestConvertMessageFormatMarkdownToHTML(t *testing.T) {
	converted, err := ConvertMessageFormat("_This is Italics Text_", "markdown", "html")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if !strings.Contains(converted, "<em>This is Italics Text</em>") {
		t.Fatalf("expected markdown converted to HTML, got %q", converted)
	}
}

func TestConvertMessageFormatHTMLToText(t *testing.T) {
	converted, err := ConvertMessageFormat("<b>This is Bold Text</b>", "html", "text")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if converted != "This is Bold Text" {
		t.Fatalf("expected plain text, got %q", converted)
	}
}

func TestConvertMessageFormatRejectsInvalidFormats(t *testing.T) {
	if _, err := ConvertMessageFormat("body", "bad", "html"); err == nil {
		t.Fatalf("expected invalid input format error")
	}
	if _, err := ConvertMessageFormat("body", "text", "bad"); err == nil {
		t.Fatalf("expected invalid output format error")
	}
}
