package notify

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Overflow decides what happens to a body longer than the service accepts.
//
// The default is upstream: leave the content alone and let the service deal
// with it. That is what this port did for everything before overflow existed
// here, and it is why the default path never diverged.
const (
	OverflowUpstream = "upstream"
	OverflowTruncate = "truncate"
	OverflowSplit    = "split"
)

// overflowLimits are the per-service limits upstream's base class works from.
// They are class attributes rather than URL arguments, so they appear in no
// schema entry — see testutil/scripts/overflow_limits.py, which is where this
// table comes from.
type overflowLimits struct {
	// bodyMax is the longest body the service accepts. -1 means upstream
	// computes it per instance rather than declaring a constant.
	bodyMax int

	// titleMax is the longest title. Zero means the service has no title at
	// all, so the framework folds it into the body.
	titleMax int

	// lineMax caps how many lines a body may have, applied before any
	// splitting.
	lineMax int

	// amalgamateTitle repeats the title on each chunk rather than sending it
	// once on the first.
	amalgamateTitle bool

	// buffer is room reserved for the separator added when a title is folded
	// into the body.
	buffer int

	// countThreshold is the body size below which a repeated title is shown
	// once instead, and maxCountWidth is the widest counter worth adding.
	countThreshold int
	maxCountWidth  int

	// displayTitleOnce overrides the threshold when a service sets it.
	displayTitleOnce *bool

	// format is the service's own notify format, which decides how a title
	// is folded into the body. ?format= overrides it.
	format string
}

// MessagePart is one notification's worth of content after overflow has been
// applied. A split body becomes several.
type MessagePart struct {
	Title string
	Body  string
}

// ApplyOverflow splits or truncates a body to the service's limits, returning
// the parts to send. The default mode returns the content untouched, which is
// what every provider here did before this existed.
func ApplyOverflow(schema, mode, format, title, body string) []MessagePart {
	return applyOverflowWithLimits(schema, mode, format, "", title, body, nil)
}

// ApplyOverflowForURL is ApplyOverflow for a service whose limits depend on
// its URL. Eight upstream plugins compute body_maxlen or title_maxlen from an
// argument rather than declaring a constant — twilio's depends on ?method=,
// webex's on whether it is a webhook — so the generated table records them as
// unknown and they are resolved here instead.
func ApplyOverflowForURL(target *ParsedURL, mode, format, title, body string) []MessagePart {
	return ApplyOverflowForURLWithInput(target, mode, format, "", title, body)
}

// ApplyOverflowForURLWithInput additionally takes the format the caller
// declared the body was in. Upstream only renders a folded title as a markdown
// heading when that input was text or HTML; a body already in the service's
// own format gets the plain separator, so the input cannot be inferred from
// the service.
func ApplyOverflowForURLWithInput(target *ParsedURL, mode, format, inputFormat, title, body string) []MessagePart {
	schema := ""
	if target != nil {
		schema = target.Scheme
	}

	// A Telegram Rich Message template defines its own structure -- body and
	// title are only substitution tokens for it, so upstream's gen_calls
	// hands them over untouched: no conversion, no fold, no split.
	if target != nil && strings.EqualFold(schema, "tgram") &&
		strings.TrimSpace(target.Query["template"]) != "" {
		return []MessagePart{{Title: title, Body: body}}
	}

	return applyOverflowWithLimits(schema, mode, format, inputFormat, title, body, target)
}

