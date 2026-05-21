package notify_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

func TestTelegramHTMLFormatSendsUnescapedHTML(t *testing.T) {
	assertTelegramFormatParity(t, "tgram://123456:abcdef/7890/?format=html", "<b>This is Bold Text</b>", "", "html")
}

func TestTelegramMarkdownFormatSetsMarkdownParseMode(t *testing.T) {
	assertTelegramFormatParity(t, "tgram://123456:abcdef/7890/?format=markdown&mdv=v1", "_This is Italics Text_", "", "markdown")
}

func TestTelegramConvertsStandardMarkdownToMarkdownV1(t *testing.T) {
	payload := captureTelegramPayload(t, "tgram://123456:abcdef/7890/?format=markdown&mdv=v1", "~~Strike~~ **Bold** _Italics_ Text", "", "markdown")

	if payload["parse_mode"] != "MARKDOWN" {
		t.Fatalf("expected MARKDOWN parse mode, got %#v", payload["parse_mode"])
	}
	if payload["text"] != "Strike *Bold* _Italics_ Text" {
		t.Fatalf("expected Telegram markdown body, got %#v", payload["text"])
	}
}

func TestTelegramConvertsStandardMarkdownToMarkdownV2(t *testing.T) {
	payload := captureTelegramPayload(t, "tgram://123456:abcdef/7890/?format=markdown&mdv=v2", "~~Strike~~ **Bold** _Italics_ Text", "", "markdown")

	if payload["parse_mode"] != "MarkdownV2" {
		t.Fatalf("expected MarkdownV2 parse mode, got %#v", payload["parse_mode"])
	}
	if payload["text"] != "~Strike~ *Bold* _Italics_ Text" {
		t.Fatalf("expected Telegram markdown body, got %#v", payload["text"])
	}
}

func TestTelegramConvertsStandardMarkdownToTelegramHTML(t *testing.T) {
	payload := captureTelegramPayload(t, "tgram://123456:abcdef/7890/?format=html", "~~Strike~~ **Bold** _Italics_ Text", "", "markdown")

	if payload["parse_mode"] != "HTML" {
		t.Fatalf("expected HTML parse mode, got %#v", payload["parse_mode"])
	}
	if payload["text"] != "<s>Strike</s> <b>Bold</b> <i>Italics</i> Text" {
		t.Fatalf("expected Telegram HTML body, got %#v", payload["text"])
	}
}

func TestTelegramConvertsInlineHTMLInMarkdownInputToTelegramHTML(t *testing.T) {
	payload := captureTelegramPayload(t, "tgram://123456:abcdef/7890/?format=html", "<b>Bold</b> <i>Italics</i> Text", "", "markdown")

	if payload["parse_mode"] != "HTML" {
		t.Fatalf("expected HTML parse mode, got %#v", payload["parse_mode"])
	}
	if payload["text"] != "<b>Bold</b> <i>Italics</i> Text" {
		t.Fatalf("expected Telegram HTML body, got %#v", payload["text"])
	}
}

func TestTelegramConvertsHTMLInputToMarkdownV1(t *testing.T) {
	payload := captureTelegramPayload(t, "tgram://123456:abcdef/7890/?format=markdown&mdv=v1", "<b>Bold</b> <i>Italics</i> Text", "", "html")

	if payload["parse_mode"] != "MARKDOWN" {
		t.Fatalf("expected MARKDOWN parse mode, got %#v", payload["parse_mode"])
	}
	if payload["text"] != "*Bold* _Italics_ Text" {
		t.Fatalf("expected Telegram markdown body, got %#v", payload["text"])
	}
}

func TestTelegramConvertsHTMLInputToMarkdownV2(t *testing.T) {
	payload := captureTelegramPayload(t, "tgram://123456:abcdef/7890/?format=markdown&mdv=v2", "<b>Bold</b> <i>Italics</i> Text", "", "html")

	if payload["parse_mode"] != "MarkdownV2" {
		t.Fatalf("expected MarkdownV2 parse mode, got %#v", payload["parse_mode"])
	}
	if payload["text"] != "*Bold* _Italics_ Text" {
		t.Fatalf("expected Telegram markdown body, got %#v", payload["text"])
	}
}

