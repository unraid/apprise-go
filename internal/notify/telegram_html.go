package notify

import (
	"regexp"
	"strings"
)

// telegramHTMLRewrite converts an HTML body into the small tag set Telegram's
// HTML parse mode accepts.
//
// Telegram supports b, i, u, s, a, code and pre and nothing else — a <br> or
// an <h1> is not ignored, it is rejected. Upstream carries a rewrite table for
// this; the port had none, so any HTML body carrying an unsupported tag went
// out unconverted. It went unnoticed because no fixture body contained one
// until the overflow work started folding a title in as <b>…</b><br />.
//
// Ordered: each rule sees what the ones before it produced.
var telegramHTMLRewrite = []struct {
	pattern *regexp.Regexp
	replace string
}{
	// Comments.
	{regexp.MustCompile(`(?is)\s*<!.+?-->\s*`), ""},

	// Tags with no Telegram equivalent are dropped along with their padding.
	{regexp.MustCompile(`(?is)\s*<\s*(!?DOCTYPE|p|div|span|body|script|link|` +
		`meta|html|font|head|label|form|input|textarea|select|iframe|` +
		`source)([^a-z0-9>][^>]*)?>\s*`), ""},
	{regexp.MustCompile(`(?is)\s*<\s*/(span|body|script|meta|html|font|head|` +
		`label|form|input|textarea|select|ol|ul|link|iframe|source)` +
		`([^a-z0-9>][^>]*)?>\s*`), ""},

	// Bold.
	{regexp.MustCompile(`(?is)<\s*(strong)([^a-z0-9>][^>]*)?>`), "<b>"},
	{regexp.MustCompile(`(?is)<\s*/\s*(strong)([^a-z0-9>][^>]*)?>`), "</b>"},

	// Headings become bold, with a line break where the block would have been.
	{regexp.MustCompile(`(?is)\s*<\s*(h[1-6]|title)([^a-z0-9>][^>]*)?>\s*`), "\r\n<b>"},
	{regexp.MustCompile(`(?is)\s*<\s*/\s*(h[1-6]|title)([^a-z0-9>][^>]*)?>\s*`), "</b>\r\n"},

	// Italic.
	{regexp.MustCompile(`(?is)<\s*(caption|em)([^a-z0-9>][^>]*)?>`), "<i>"},
	{regexp.MustCompile(`(?is)<\s*/\s*(caption|em)([^a-z0-9>][^>]*)?>`), "</i>"},

	// Bullets.
	{regexp.MustCompile(`(?is)<\s*li([^a-z0-9>][^>]*)?>\s*`), " -"},

	// Anything that meant a line break becomes one.
	{regexp.MustCompile(`(?is)\s*<\s*/?\s*(ol|ul|br|hr)\s*/?>\s*`), "\r\n"},
	{regexp.MustCompile(`(?is)\s*<\s*/\s*(br|p|hr|li|div)([^a-z0-9>][^>]*)?>\s*`), "\r\n"},

	// Entities Telegram does not accept.
	{regexp.MustCompile(`(?i)&nbsp;?`), " "},
	{regexp.MustCompile(`(?i)&emsp;?`), "   "},
	{regexp.MustCompile(`(?i)&apos;?`), "'"},
	{regexp.MustCompile(`(?i)&quot;?`), `"`},

	// Collapse the runs the rules above tend to leave behind.
	{regexp.MustCompile(`(?i)\r*\n[\r\n]+`), "\r\n"},
}

// rewriteTelegramHTML applies the table in order.
func rewriteTelegramHTML(body string) string {
	for _, rule := range telegramHTMLRewrite {
		body = rule.pattern.ReplaceAllString(body, rule.replace)
	}

	return strings.TrimSpace(body)
}