func applyOverflowWithLimits(schema, mode, format, inputFormat, title, body string, target *ParsedURL) []MessagePart {
	limits, known := overflowLimitsFor(schema)
	if resolved, ok := dynamicOverflowLimits(schema, target, limits); ok {
		limits, known = resolved, true
	}

	// A service has its own format; ?format= overrides it. The fold depends
	// on it, so telegram's HTML default gives a different separator from
	// discord's text one.
	if strings.TrimSpace(format) == "" {
		format = limits.format
	}

	title = strings.TrimSpace(title)
	body = strings.TrimRight(body, " \t\r\n")

	// A line cap applies whatever the mode, since upstream enforces it before
	// it looks at overflow at all.
	if known && limits.lineMax > 0 {
		lines := overflowLineBreak.Split(body, -1)
		if len(lines) > limits.lineMax {
			lines = lines[:limits.lineMax]
		}
		body = strings.Join(lines, "\r\n")
	}

	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = OverflowUpstream
	}
	// A service with no title of its own has the title folded into the body
	// before anything is measured, and before the mode is consulted, and the title is cleared. Measuring the
	// body on its own and letting the provider fold afterwards produces
	// chunks that are longer than the limit by the width of the title.
	if limits.titleMax <= 0 && title != "" {
		// The bold line is part of the override path, which upstream only
		// takes for a markdown-native service handed HTML. By default the
		// framework fold applies here too.
		markdownOut := strings.EqualFold(strings.TrimSpace(format), "markdown") &&
			strings.EqualFold(strings.TrimSpace(inputFormat), "html")
		if markdownOut && boldTitleSchemas[strings.ToLower(strings.TrimSpace(schema))] {
			// WhatsApp has no heading syntax, so upstream uses a bold line.
			body = "**" + strings.TrimLeft(title, "\r\n \t\v\f#-") + "**\n" + body
			title = ""
		} else {
			body = foldTitleIntoBody(title, body, format, inputFormat)
		}
		if title != "" {
			title = ""
		}
	}

	if mode == OverflowUpstream || !known || limits.bodyMax < 0 {
		return []MessagePart{{Title: title, Body: body}}
	}

	// The buffer is only reserved while a title still needs folding, which
	// after the step above it never does.
	buffer := limits.buffer

	// A service that repeats the title on every chunk sizes the title against
	// the room a counter would need, not against its own maximum.
	titleMax := limits.titleMax
	if limits.amalgamateTitle {
		titleMax = minInt(minInt(
			len(title)+limits.maxCountWidth, limits.titleMax), limits.bodyMax)
	}
	if len(title) > titleMax {
		title = strings.TrimRight(title[:titleMax], " \t\r\n")
	}

	bodyMax := limits.bodyMax
	if limits.amalgamateTitle && limits.bodyMax-buffer >= titleMax {
		bodyMax = limits.bodyMax
		if title != "" {
			bodyMax = limits.bodyMax - titleMax
		}
		bodyMax -= buffer
	} else if limits.amalgamateTitle {
		bodyMax = limits.bodyMax - buffer
	}

	if bodyMax > 0 && len(body) <= bodyMax {
		return []MessagePart{{Title: title, Body: body}}
	}

	if mode == OverflowTruncate {
		cut := body
		if bodyMax > 0 && len(cut) > bodyMax {
			cut = cut[:bodyMax]
		}

		return []MessagePart{{Title: title, Body: trimChunk(cut)}}
	}

	// Whether the title rides on every chunk or only the first.
	titleOnce := limits.amalgamateTitle && bodyMax < limits.countThreshold
	if limits.displayTitleOnce != nil {
		titleOnce = *limits.displayTitleOnce
	}

	if titleOnce || (limits.amalgamateTitle && bodyMax <= 0) {
		return splitTitleOnce(body, title, bodyMax, format)
	}

	// A counter is only worth showing when there is room for it in the title
	// and the service's title is long enough to carry one.
	showCounter := title != "" && len(body) > bodyMax &&
		((limits.amalgamateTitle && bodyMax >= limits.countThreshold) ||
			(!limits.amalgamateTitle && titleMax > limits.countThreshold)) &&
		titleMax > limits.maxCountWidth+buffer &&
		limits.titleMax >= limits.countThreshold

	chunkMax := bodyMax
	if showCounter {
		chunkMax -= buffer
	}

	chunks := smartSplit(body, chunkMax, format)

	template := ""
	if showCounter {
		digits := len(fmt.Sprintf("%d", len(chunks)))
		width := 4 + digits*2
		if width <= limits.maxCountWidth {
			if room := titleMax - width; len(title) > room && room >= 0 {
				title = title[:room]
			}
			template = fmt.Sprintf(" [%%0%dd/%%0%dd]", digits, digits)
		} else {
			showCounter = false
		}
	}

	parts := make([]MessagePart, 0, len(chunks))
	for index, chunk := range chunks {
		suffix := ""
		if showCounter {
			suffix = fmt.Sprintf(template, index+1, len(chunks))
		}
		parts = append(parts, MessagePart{
			Title: title + suffix,
			Body:  trimChunk(chunk),
		})
	}

	if len(parts) == 0 {
		return []MessagePart{{Title: title, Body: ""}}
	}

	return parts
}