func TestTelegramConvertsMarkdownFencedCodeToMarkdownV1Pre(t *testing.T) {
	payload := captureTelegramPayload(t, "tgram://123456:abcdef/7890/?format=markdown&mdv=v1", "**Bold**\n_Italics_\n```go\nif x > 0 { return `tick` }\\path\n```", "", "markdown")

	if payload["parse_mode"] != "MARKDOWN" {
		t.Fatalf("expected MARKDOWN parse mode, got %#v", payload["parse_mode"])
	}
	text, ok := payload["text"].(string)
	if !ok {
		t.Fatalf("expected text payload, got %#v", payload["text"])
	}
	if strings.Contains(text, "````") {
		t.Fatalf("expected fenced code not to be double wrapped, got %q", text)
	}
	if !strings.Contains(text, "```\nif x > 0 { return `tick` }\\path\n```") {
		t.Fatalf("expected Telegram markdown pre block, got %q", text)
	}
	if strings.Contains(text, "\\`") || strings.Contains(text, "\\\\path") {
		t.Fatalf("expected Markdown v1 pre text not to show escape backslashes, got %q", text)
	}
}

func TestTelegramConvertsMarkdownFencedCodeToMarkdownV2Pre(t *testing.T) {
	payload := captureTelegramPayload(t, "tgram://123456:abcdef/7890/?format=markdown&mdv=v2", "```go\nif x > 0 { return `tick` }\\path\n```", "", "markdown")

	if payload["parse_mode"] != "MarkdownV2" {
		t.Fatalf("expected MarkdownV2 parse mode, got %#v", payload["parse_mode"])
	}
	text, ok := payload["text"].(string)
	if !ok {
		t.Fatalf("expected text payload, got %#v", payload["text"])
	}
	if strings.Contains(text, "````") {
		t.Fatalf("expected fenced code not to be double wrapped, got %q", text)
	}
	if !strings.Contains(text, "if x > 0 { return \\`tick\\` }\\\\path") {
		t.Fatalf("expected Telegram markdown pre body with code-only escaping, got %q", text)
	}
}

func TestTelegramConvertsMarkdownFencedCodeToTelegramHTML(t *testing.T) {
	payload := captureTelegramPayload(t, "tgram://123456:abcdef/7890/?format=html", "**Bold**\n_Italics_\n```\ncode\n```", "", "markdown")

	if payload["parse_mode"] != "HTML" {
		t.Fatalf("expected HTML parse mode, got %#v", payload["parse_mode"])
	}
	text, ok := payload["text"].(string)
	if !ok {
		t.Fatalf("expected text payload, got %#v", payload["text"])
	}
	for _, expected := range []string{"<b>Bold</b>", "<i>Italics</i>", "<pre><code>code\n</code></pre>"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in Telegram HTML body, got %q", expected, text)
		}
	}
}

func TestTelegramMarkdownV2CodeEscapesOnlyBackticksAndBackslashes(t *testing.T) {
	payload := captureTelegramPayload(t, "tgram://123456:abcdef/7890/?format=markdown&mdv=v2", "`if x > 0 { return path\\name }`", "", "markdown")

	text, ok := payload["text"].(string)
	if !ok {
		t.Fatalf("expected text payload, got %#v", payload["text"])
	}
	if text != "`if x > 0 { return path\\\\name }`" {
		t.Fatalf("expected minimally escaped code text, got %q", text)
	}
	for _, overescaped := range []string{`\>`, `\{`, `\}`} {
		if strings.Contains(text, overescaped) {
			t.Fatalf("expected code text not to contain overescaped fragment %q in %q", overescaped, text)
		}
	}
}

func TestTelegramMarkdownV2PreEscapesOnlyBackticksAndBackslashes(t *testing.T) {
	payload := captureTelegramPayload(t, "tgram://123456:abcdef/7890/?format=markdown&mdv=v2", "<pre>if x > 0 { return path\\name }</pre>", "", "html")

	text, ok := payload["text"].(string)
	if !ok {
		t.Fatalf("expected text payload, got %#v", payload["text"])
	}
	if text != "```\nif x > 0 { return path\\\\name }```" {
		t.Fatalf("expected minimally escaped pre text, got %q", text)
	}
	for _, overescaped := range []string{`\>`, `\{`, `\}`} {
		if strings.Contains(text, overescaped) {
			t.Fatalf("expected pre text not to contain overescaped fragment %q in %q", overescaped, text)
		}
	}
}

