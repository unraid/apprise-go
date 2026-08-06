package notify

import "regexp"

// The four services that take over their own splitting upstream all do the
// same thing first: convert HTML-derived CommonMark into their own markdown
// dialect. Without it a body arrives carrying CommonMark the service does not
// speak — **bold** shown literally, and a link rendered as its raw syntax.
//
// Only reachable when the caller declares an HTML body for a markdown-native
// service, which is why it went unnoticed until a fixture set the input format.
var (
	// CommonMark strong is two markers; every dialect here uses one.
	commonMarkStrong = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)

	// A CommonMark link, whose destination upstream wraps in angle brackets.
	commonMarkLink = regexp.MustCompile(`\[([^\]\n]*)\]\(<?([^)>\s]+)>?\)`)

	// A single-backtick code span, which WhatsApp widens to three.
	commonMarkCodeSpan = regexp.MustCompile("`([^`\n]+)`")

	// CommonMark emphasis, once strong has already been consumed.
	commonMarkEmphasis = regexp.MustCompile(`(^|[^*])\*([^*\n]+)\*`)
)

// commonMarkToGoogleChat renders CommonMark in Google Chat's dialect: one
// asterisk for bold, and its anchor syntax for links.
func commonMarkToGoogleChat(body string) string {
	body = commonMarkLink.ReplaceAllString(body, "<$2|$1>")

	return commonMarkStrong.ReplaceAllString(body, "*$1*")
}

// commonMarkToSlack renders CommonMark in Slack's mrkdwn: one asterisk for
// bold, underscores for italic, and its anchor syntax for links.
func commonMarkToSlack(body string) string {
	body = commonMarkLink.ReplaceAllString(body, "<$2|$1>")
	body = commonMarkStrong.ReplaceAllString(body, "\x00$1\x00")
	body = commonMarkEmphasis.ReplaceAllString(body, "${1}_${2}_")

	return regexp.MustCompile("\x00").ReplaceAllString(body, "*")
}

// commonMarkToWhatsApp renders CommonMark in WhatsApp's dialect: one asterisk
// for bold, underscores for italic, and a bare label followed by the URL,
// since WhatsApp has no labeled-link syntax.
func commonMarkToWhatsApp(body string) string {
	body = commonMarkLink.ReplaceAllString(body, "$1 ($2)")

	// WhatsApp delimits code with three backticks, not one.
	body = commonMarkCodeSpan.ReplaceAllString(body, "```$1```")
	body = commonMarkStrong.ReplaceAllString(body, "\x00$1\x00")
	body = commonMarkEmphasis.ReplaceAllString(body, "${1}_${2}_")

	return regexp.MustCompile("\x00").ReplaceAllString(body, "*")
}
