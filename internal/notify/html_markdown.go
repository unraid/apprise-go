package notify

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// htmlToMarkdown converts HTML to the CommonMark upstream produces. Upstream
// added a real converter in 1.12.0; before that both sides simply stripped
// tags, which lost every emphasis, link and list in an HTML notification.
//
// The constructs covered are the ones in TestHTMLToMarkdownMatchesUpstream.
// That corpus is the boundary of what is known to match; HTML outside it may
// still differ, so grow the corpus when a gap appears rather than guessing.
func htmlToMarkdown(content string) string {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		// Losing the markup is better than losing the message.
		return htmlToText(content)
	}

	var out bytes.Buffer
	walkHTMLNode(doc, &markdownState{out: &out})

	return strings.Trim(collapseBlankRuns(out.String()), "\n")
}

// markdownState carries the context a node needs from its ancestors: how deep
// a list is nested, whether we are inside preformatted text, and the ordered
// list counter.
type markdownState struct {
	out       *bytes.Buffer
	listDepth int
	ordered   bool
	counter   int
	inPre     bool
	inQuote   bool
}

func walkHTMLNode(node *html.Node, state *markdownState) {
	switch node.Type {
	case html.TextNode:
		text := node.Data
		if !state.inPre {
			text = collapseHTMLWhitespace(text)
			if text == "" {
				return
			}
			text = escapeMarkdown(text)
		}
		state.out.WriteString(text)
		return

	case html.ElementNode:
		writeHTMLElement(node, state)
		return
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkHTMLNode(child, state)
	}
}

func writeHTMLElement(node *html.Node, state *markdownState) {
	children := func(inner *markdownState) {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walkHTMLNode(child, inner)
		}
	}

	switch node.Data {
	case "b", "strong":
		state.out.WriteString("**")
		children(state)
		state.out.WriteString("**")

	case "i", "em":
		state.out.WriteString("*")
		children(state)
		state.out.WriteString("*")

	case "code":
		if state.inPre {
			children(state)
			return
		}
		state.out.WriteString("`")
		children(state)
		state.out.WriteString("`")

	case "a":
		state.out.WriteString("[")
		children(state)
		state.out.WriteString("](<" + attrValue(node, "href") + ">)")

	case "img":
		// Upstream drops images rather than emitting a markdown image.

	case "br":
		// Two trailing spaces is a markdown hard break.
		state.out.WriteString("  \n")

	case "hr":
		state.out.WriteString("\n\n---\n")

	case "p", "div":
		state.out.WriteString("\n\n")
		children(state)
		state.out.WriteString("\n\n")

	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(node.Data[1] - '0')
		// Trim back to a single newline so consecutive headings stay on
		// adjacent lines rather than gaining a blank line between them.
		trimTrailingNewlines(state.out, 1)
		state.out.WriteString(strings.Repeat("#", level) + " ")
		children(state)
		state.out.WriteString("\n")

	case "blockquote":
		nested := *state
		nested.inQuote = true
		state.out.WriteString("\n\n> ")
		children(&nested)
		state.out.WriteString("\n\n")

	case "pre":
		nested := *state
		nested.inPre = true
		state.out.WriteString("\n\n```\n")
		children(&nested)
		state.out.WriteString("\n```\n\n")

	case "ul", "ol":
		nested := *state
		nested.listDepth = state.listDepth + 1
		nested.ordered = node.Data == "ol"
		nested.counter = 0
		if state.listDepth == 0 {
			state.out.WriteString("\n\n")
		}
		children(&nested)
		if state.listDepth == 0 {
			state.out.WriteString("\n\n")
		}

	case "li":
		state.counter++
		indent := strings.Repeat("  ", max(state.listDepth-1, 0))
		marker := "- "
		if state.ordered {
			marker = fmt.Sprintf("%d. ", state.counter)
		}
		state.out.WriteString("\n" + indent + marker)
		children(state)

	default:
		children(state)
	}
}

func attrValue(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

// collapseHTMLWhitespace folds runs of whitespace the way an HTML renderer
// would, since newlines in the source are not line breaks in the output.
func collapseHTMLWhitespace(text string) string {
	var out strings.Builder
	space := false
	for _, r := range text {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			space = true
			continue
		}
		if space {
			out.WriteByte(' ')
		}
		space = false
		out.WriteRune(r)
	}
	if space {
		out.WriteByte(' ')
	}

	return out.String()
}

// escapeMarkdown escapes the characters that would otherwise be read as
// markup. Ampersands are left alone, matching upstream.
func escapeMarkdown(text string) string {
	replacer := strings.NewReplacer("<", `\<`, ">", `\>`)
	return replacer.Replace(text)
}

// collapseBlankRuns reduces the blank lines block elements emit to the single
// blank line markdown needs.
func collapseBlankRuns(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	blanks := 0

	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		// A line of only spaces is blank, but a hard break's two trailing
		// spaces belong to the line before it.
		if strings.TrimSpace(trimmed) == "" {
			blanks++
			continue
		}
		if blanks > 0 && len(out) > 0 {
			out = append(out, "")
		}
		blanks = 0
		out = append(out, line)
	}

	return strings.Join(out, "\n")
}

// trimTrailingNewlines leaves the buffer ending in exactly keep newlines,
// unless it is empty.
func trimTrailingNewlines(buf *bytes.Buffer, keep int) {
	trimmed := bytes.TrimRight(buf.Bytes(), "\n")
	if len(trimmed) == 0 {
		buf.Reset()
		return
	}

	buf.Truncate(len(trimmed))
	buf.WriteString(strings.Repeat("\n", keep))
}
