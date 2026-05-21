package notify

import (
	"fmt"
	"html"
	"strings"

	nethtml "golang.org/x/net/html"
)

var telegramHTMLBlockTags = map[string]struct{}{
	"blockquote": {},
	"div":        {},
	"h1":         {},
	"h2":         {},
	"h3":         {},
	"h4":         {},
	"h5":         {},
	"h6":         {},
	"li":         {},
	"p":          {},
	"pre":        {},
}

func convertTelegramMessageFormat(content, inputFormat, outputFormat, markdownVersion string) (string, error) {
	input := normalizeNotifyFormat(inputFormat)
	output := normalizeNotifyFormat(outputFormat)
	if input == "" {
		input = "text"
	}
	if output == "" {
		output = "html"
	}
	if !isSupportedNotifyFormat(input) {
		return "", fmt.Errorf("invalid input format: %s", inputFormat)
	}
	if !isSupportedNotifyFormat(output) {
		return "", fmt.Errorf("invalid output format: %s", outputFormat)
	}

	switch output {
	case "html":
		switch input {
		case "markdown":
			return telegramHTMLFromHTML(markdownToHTML(content)), nil
		case "html":
			return telegramHTMLFromHTML(content), nil
		case "text":
			return html.EscapeString(content), nil
		}
	case "markdown":
		mode := telegramMarkdownMode(markdownVersion)
		switch input {
		case "markdown":
			return telegramMarkdownFromHTML(markdownToHTML(content), mode), nil
		case "html":
			return telegramMarkdownFromHTML(content, mode), nil
		case "text":
			return escapeTelegramMarkdownText(content, mode), nil
		}
	case "text":
		return ConvertMessageFormat(content, input, output)
	}

	return content, nil
}

func telegramHTMLFromHTML(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	root, err := nethtml.Parse(strings.NewReader(content))
	if err != nil {
		return html.EscapeString(content)
	}

	var out strings.Builder
	renderTelegramHTMLNode(&out, root, false)
	return strings.Trim(out.String(), "\n")
}

func renderTelegramHTMLNode(out *strings.Builder, node *nethtml.Node, inCode bool) {
	if node.Type == nethtml.TextNode {
		if !inCode && strings.TrimSpace(node.Data) == "" && strings.ContainsAny(node.Data, "\r\n") {
			return
		}
		out.WriteString(html.EscapeString(node.Data))
		return
	}
	if node.Type != nethtml.ElementNode {
		renderTelegramHTMLChildren(out, node, inCode)
		return
	}

	tag := strings.ToLower(node.Data)
	if _, ok := telegramHTMLBlockTags[tag]; ok && out.Len() > 0 {
		ensureLineBreak(out)
	}

	switch tag {
	case "a":
		href := htmlAttr(node, "href")
		if href == "" {
			renderTelegramHTMLChildren(out, node, inCode)
			break
		}
		out.WriteString(`<a href="`)
		out.WriteString(html.EscapeString(href))
		out.WriteString(`">`)
		renderTelegramHTMLChildren(out, node, inCode)
		out.WriteString("</a>")
	case "b", "strong":
		out.WriteString("<b>")
		renderTelegramHTMLChildren(out, node, inCode)
		out.WriteString("</b>")
	case "blockquote":
		out.WriteString("<blockquote>")
		renderTelegramHTMLChildren(out, node, inCode)
		out.WriteString("</blockquote>")
	case "br":
		ensureLineBreak(out)
	case "code":
		out.WriteString("<code>")
		renderTelegramHTMLChildren(out, node, true)
		out.WriteString("</code>")
	case "del", "s", "strike":
		out.WriteString("<s>")
		renderTelegramHTMLChildren(out, node, inCode)
		out.WriteString("</s>")
	case "em", "i":
		out.WriteString("<i>")
		renderTelegramHTMLChildren(out, node, inCode)
		out.WriteString("</i>")
	case "h1", "h2", "h3", "h4", "h5", "h6":
		out.WriteString("<b>")
		renderTelegramHTMLChildren(out, node, inCode)
		out.WriteString("</b>")
	case "li":
		out.WriteString("- ")
		renderTelegramHTMLChildren(out, node, inCode)
	case "pre":
		out.WriteString("<pre>")
		renderTelegramHTMLChildren(out, node, true)
		out.WriteString("</pre>")
	case "u", "ins":
		out.WriteString("<u>")
		renderTelegramHTMLChildren(out, node, inCode)
		out.WriteString("</u>")
	default:
		renderTelegramHTMLChildren(out, node, inCode)
	}

	if _, ok := telegramHTMLBlockTags[tag]; ok {
		ensureLineBreak(out)
	}
}

func renderTelegramHTMLChildren(out *strings.Builder, node *nethtml.Node, inCode bool) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		renderTelegramHTMLNode(out, child, inCode)
	}
}