func TestTelegramMarkdownV2PreservesLinks(t *testing.T) {
	payload := captureTelegramPayload(t, "tgram://123456:abcdef/7890/?format=markdown&mdv=v2", `<a href="https://example.com/a)b\c">Docs</a>`, "", "html")

	text, ok := payload["text"].(string)
	if !ok {
		t.Fatalf("expected text payload, got %#v", payload["text"])
	}
	if text != "[Docs](https://example.com/a\\)b\\\\c)" {
		t.Fatalf("expected escaped Telegram markdown link, got %q", text)
	}
}

func TestTelegramTextFormatUsesHTMLParseMode(t *testing.T) {
	assertTelegramFormatParity(t, "tgram://123456:abcdef/7890/?format=text", "<b>plain</b>", "Title", "text")
}

func TestTelegramRejectsInvalidFormat(t *testing.T) {
	parsed, err := notify.ParseURL("tgram://123456:abcdef/7890/?format=bad")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if _, err := notify.NewTelegramTarget(parsed); err == nil {
		t.Fatalf("expected invalid format error")
	}
}

func TestTelegramMarkdownV2TitleEscapesReservedCharacters(t *testing.T) {
	target := mustTelegramTarget(t, "tgram://123456:abcdef/7890/?format=markdown&mdv=v2")

	spec, err := target.BuildRequest("body", `\_*[]()~`+"`"+`>#+-=|{}.!`, notify.NotifyInfo)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	payload := decodeTelegramPayload(t, spec.Body)
	if payload["parse_mode"] != "MarkdownV2" {
		t.Fatalf("expected MarkdownV2 parse mode, got %#v", payload["parse_mode"])
	}
	text, ok := payload["text"].(string)
	if !ok {
		t.Fatalf("expected text payload, got %#v", payload["text"])
	}
	for _, escaped := range []string{`\\`, `\_`, `\*`, `\[`, `\]`, `\(`, `\)`, `\~`, "\\`", `\>`, `\#`, `\+`, `\-`, `\=`, `\|`, `\{`, `\}`, `\.`, `\!`} {
		if !strings.Contains(text, escaped) {
			t.Fatalf("expected escaped fragment %q in %q", escaped, text)
		}
	}
}

func assertTelegramFormatParity(t *testing.T, rawURL, body, title, bodyFormat string) {
	t.Helper()
	testutil.RequirePythonApprise(t)

	pythonSpecs := testutil.CapturePythonRequestsWithFormat(t, rawURL, body, title, bodyFormat)

	parsed, err := notify.ParseURL(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	target, err := notify.NewTelegramTarget(parsed)
	if err != nil {
		t.Fatalf("new telegram target: %v", err)
	}
	convertedBody, err := notify.ConvertMessageFormatForTarget(parsed, body, bodyFormat)
	if err != nil {
		t.Fatalf("convert body: %v", err)
	}
	goSpecs := testutil.CaptureGoRequests(t, func() error {
		return target.Send(convertedBody, title, notify.NotifyInfo)
	})

	testutil.AssertRequestSpecSequenceMatches(t, pythonSpecs, goSpecs)
}

func captureTelegramPayload(t *testing.T, rawURL, body, title, bodyFormat string) map[string]any {
	t.Helper()

	specs := testutil.CaptureGoRequests(t, func() error {
		return notify.SendTargetURL(rawURL, body, title, bodyFormat, notify.NotifyInfo)
	})
	if len(specs) != 1 {
		t.Fatalf("expected one request, got %d", len(specs))
	}
	return decodeTelegramPayload(t, specs[0].Body)
}

func mustTelegramTarget(t *testing.T, raw string) *notify.TelegramTarget {
	t.Helper()
	parsed, err := notify.ParseURL(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	target, err := notify.NewTelegramTarget(parsed)
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
