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
	if payload["text"] != "~Strike~ *Bold* _Italics_ Text" {
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