// foldTitleIntoBody merges a title into the body for a service that has no
// title of its own, the way upstream's base class does. The separator depends
// on the format the body is being sent in.
func foldTitleIntoBody(title, body, format, inputFormat string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "html":
		return "<b>" + title + "</b><br />\r\n" + body
	case "markdown":
		// A heading only when the caller's body was text or HTML. Markdown
		// in, markdown out gets the plain separator.
		switch strings.ToLower(strings.TrimSpace(inputFormat)) {
		case "text", "html":
		default:
			return title + "\r\n" + body
		}

		trimmed := strings.TrimLeft(title, "\r\n \t\v\f#-")
		if trimmed == "" {
			return body
		}

		return "# " + trimmed + "\n" + body
	default:
		return title + "\r\n" + body
	}
}

// splitTitleOnce puts the title on the first chunk and leaves the rest
// untitled, which is what a service with a short body limit does rather than
// repeating a title that would crowd out the message.
func splitTitleOnce(body, title string, bodyMax int, format string) []MessagePart {
	if bodyMax <= 0 || body == "" {
		return []MessagePart{{Title: title, Body: ""}}
	}

	first := smartSplit(body, bodyMax, format)
	if len(first) == 0 {
		return []MessagePart{{Title: title, Body: ""}}
	}

	parts := []MessagePart{{Title: title, Body: trimChunk(first[0])}}
	for _, chunk := range smartSplit(body[len(first[0]):], bodyMax, format) {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		parts = append(parts, MessagePart{Body: trimChunk(chunk)})
	}

	return parts
}

func trimChunk(chunk string) string {
	return strings.TrimRight(strings.TrimLeft(chunk, "\r\n\v\f"), " \t\r\n")
}

var overflowLineBreak = regexp.MustCompile(`\r*\n`)

// overflowPunctuation matches the punctuation-then-space boundary smart
// splitting prefers when there is no newline or space to break on.
var overflowPunctuation = regexp.MustCompile(`[.!?,;:][ \t\r\n]`)

// smartSplit breaks text into chunks no longer than limit, preferring to break
// at a newline, then a space or tab, then punctuation followed by whitespace,
// and only cutting mid-word when none of those is available.
func smartSplit(text string, limit int, format string) []string {
	_ = format

	if text == "" || limit <= 0 {
		return []string{""}
	}

	result := []string{}
	start := 0
	for start < len(text) {
		if len(text)-start <= limit {
			result = append(result, text[start:])
			break
		}

		windowEnd := start + limit
		window := text[start:windowEnd]

		splitAt := -1
		if index := strings.LastIndexAny(window, "\r\n"); index >= 0 {
			splitAt = start + index + 1
		} else if index := strings.LastIndexAny(window, " \t"); index >= 0 {
			splitAt = start + index + 1
		} else if matches := overflowPunctuation.FindAllStringIndex(window, -1); len(matches) > 0 {
			splitAt = start + matches[len(matches)-1][1]
		}

		if splitAt <= start {
			// Nothing to break on, so cut at the limit.
			splitAt = windowEnd
		}

		result = append(result, text[start:splitAt])
		start = splitAt
	}

	return result
}

func overflowLimitsFor(schema string) (overflowLimits, bool) {
	limits, ok := overflowLimitsBySchema[strings.ToLower(strings.TrimSpace(schema))]

	return limits, ok
}

// OverflowSchemaKnown reports whether limits are recorded for a schema, so a
// test can tell a missing entry from a service with no limit.
func OverflowSchemaKnown(schema string) bool {
	_, ok := overflowLimitsFor(schema)

	return ok
}

