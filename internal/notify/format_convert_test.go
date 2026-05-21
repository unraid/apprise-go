package notify_test

import (
	"encoding/json"
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

func TestTargetFormatConversionCorpusAcrossWorkflowTargets(t *testing.T) {
	body := strings.Join([]string{
		"# Deploy Summary",
		"",
		"**Bold** _Italics_ ~~Strike~~ [Docs](https://example.com/docs)",
		"",
		"`inline code`",
		"",
		"```go",
		"if x > 0 { return `tick` }\\path",
		"```",
		"",
		"- first",
		"- second",
		"",
		"<em>inline html</em>",
	}, "\n")

	cases := []struct {
		name       string
		rawURL     string
		assertBody func(t *testing.T, body string)
		assertSpec func(t *testing.T, spec notify.RequestSpec)
	}{
		{
			name:   "json html",
			rawURL: "json://example.com/notify?format=html",
			assertBody: func(t *testing.T, converted string) {
				t.Helper()
				assertContainsAll(t, converted, "<h1>Deploy Summary</h1>", "<strong>Bold</strong>", "<em>Italics</em>", "<pre><code class=\"language-go\">")
			},
		},
		{
			name:   "workflow markdown",
			rawURL: "workflow://example.com/WORKFLOWID/SIGNATURE?format=markdown&image=no",
			assertBody: func(t *testing.T, converted string) {
				t.Helper()
				assertContainsAll(t, converted, "**Bold**", "```go", "<em>inline html</em>")
			},
			assertSpec: func(t *testing.T, spec notify.RequestSpec) {
				t.Helper()
				assertContainsAll(t, workflowTextBlock(t, spec, "body"), "**Bold**", "```go", "<em>inline html</em>")
			},
		},
		{
			name:   "workflow html",
			rawURL: "workflow://example.com/WORKFLOWID/SIGNATURE?format=html&image=no",
			assertBody: func(t *testing.T, converted string) {
				t.Helper()
				assertContainsAll(t, converted, "<h1>Deploy Summary</h1>", "<strong>Bold</strong>", "<pre><code class=\"language-go\">")
			},
			assertSpec: func(t *testing.T, spec notify.RequestSpec) {
				t.Helper()
				assertContainsAll(t, workflowTextBlock(t, spec, "body"), "<h1>Deploy Summary</h1>", "<strong>Bold</strong>", "<pre><code class=\"language-go\">")
			},
		},
		{
			name:   "ntfy markdown",
			rawURL: "ntfy://example.com/topic?format=markdown&image=no",
			assertBody: func(t *testing.T, converted string) {
				t.Helper()
				assertContainsAll(t, converted, "**Bold**", "```go")
			},
			assertSpec: func(t *testing.T, spec notify.RequestSpec) {
				t.Helper()
				if spec.Headers["X-Markdown"] != "yes" {
					t.Fatalf("expected ntfy markdown header, got %#v", spec.Headers["X-Markdown"])
				}
				assertContainsAll(t, jsonStringField(t, spec.Body, "message"), "**Bold**", "```go")
			},
		},
		{
			name:   "discord markdown",
			rawURL: "discord://123456/token?format=markdown",
			assertBody: func(t *testing.T, converted string) {
				t.Helper()
				assertContainsAll(t, converted, "**Bold**", "```go")
			},
			assertSpec: func(t *testing.T, spec notify.RequestSpec) {
				t.Helper()
				assertContainsAll(t, discordEmbedDescription(t, spec), "**Bold**", "```go")
			},
		},
		{
			name:   "telegram markdown v1",
			rawURL: "tgram://123456:abcdef/7890/?format=markdown&mdv=v1",
			assertBody: func(t *testing.T, converted string) {
				t.Helper()
				assertContainsAll(t, converted, "*Deploy Summary*", "*Bold*", "_Italics_", "Strike", "[Docs](https://example.com/docs)", "```\nif x > 0 { return `tick` }\\path\n```", "- first", "- second")
				assertNotContains(t, converted, "~Strike~")
				assertNotContains(t, converted, "````")
			},
		},
		{
			name:   "telegram markdown v2",
			rawURL: "tgram://123456:abcdef/7890/?format=markdown&mdv=v2",
			assertBody: func(t *testing.T, converted string) {
				t.Helper()
				assertContainsAll(t, converted, "*Deploy Summary*", "*Bold*", "_Italics_", "[Docs](https://example.com/docs)", "```\nif x > 0 { return \\`tick\\` }\\\\path\n```", "\\- first", "\\- second")
				assertNotContains(t, converted, "````")
			},
		},
		{
			name:   "telegram html",
			rawURL: "tgram://123456:abcdef/7890/?format=html",
			assertBody: func(t *testing.T, converted string) {
				t.Helper()
				assertContainsAll(t, converted, "<b>Deploy Summary</b>", "<b>Bold</b>", "<i>Italics</i>", `<a href="https://example.com/docs">Docs</a>`, "<pre><code>if x &gt; 0 { return `tick` }\\path\n</code></pre>", "- first", "- second")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := notify.ParseURL(tc.rawURL)
			if err != nil {
				t.Fatalf("parse url: %v", err)
			}
			converted, err := notify.ConvertMessageFormatForTarget(parsed, body, "markdown")
			if err != nil {
				t.Fatalf("convert target body: %v", err)
			}
			tc.assertBody(t, converted)

			specs := testutil.CaptureGoRequests(t, func() error {
				return notify.SendTargetURL(tc.rawURL, body, "generalized parser", "markdown", notify.NotifyInfo)
			})
			if len(specs) != 1 {
				t.Fatalf("expected one request, got %d", len(specs))
			}
			if tc.assertSpec != nil {
				tc.assertSpec(t, specs[0])
			}
		})
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

func assertContainsAll(t *testing.T, value string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(value, fragment) {
			t.Fatalf("expected %q to contain %q", value, fragment)
		}
	}
}

func assertNotContains(t *testing.T, value, fragment string) {
	t.Helper()
	if strings.Contains(value, fragment) {
		t.Fatalf("expected %q not to contain %q", value, fragment)
	}
}

func jsonStringField(t *testing.T, body, field string) string {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode json body: %v", err)
	}
	value, ok := payload[field].(string)
	if !ok {
		t.Fatalf("expected string field %q in %#v", field, payload[field])
	}
	return value
}

func workflowTextBlock(t *testing.T, spec notify.RequestSpec, id string) string {
	t.Helper()
	var payload struct {
		Attachments []struct {
			Content struct {
				Body []map[string]any `json:"body"`
			} `json:"content"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal([]byte(spec.Body), &payload); err != nil {
		t.Fatalf("decode workflow body: %v", err)
	}
	for _, attachment := range payload.Attachments {
		for _, block := range attachment.Content.Body {
			if block["id"] == id {
				text, ok := block["text"].(string)
				if !ok {
					t.Fatalf("expected workflow block %q text to be a string: %#v", id, block["text"])
				}
				return text
			}
		}
	}
	t.Fatalf("workflow text block %q not found in %s", id, spec.Body)
	return ""
}

func discordEmbedDescription(t *testing.T, spec notify.RequestSpec) string {
	t.Helper()
	var payload struct {
		Embeds []struct {
			Description string `json:"description"`
		} `json:"embeds"`
	}
	if err := json.Unmarshal([]byte(spec.Body), &payload); err != nil {
		t.Fatalf("decode discord body: %v", err)
	}
	if len(payload.Embeds) != 1 {
		t.Fatalf("expected one discord embed, got %d", len(payload.Embeds))
	}
	return payload.Embeds[0].Description
}