func telegramMarkdownFromHTML(content, markdownMode string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	root, err := nethtml.Parse(strings.NewReader(content))
	if err != nil {
		return escapeTelegramMarkdownText(content, markdownMode)
	}

	var out strings.Builder
	renderTelegramMarkdownNode(&out, root, markdownMode, false)
	return strings.Trim(out.String(), "\n")
}

func renderTelegramMarkdownNode(out *strings.Builder, node *nethtml.Node, markdownMode string, inCode bool) {
	if node.Type == nethtml.TextNode {
		if inCode {
			out.WriteString(escapeTelegramMarkdownCodeText(node.Data, markdownMode))
			return
		}
		if strings.TrimSpace(node.Data) == "" && strings.ContainsAny(node.Data, "\r\n") {
			return
		}
		out.WriteString(escapeTelegramMarkdownText(node.Data, markdownMode))
		return
	}
	if node.Type != nethtml.ElementNode {
		renderTelegramMarkdownChildren(out, node, markdownMode, inCode)
		return
	}

	tag := strings.ToLower(node.Data)
	if _, ok := telegramHTMLBlockTags[tag]; ok && out.Len() > 0 {
		ensureLineBreak(out)
	}

	switch tag {
	case "a":
		href := htmlAttr(node, "href")
		if href == "" {
			renderTelegramMarkdownChildren(out, node, markdownMode, inCode)
			break
		}
		out.WriteString("[")
		renderTelegramMarkdownChildren(out, node, markdownMode, inCode)
		out.WriteString("](")
		out.WriteString(escapeTelegramMarkdownLink(href, markdownMode))
		out.WriteString(")")
	case "b", "strong":
		out.WriteString("*")
		renderTelegramMarkdownChildren(out, node, markdownMode, inCode)
		out.WriteString("*")
	case "br":
		ensureLineBreak(out)
	case "code":
		if inCode {
			renderTelegramMarkdownChildren(out, node, markdownMode, true)
			break
		}
		out.WriteString("`")
		renderTelegramMarkdownChildren(out, node, markdownMode, true)
		out.WriteString("`")
	case "del", "s", "strike":
		if markdownMode == "MarkdownV2" {
			out.WriteString("~")
		}
		renderTelegramMarkdownChildren(out, node, markdownMode, inCode)
		if markdownMode == "MarkdownV2" {
			out.WriteString("~")
		}
	case "em", "i":
		out.WriteString("_")
		renderTelegramMarkdownChildren(out, node, markdownMode, inCode)
		out.WriteString("_")
	case "h1", "h2", "h3", "h4", "h5", "h6":
		out.WriteString("*")
		renderTelegramMarkdownChildren(out, node, markdownMode, inCode)
		out.WriteString("*")
	case "li":
		if markdownMode == "MarkdownV2" {
			out.WriteString("\\- ")
		} else {
			out.WriteString("- ")
		}
		renderTelegramMarkdownChildren(out, node, markdownMode, inCode)
	case "pre":
		out.WriteString("```\n")
		renderTelegramMarkdownChildren(out, node, markdownMode, true)
		out.WriteString("```")
	default:
		renderTelegramMarkdownChildren(out, node, markdownMode, inCode)
	}

	if _, ok := telegramHTMLBlockTags[tag]; ok {
		ensureLineBreak(out)
	}
}

func renderTelegramMarkdownChildren(out *strings.Builder, node *nethtml.Node, markdownMode string, inCode bool) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		renderTelegramMarkdownNode(out, child, markdownMode, inCode)
	}
}

func escapeTelegramMarkdownCodeText(value, markdownMode string) string {
	if markdownMode != "MarkdownV2" {
		return value
	}
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"`", "\\`",
	)
	return replacer.Replace(value)
}

func escapeTelegramMarkdownLink(value, markdownMode string) string {
	if markdownMode != "MarkdownV2" {
		return value
	}
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		")", "\\)",
	)
	return replacer.Replace(value)
}

func escapeTelegramMarkdownText(value, markdownMode string) string {
	if markdownMode == "MarkdownV2" {
		replacer := strings.NewReplacer(
			"\\", "\\\\",
			"_", "\\_",
			"*", "\\*",
			"[", "\\[",
			"]", "\\]",
			"(", "\\(",
			")", "\\)",
			"~", "\\~",
			"`", "\\`",
			">", "\\>",
			"#", "\\#",
			"+", "\\+",
			"-", "\\-",
			"=", "\\=",
			"|", "\\|",
			"{", "\\{",
			"}", "\\}",
			".", "\\.",
			"!", "\\!",
		)
		return replacer.Replace(value)
	}

	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"*", "\\*",
		"_", "\\_",
		"`", "\\`",
		"[", "\\[",
	)
	return replacer.Replace(value)
}

func htmlAttr(node *nethtml.Node, key string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}

func ensureLineBreak(out *strings.Builder) {
	if out.Len() == 0 {
		return
	}
	value := out.String()
	if value[len(value)-1] != '\n' {
		out.WriteByte('\n')
	}
}
