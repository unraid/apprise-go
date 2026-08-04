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
	limits, known := overflowLimitsFor(schema)

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
	if mode == OverflowUpstream || !known || limits.bodyMax < 0 {
		return []MessagePart{{Title: title, Body: body}}
	}

	// A service with no title of its own has the title folded into the body
	// before anything is measured, and the title is cleared. Measuring the
	// body on its own and letting the provider fold afterwards produces
	// chunks that are longer than the limit by the width of the title.
	if limits.titleMax <= 0 && title != "" {
		body = foldTitleIntoBody(title, body, format)
		title = ""
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
func foldTitleIntoBody(title, body, format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "html":
		return "<b>" + title + "</b><br />\r\n" + body
	case "markdown":
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
	"46elks":          {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"apprise":         {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"apprises":        {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"aprs":            {bodyMax: 67, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"atalk":           {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"azure":           {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"bark":            {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"barks":           {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"blink1":          {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"bluesky":         {bodyMax: 280, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"brevo":           {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"bsky":            {bodyMax: 280, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"bulksms":         {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"bulkvs":          {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"burstsms":        {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"chanify":         {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"chime":           {bodyMax: 4096, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"clickatell":      {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"clicksend":       {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"d7sms":           {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"dapnet":          {bodyMax: 80, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"dbus":            {bodyMax: 32768, titleMax: 250, lineMax: 10, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"dingtalk":        {bodyMax: 32768, titleMax: -1, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"discord":         {bodyMax: 2000, titleMax: 250, lineMax: 0, amalgamateTitle: true, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"dot":             {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"eight00com":      {bodyMax: 600, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"elks":            {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"emby":            {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"embys":           {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"enigma2":         {bodyMax: 1000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"enigma2s":        {bodyMax: 1000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"evolution":       {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"evolutions":      {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"exotel":          {bodyMax: 2000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"fcm":             {bodyMax: 1024, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"feishu":          {bodyMax: 19985, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"flock":           {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"flowtriq":        {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"flowtriqs":       {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"fluxer":          {bodyMax: 2000, titleMax: 250, lineMax: 0, amalgamateTitle: true, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"fluxers":         {bodyMax: 2000, titleMax: 250, lineMax: 0, amalgamateTitle: true, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"form":            {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"forms":           {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"freemobile":      {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"gchat":           {bodyMax: 4000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"gio":             {bodyMax: 32768, titleMax: 250, lineMax: 10, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"glib":            {bodyMax: 32768, titleMax: 250, lineMax: 10, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"gnome":           {bodyMax: 32768, titleMax: 0, lineMax: 10, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"gotify":          {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"gotifys":         {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"groupme":         {bodyMax: 1000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"growl":           {bodyMax: 32768, titleMax: 250, lineMax: 2, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"guilded":         {bodyMax: 2000, titleMax: 250, lineMax: 0, amalgamateTitle: true, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"hassio":          {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"hassios":         {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"httpsms":         {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"humhub":          {bodyMax: 4000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"humhubs":         {bodyMax: 4000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"ifttt":           {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"irc":             {bodyMax: 380, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"ircs":            {bodyMax: 380, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"jellyfin":        {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"jellyfins":       {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"jira":            {bodyMax: 15000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"join":            {bodyMax: 1000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"json":            {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"jsons":           {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"kavenegar":       {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"kde":             {bodyMax: 32768, titleMax: 250, lineMax: 10, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"kodi":            {bodyMax: 32768, titleMax: 250, lineMax: 2, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"kodis":           {bodyMax: 32768, titleMax: 250, lineMax: 2, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"kook":            {bodyMax: 5000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"kumulos":         {bodyMax: 240, titleMax: 64, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"lametric":        {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"lametrics":       {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"lark":            {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"line":            {bodyMax: 5000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"macosx":          {bodyMax: 32768, titleMax: 250, lineMax: 10, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"mailersend":      {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"mailgun":         {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"mailto":          {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"mailtos":         {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"mastodon":        {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"mastodons":       {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"matrix":          {bodyMax: 65000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"matrixs":         {bodyMax: 65000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"misskey":         {bodyMax: 512, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"misskeys":        {bodyMax: 512, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"mmost":           {bodyMax: 4000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"mmosts":          {bodyMax: 4000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"mqtt":            {bodyMax: 268435455, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"mqtts":           {bodyMax: 268435455, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"msg91":           {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"msgbird":         {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"napi":            {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"ncloud":          {bodyMax: 4000, titleMax: 255, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"nclouds":         {bodyMax: 4000, titleMax: 255, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"nctalk":          {bodyMax: 4000, titleMax: 255, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"nctalks":         {bodyMax: 4000, titleMax: 255, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"nexmo":           {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"notica":          {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"noticas":         {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"notifiarr":       {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"notificationapi": {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12, format: "text"},
	"notifico":        {bodyMax: 512, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"notificos":       {bodyMax: 512, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"notifyre":        {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"ntfy":            {bodyMax: 7800, titleMax: 200, lineMax: 0, amalgamateTitle: true, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"ntfys":           {bodyMax: 7800, titleMax: 200, lineMax: 0, amalgamateTitle: true, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"o365":            {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"octopush":        {bodyMax: 1224, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"onesignal":       {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"opsgenie":        {bodyMax: 15000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"pagerduty":       {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"pagertree":       {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"parsep":          {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"parseps":         {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"pbul":            {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"pjet":            {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"pjets":           {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"plivo":           {bodyMax: 140, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"postmark":        {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"pover":           {bodyMax: 1024, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"prowl":           {bodyMax: 10000, titleMax: 1024, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"psafer":          {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"psafers":         {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"push":            {bodyMax: 1000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"pushdeer":        {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"pushdeers":       {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"pushed":          {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"pushme":          {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"pushplus":        {bodyMax: 20000, titleMax: 200, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"pushward":        {bodyMax: 3000, titleMax: 256, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"pushy":           {bodyMax: 4096, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"qq":              {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"qt":              {bodyMax: 32768, titleMax: 250, lineMax: 10, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"reddit":          {bodyMax: 6000, titleMax: 300, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"resend":          {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"revolt":          {bodyMax: 2000, titleMax: 100, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"ringc":           {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"rocket":          {bodyMax: 1000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"rockets":         {bodyMax: 1000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"rsyslog":         {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"ryver":           {bodyMax: 1000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"schan":           {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"sendgrid":        {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"sendpulse":       {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"serwersms":       {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"ses":             {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"session":         {bodyMax: 2000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"sessions":        {bodyMax: 2000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"seven":           {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"sfr":             {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"signal":          {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"signals":         {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"signl4":          {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"sinch":           {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"slack":           {bodyMax: 35000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"smpp":            {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"smpps":           {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"smsc":            {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"smseagle":        {bodyMax: 1200, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"smseagles":       {bodyMax: 1200, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"smsmanager":      {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"smsmgr":          {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"smtp2go":         {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"sogs":            {bodyMax: 2000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"sparkpost":       {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"spike":           {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"splunk":          {bodyMax: 400, titleMax: 60, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"spugpush":        {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"spush":           {bodyMax: 10000, titleMax: 1024, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"stackfield":      {bodyMax: 4000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"strmlabs":        {bodyMax: 255, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"synology":        {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"synologys":       {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"syslog":          {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"tgram":           {bodyMax: 4096, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"threema":         {bodyMax: 3500, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"toot":            {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"toots":           {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"tweet":           {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"twilio":          {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"twist":           {bodyMax: 1000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"twitter":         {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"vapid":           {bodyMax: 4000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"viber":           {bodyMax: 30000, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"victorops":       {bodyMax: 400, titleMax: 60, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"voipms":          {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"vonage":          {bodyMax: 160, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"webex":           {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"wechat":          {bodyMax: 2048, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"wecom":           {bodyMax: 20000, titleMax: 200, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"wecombot":        {bodyMax: 32768, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"whatsapp":        {bodyMax: 1024, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"windows":         {bodyMax: 32768, titleMax: 250, lineMax: 2, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"workflow":        {bodyMax: 1000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"workflows":       {bodyMax: 1000, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"wxpusher":        {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"wxteams":         {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"x":               {bodyMax: -1, titleMax: 0, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"xbmc":            {bodyMax: 32768, titleMax: 250, lineMax: 2, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"xbmcs":           {bodyMax: 32768, titleMax: 250, lineMax: 2, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"xml":             {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"xmls":            {bodyMax: 32768, titleMax: 250, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"xmpp":            {bodyMax: 32768, titleMax: -1, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"xmpps":           {bodyMax: 32768, titleMax: -1, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"zoom":            {bodyMax: 4000, titleMax: 250, lineMax: 0, amalgamateTitle: true, buffer: 0, countThreshold: 130, maxCountWidth: 12},
	"zulip":           {bodyMax: 10000, titleMax: 60, lineMax: 0, amalgamateTitle: false, buffer: 0, countThreshold: 130, maxCountWidth: 12},
}

// OverflowLimits is the exported view of a service's limits, so the parity
// suite can compare the generated table against upstream.
type OverflowLimits struct {
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
