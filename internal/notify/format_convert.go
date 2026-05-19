package notify

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	nethtml "golang.org/x/net/html"
)

var htmlWSCondense = regexp.MustCompile(`[\s]+`)

var htmlBlockTags = map[string]struct{}{
	"p":     {},
	"h1":    {},
	"h2":    {},
	"h3":    {},
	"h4":    {},
	"h5":    {},
	"h6":    {},
	"div":   {},
	"td":    {},
	"th":    {},
	"code":  {},
	"pre":   {},
	"label": {},
	"li":    {},
}

var htmlIgnoreTags = map[string]struct{}{
	"form":     {},
	"input":    {},
	"textarea": {},
	"select":   {},
	"ul":       {},
	"ol":       {},
	"style":    {},
	"link":     {},
	"meta":     {},
	"title":    {},
	"html":     {},
	"head":     {},
	"script":   {},
}

type htmlChunk struct {
	text     string
	blockEnd bool
}

type htmlTextConverter struct {
	doStore bool
	result  []htmlChunk
}

func markdownToHTML(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}

	extensions := parser.CommonExtensions | parser.HardLineBreak
	mdParser := parser.NewWithExtensions(extensions)
	renderer := html.NewRenderer(html.RendererOptions{
		Flags: html.CommonFlags,
	})
	return strings.TrimSuffix(string(markdown.ToHTML([]byte(content), mdParser, renderer)), "\n")
}

func ConvertMessageFormat(content, inputFormat, outputFormat string) (string, error) {
	input := normalizeNotifyFormat(inputFormat)
	output := normalizeNotifyFormat(outputFormat)
	if input == "" {
		input = "text"
	}
	if output == "" {
		return content, nil
	}
	if !isSupportedNotifyFormat(input) {
		return "", fmt.Errorf("invalid input format: %s", inputFormat)
	}
	if !isSupportedNotifyFormat(output) {
		return "", fmt.Errorf("invalid output format: %s", outputFormat)
	}
	if input == output {
		return content, nil
	}

	switch input {
	case "markdown":
		if output == "html" {
			return markdownToHTML(content), nil
		}
		if output == "text" {
			return htmlToText(markdownToHTML(content)), nil
		}
	case "html":
		if output == "text" || output == "markdown" {
			return htmlToText(content), nil
		}
	case "text":
		if output == "html" {
			return textToHTML(content), nil
		}
	}

	return content, nil
}

func isSupportedNotifyFormat(format string) bool {
	switch format {
	case "html", "markdown", "text":
		return true
	default:
		return false
	}
}

func textToHTML(content string) string {
	return escapeHTML(content, true, true)
}

func htmlToText(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}

	converter := &htmlTextConverter{doStore: true}
	tokenizer := nethtml.NewTokenizer(strings.NewReader(content))

	for {
		switch tokenizer.Next() {
		case nethtml.ErrorToken:
			return converter.finalize()
		case nethtml.TextToken:
			converter.handleData(string(tokenizer.Text()))
		case nethtml.StartTagToken, nethtml.SelfClosingTagToken:
			name, _ := tokenizer.TagName()
			converter.handleStartTag(strings.ToLower(string(name)))
		case nethtml.EndTagToken:
			name, _ := tokenizer.TagName()
			converter.handleEndTag(strings.ToLower(string(name)))
		}
	}
}

func escapeHTML(value string, convertNewLines bool, whitespace bool) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}

	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	escaped := replacer.Replace(value)

	if whitespace {
		escaped = strings.ReplaceAll(escaped, "\t", "&emsp;")
		escaped = strings.ReplaceAll(escaped, " ", "&nbsp;")
	}

	if convertNewLines {
		return strings.ReplaceAll(escaped, "\n", "<br/>")
	}
	return escaped
}

func (c *htmlTextConverter) handleData(data string) {
	if !c.doStore {
		return
	}
	content := htmlWSCondense.ReplaceAllString(data, " ")
	if content == "" {
		return
	}
	c.result = append(c.result, htmlChunk{text: content})
}

func (c *htmlTextConverter) handleStartTag(tag string) {
	_, ignore := htmlIgnoreTags[tag]
	c.doStore = !ignore

	if _, ok := htmlBlockTags[tag]; ok {
		c.result = append(c.result, htmlChunk{blockEnd: true})
	}

	switch tag {
	case "li":
		c.result = append(c.result, htmlChunk{text: "- "})
	case "br":
		c.result = append(c.result, htmlChunk{text: "\n"})
	case "hr":
		if len(c.result) > 0 {
			last := c.result[len(c.result)-1]
			if !last.blockEnd {
				last.text = strings.TrimRight(last.text, " ")
				c.result[len(c.result)-1] = last
			}
		}
		c.result = append(c.result, htmlChunk{text: "\n---\n"})
	case "blockquote":
		c.result = append(c.result, htmlChunk{text: " >"})
	}
}

func (c *htmlTextConverter) handleEndTag(tag string) {
	c.doStore = true
	if _, ok := htmlBlockTags[tag]; ok {
		c.result = append(c.result, htmlChunk{blockEnd: true})
	}
}

func (c *htmlTextConverter) finalize() string {
	var out strings.Builder
	var accum string
	hasAccum := false

	for _, item := range c.result {
		if item.blockEnd {
			if !hasAccum {
				continue
			}
			out.WriteString(strings.TrimSpace(accum))
			out.WriteString("\n")
			accum = ""
			hasAccum = false
			continue
		}

		if hasAccum {
			accum += item.text
		} else {
			accum = item.text
			hasAccum = true
		}
	}

	if hasAccum {
		out.WriteString(strings.TrimSpace(accum))
	}

	return strings.TrimSpace(out.String())
}
