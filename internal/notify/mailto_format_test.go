package notify

import (
	"strings"
	"testing"
)

func TestMailtoHTMLFormatAcceptsConvertedMarkdownBody(t *testing.T) {
	parsed, err := ParseURL("mailto://smtp.example.com/recipient@example.com?from=sender@example.com&format=html")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	target, err := NewMailtoTarget(parsed)
	if err != nil {
		t.Fatalf("new mailto target: %v", err)
	}
	body, err := ConvertMessageFormat("_This is Italics Text_", "markdown", "html")
	if err != nil {
		t.Fatalf("convert body: %v", err)
	}

	messages, err := target.buildMessages(body, "subject")
	if err != nil {
		t.Fatalf("build messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected one message, got %d", len(messages))
	}
	if !strings.Contains(messages[0].body, "Content-Type: text/html") {
		t.Fatalf("expected html part, got %s", messages[0].body)
	}
	if !strings.Contains(messages[0].body, "<em>This is Italics Text</em>") {
		t.Fatalf("expected converted markdown in html part, got %s", messages[0].body)
	}
}