// overflowLimitsBySchema is generated from upstream by
// testutil/scripts/overflow_limits.py and checked against it by
// TestOverflowLimitsMatchUpstream.
var overflowLimitsBySchema = map[string]overflowLimits{
	"46elks":     {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"apprise":    {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"apprises":   {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"aprs":       {bodyMax: 67, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"atalk":      {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"azure":      {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "html"},
	"bark":       {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"barks":      {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"blink1":     {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"bluesky":    {bodyMax: 280, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"brevo":      {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "html"},
	"bsky":       {bodyMax: 280, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"bulksms":    {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"bulkvs":     {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"burstsms":   {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"chanify":    {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"chime":      {bodyMax: 4096, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "markdown"},
	"clickatell": {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"clicksend":  {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"d7sms":      {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"dapnet":     {bodyMax: 80, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"dbus":       {bodyMax: 32768, titleMax: 250, lineMax: 10, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"dingtalk":   {bodyMax: 32768, titleMax: -1, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"discord":    {bodyMax: 2000, titleMax: 250, lineMax: 0, amalgamateTitle: true, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"dot":        {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"eight00com": {bodyMax: 600, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"elks":       {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"emby":       {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"embys":      {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"enigma2":    {bodyMax: 1000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"enigma2s":   {bodyMax: 1000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"evolution":  {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "markdown"},
	"evolutions": {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "markdown"},
	"exotel":     {bodyMax: 2000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"fcm":        {bodyMax: 1024, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"feishu":     {bodyMax: 19985, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"flock":      {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"flowtriq":   {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"flowtriqs":  {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"fluxer":     {bodyMax: 2000, titleMax: 250, lineMax: 0, amalgamateTitle: true, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"fluxers":    {bodyMax: 2000, titleMax: 250, lineMax: 0, amalgamateTitle: true, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"form":       {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"forms":      {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"freemobile": {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"gchat":      {bodyMax: 4000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "markdown"},
	"gio":        {bodyMax: 32768, titleMax: 250, lineMax: 10, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"glib":       {bodyMax: 32768, titleMax: 250, lineMax: 10, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"gnome":      {bodyMax: 32768, titleMax: 0, lineMax: 10, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"gotify":     {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"gotifys":    {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"groupme":    {bodyMax: 1000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"growl":      {bodyMax: 32768, titleMax: 250, lineMax: 2, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"guilded":    {bodyMax: 2000, titleMax: 250, lineMax: 0, amalgamateTitle: true, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"hassio":     {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"hassios":    {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"httpsms":    {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"humhub":     {bodyMax: 4000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"humhubs":    {bodyMax: 4000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"ifttt":      {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"irc":        {bodyMax: 380, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"ircs":       {bodyMax: 380, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"jellyfin":   {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"jellyfins":  {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"jira":       {bodyMax: 15000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"join":       {bodyMax: 1000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"json":       {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"jsons":      {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"kavenegar":  {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"kde":        {bodyMax: 32768, titleMax: 250, lineMax: 10, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"kodi":       {bodyMax: 32768, titleMax: 250, lineMax: 2, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"kodis":      {bodyMax: 32768, titleMax: 250, lineMax: 2, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"kook":       {bodyMax: 5000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "markdown"},
	"kumulos":    {bodyMax: 240, titleMax: 64, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"lametric":   {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"lametrics":  {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"lark":       {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"lauther":    {bodyMax: 2000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"line":       {bodyMax: 5000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"macosx":     {bodyMax: 32768, titleMax: 250, lineMax: 10, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"mailersend": {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "html"},
	"mailgun":    {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "html"},
	"mailto":     {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "html"},
	"mailtos":    {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "html"},
	"mastodon":   {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"mastodons":  {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"matrix":     {bodyMax: -1, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"matrixs":    {bodyMax: -1, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"misskey":    {bodyMax: 512, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"misskeys":   {bodyMax: 512, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"mmost":      {bodyMax: 4000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"mmosts":     {bodyMax: 4000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"mqtt":       {bodyMax: 268435455, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"mqtts":      {bodyMax: 268435455, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"msg91":      {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"msgbird":    {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"ncloud":     {bodyMax: 4000, titleMax: 255, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"nclouds":    {bodyMax: 4000, titleMax: 255, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"nctalk":     {bodyMax: 4000, titleMax: 255, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"nctalks":    {bodyMax: 4000, titleMax: 255, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"nexmo":      {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"notica":     {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"noticas":    {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"notifiarr":  {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"notifico":   {bodyMax: 512, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"notificos":  {bodyMax: 512, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"notifyre":   {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"ntfy":       {bodyMax: 7800, titleMax: 200, lineMax: 0, amalgamateTitle: true, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"ntfys":      {bodyMax: 7800, titleMax: 200, lineMax: 0, amalgamateTitle: true, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"o365":       {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "html"},
	"octopush":   {bodyMax: 1224, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"onesignal":  {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"opsgenie":   {bodyMax: 15000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"pagerduty":  {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"pagertree":  {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"parsep":     {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"parseps":    {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"pbul":       {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"pinglet":    {bodyMax: 3000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"pinglets":   {bodyMax: 3000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"pingram":    {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"pjet":       {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"pjets":      {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"plivo":      {bodyMax: 140, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"postmark":   {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "html"},
	"pover":      {bodyMax: 1024, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"prowl":      {bodyMax: 10000, titleMax: 1024, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"psafer":     {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"psafers":    {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"push":       {bodyMax: 1000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"pushdeer":   {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"pushdeers":  {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"pushed":     {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"pushme":     {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"pushplus":   {bodyMax: 20000, titleMax: 200, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "html"},
	"pushward":   {bodyMax: 3000, titleMax: 256, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"pushy":      {bodyMax: 4096, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"qq":         {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"qt":         {bodyMax: 32768, titleMax: 250, lineMax: 10, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"reddit":     {bodyMax: 6000, titleMax: 300, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "markdown"},
	"resend":     {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "html"},
	"revolt":     {bodyMax: 2000, titleMax: 100, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"ringc":      {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"rocket":     {bodyMax: 1000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "markdown"},
	"rockets":    {bodyMax: 1000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "markdown"},
	"rsyslog":    {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"ryver":      {bodyMax: 1000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"schan":      {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"sendgrid":   {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "html"},
	"sendpulse":  {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "html"},
	"serwersms":  {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"ses":        {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "html"},
	"session":    {bodyMax: 2000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"sessions":   {bodyMax: 2000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"seven":      {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"sfr":        {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"signal":     {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"signals":    {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"signalgrid": {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"signl4":     {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"sinch":      {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"slack":      {bodyMax: 35000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "markdown"},
	"smpp":       {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"smpps":      {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"smsc":       {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"smseagle":   {bodyMax: 1200, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"smseagles":  {bodyMax: 1200, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"smsmanager": {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"smsmgr":     {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"smtp2go":    {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "html"},
	"sogs":       {bodyMax: 2000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"sparkpost":  {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "html"},
	"spike":      {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"splunk":     {bodyMax: 400, titleMax: 60, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"spugpush":   {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"spush":      {bodyMax: 10000, titleMax: 1024, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"stackfield": {bodyMax: 4000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"strmlabs":   {bodyMax: 255, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"synology":   {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"synologys":  {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"syslog":     {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"tgram":      {bodyMax: 4096, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "html"},
	"threema":    {bodyMax: 3500, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"toot":       {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"toots":      {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"tweet":      {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"trigv":      {bodyMax: 1000, titleMax: 255, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"trigvs":     {bodyMax: 1000, titleMax: 255, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"twilio":     {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"twist":      {bodyMax: 1000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "markdown"},
	"twitter":    {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"vapid":      {bodyMax: 4000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"viber":      {bodyMax: 30000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"victorops":  {bodyMax: 400, titleMax: 60, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"voipms":     {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"vonage":     {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"webex":      {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "markdown"},
	"wechat":     {bodyMax: 2048, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"wecom":      {bodyMax: 20000, titleMax: 200, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "html"},
	"wecombot":   {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"whatsapp":   {bodyMax: 1024, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"windows":    {bodyMax: 32768, titleMax: 250, lineMax: 2, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"workflow":   {bodyMax: 1000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "markdown"},
	"workflows":  {bodyMax: 1000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "markdown"},
	"wxpusher":   {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"wxteams":    {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "markdown"},
	"x":          {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"xbmc":       {bodyMax: 32768, titleMax: 250, lineMax: 2, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"xbmcs":      {bodyMax: 32768, titleMax: 250, lineMax: 2, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"xml":        {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"xmls":       {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"xmpp":       {bodyMax: 32768, titleMax: -1, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"xmpps":      {bodyMax: 32768, titleMax: -1, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"zoom":       {bodyMax: 4000, titleMax: 250, lineMax: 0, amalgamateTitle: true, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"zulip":      {bodyMax: 10000, titleMax: 60, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
}

// OverflowLimits is the exported view of a service's limits, so the parity
// suite can compare the generated table against upstream.
type OverflowLimits struct {
	// Format is the service's own notify format, which decides both how a
	// title is folded and what an incoming body is converted to.
	Format string

	BodyMax         int
	TitleMax        int
	LineMax         int
	AmalgamateTitle bool
	Buffer          int
}

// OverflowLimitsFor returns the recorded limits for a schema.
func OverflowLimitsFor(schema string) OverflowLimits {
	limits, _ := overflowLimitsFor(schema)

	return OverflowLimits{
		Format:          limits.format,
		BodyMax:         limits.bodyMax,
		TitleMax:        limits.titleMax,
		LineMax:         limits.lineMax,
		AmalgamateTitle: limits.amalgamateTitle,
		Buffer:          limits.buffer,
	}
}

// parseTimezone resolves a ?tz= value to a location, ignoring one the system
// does not know rather than failing the send — which is what upstream does
// with an unrecognized zone.
func parseTimezone(raw string) *time.Location {
	name := strings.TrimSpace(raw)
	if name == "" {
		return nil
	}

	location, err := time.LoadLocation(name)
	if err != nil {
		return nil
	}

	return location
}

// dynamicOverflowLimits fills in the limits upstream computes per instance.
// Each is derived from a single URL argument, so resolving them needs the
// parsed URL rather than the schema alone.
//
// Reported as unknown by overflow_limits.py, since a property cannot be read
// off a class — which is why they were skipped rather than wrong.
func dynamicOverflowLimits(schema string, target *ParsedURL, base overflowLimits) (overflowLimits, bool) {
	if target == nil {
		return base, false
	}

	query := func(name string) string {
		return strings.ToLower(strings.TrimSpace(target.Query[name]))
	}

	switch strings.ToLower(strings.TrimSpace(schema)) {
	case "matrix", "matrixs":
		// Webhooks are not sent as direct Matrix room events, so they keep
		// their historical v1 character allowance. Everything else reserves
		// headroom for the completed JSON event -- less when a formatted
		// (HTML/markdown) message carries both representations, and less
		// again when e2ee encryption will expand it.
		mode := query("mode")
		if mode == "" {
			// The t2bot shorthand: no password and no rooms on the URL.
			password := strings.TrimSpace(target.Password)
			if value := strings.TrimSpace(target.Query["token"]); value != "" {
				password = value
			}
			if password == "" && len(splitPath(target.Path)) == 0 {
				mode = "t2bot"
			}
		}
		if mode != "" && mode != "off" {
			base.bodyMax = 65000

			return base, true
		}

		format := normalizeNotifyFormat(target.Query["format"])
		formatted := format == "html" || format == "markdown"

		secure := strings.EqualFold(strings.TrimSpace(schema), "matrixs")
		if secure && parseBoolWithDefault(target.Query["e2ee"], true) {
			if formatted {
				base.bodyMax = 19000
			} else {
				base.bodyMax = 40000
			}

			return base, true
		}

		if formatted {
			base.bodyMax = 29000
		} else {
			base.bodyMax = 60000
		}

		return base, true

	case "twilio":
		// SMS or a voice call, which have different ceilings.
		base.bodyMax = 160
		if method := query("method"); method != "" && strings.HasPrefix("call", method) {
			base.bodyMax = 4000
		}

		return base, true

	case "webex", "wxteams":
		// A webhook takes far less than the bot API.
		base.bodyMax = 7439
		if mode := query("mode"); mode != "" && strings.HasPrefix("webhook", mode) {
			base.bodyMax = 1000
		}

		return base, true

	case "sns":
		// A topic carries a title of its own; an SMS does not.
		if mode := query("mode"); mode != "" && strings.HasPrefix("topic", mode) {
			base.bodyMax, base.titleMax = 256000, 100

			return base, true
		}
		base.bodyMax, base.titleMax = 160, 0

		return base, true

	case "notifyre":
		// Fax has room for far more than a text message.
		base.bodyMax = 160
		if mode := query("mode"); mode != "" && strings.HasPrefix("fax", mode) {
			base.bodyMax = 32768
		}

		return base, true

	case "tweet", "twitter", "x":
		// A direct message is not held to the public post limit.
		base.bodyMax = 280
		if mode := query("mode"); mode != "" && strings.HasPrefix("dm", mode) {
			base.bodyMax = 10000
		}

		return base, true

	case "dingtalk":
		// Only markdown carries a title.
		base.titleMax = 0
		if strings.HasPrefix(query("format"), "markdown") {
			base.titleMax = 250
		}

		return base, true

	case "xmpp", "xmpps":
		// ?subject=yes is what gives XMPP a title at all.
		base.titleMax = 0
		if parseBoolWithDefault(target.Query["subject"], false) {
			base.titleMax = 250
		}

		return base, true

	case "mastodon", "mastodons", "toot", "toots":
		// The ping tokens are prepended to every status, so they come out of
		// the body's budget.
		base.bodyMax = 500 - len(mastodonPingPayload(mastodonPingTokens(target)))

		return base, true
	}

	return base, false
}

// boldTitleSchemas fold a title as a bold line rather than a heading, because
// their dialect has no heading syntax.
var boldTitleSchemas = map[string]bool{"evolution": true, "evolutions": true}
