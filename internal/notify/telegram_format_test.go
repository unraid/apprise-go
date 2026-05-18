package notify

import (
	"encoding/json"
	"testing"
)

func TestTelegramHTMLFormatSendsUnescapedHTML(t *testing.T) {
	target := mustTelegramTarget(t, "tgram://123456:abcdef/7890/?format=html")

	spec, err := target.BuildRequest("<b>This is Bold Text</b>", "", NotifyInfo)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	payload := decodeTelegramPayload(t, spec.Body)
	if payload["parse_mode"] != "HTML" {
		t.Fatalf("expected HTML parse mode, got %#v", payload["parse_mode"])
	}
	if payload["text"] != "<b>This is Bold Text</b>" {
		t.Fatalf("expected unescaped HTML body, got %#v", payload["text"])
	}
}

func TestTelegramMarkdownFormatSetsMarkdownParseMode(t *testing.T) {
	target := mustTelegramTarget(t, "tgram://123456:abcdef/7890/?format=markdown&mdv=v1")

	spec, err := target.BuildRequest("_This is Italics Text_", "", NotifyInfo)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	payload := decodeTelegramPayload(t, spec.Body)
	if payload["parse_mode"] != "Markdown" {
		t.Fatalf("expected Markdown parse mode, got %#v", payload["parse_mode"])
	}
	if payload["text"] != "_This is Italics Text_" {
		t.Fatalf("expected markdown body, got %#v", payload["text"])
	}
}

func TestTelegramTextFormatOmitsParseMode(t *testing.T) {
	target := mustTelegramTarget(t, "tgram://123456:abcdef/7890/?format=text")

	spec, err := target.BuildRequest("<b>plain</b>", "Title", NotifyInfo)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	payload := decodeTelegramPayload(t, spec.Body)
	if _, ok := payload["parse_mode"]; ok {
		t.Fatalf("did not expect parse mode for text format: %#v", payload)
	}
	if payload["text"] != "Title\r\n<b>plain</b>" {
		t.Fatalf("expected plain text title/body, got %#v", payload["text"])
	}
}

func TestTelegramRejectsInvalidFormat(t *testing.T) {
	parsed, err := ParseURL("tgram://123456:abcdef/7890/?format=bad")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if _, err := NewTelegramTarget(parsed); err == nil {
		t.Fatalf("expected invalid format error")
	}
}

func mustTelegramTarget(t *testing.T, raw string) *TelegramTarget {
	t.Helper()
	parsed, err := ParseURL(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	target, err := NewTelegramTarget(parsed)
	if err != nil {
		t.Fatalf("new telegram target: %v", err)
	}
	return target
}

func decodeTelegramPayload(t *testing.T, body string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return payload
}
