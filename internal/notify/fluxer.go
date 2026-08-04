package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Fluxer posts to the developer API by default; a private server is addressed
// by putting its host in the URL.
const fluxerCloudHost = "https://api.fluxer.app"

var (
	fluxerWebhookIDPattern    = regexp.MustCompile(`^[0-9]{10,}$`)
	fluxerWebhookTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_\-]{16,}$`)
	// A host under fluxer.app is the cloud API however the mode reads.
	fluxerCloudHostPattern = regexp.MustCompile(`(?i)fluxer\.app`)
	fluxerHostnamePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.\-]*$`)
)

type FluxerTarget struct {
	webhookID    string
	webhookToken string
	host         string
	port         int
	secure       bool
	mode         string
	username     string
	tts          bool
	avatar       bool
	avatarURL    string
	href         string
	threadID     string
	threadName   string
	footer       bool
	footerLogo   bool
	includeImage bool
	flags        int
	hasFlags     bool
	format       string
}

func NewFluxerTarget(target *ParsedURL) (*FluxerTarget, error) {
	// The webhook id and token are the last two tokens of host plus path, so
	// a private server's host and port simply push them further along.
	tokens := append([]string{strings.TrimSpace(target.Host)}, splitPath(target.Path)...)
	if len(tokens) < 2 {
		return nil, fmt.Errorf("missing webhook credentials")
	}

	webhookToken := tokens[len(tokens)-1]
	webhookID := tokens[len(tokens)-2]
	if !fluxerWebhookIDPattern.MatchString(webhookID) {
		return nil, fmt.Errorf("invalid webhook id: %s", webhookID)
	}
	if !fluxerWebhookTokenPattern.MatchString(webhookToken) {
		return nil, fmt.Errorf("invalid webhook token")
	}

	host := strings.TrimSpace(target.Host)

	mode := strings.ToLower(strings.TrimSpace(target.Query["mode"]))
	if mode == "" {
		// With no mode given, a usable hostname plus enough tokens to sit
		// behind it means a private server.
		if fluxerHostnamePattern.MatchString(host) && len(tokens) > 2 {
			mode = "private"
		} else {
			mode = "cloud"
		}
	}
	if mode != "cloud" && mode != "private" {
		return nil, fmt.Errorf("invalid mode: %s", mode)
	}

	if mode == "private" {
		if host == "" {
			return nil, fmt.Errorf("missing host")
		}
		// Pointing private mode at fluxer.app is really cloud mode.
		if fluxerCloudHostPattern.MatchString(host) {
			mode = "cloud"
		}
	}

	format := normalizeDiscordFormat(target.Query["format"])

	fluxer := &FluxerTarget{
		webhookID:    webhookID,
		webhookToken: webhookToken,
		host:         host,
		port:         target.Port,
		secure:       strings.EqualFold(target.Scheme, "fluxers"),
		mode:         mode,
		username:     strings.TrimSpace(target.User),
		tts:          parseBoolWithDefault(target.Query["tts"], false),
		avatar:       parseBoolWithDefault(target.Query["avatar"], true),
		avatarURL:    strings.TrimSpace(target.Query["avatar_url"]),
		threadID:     strings.TrimSpace(target.Query["thread"]),
		threadName:   strings.TrimSpace(target.Query["thread_name"]),
		footer:       parseBoolWithDefault(target.Query["footer"], false),
		footerLogo:   parseBoolWithDefault(target.Query["footer_logo"], true),
		includeImage: parseBoolWithDefault(target.Query["image"], false),
	}

	// ?url= is an alias for ?href=.
	fluxer.href = strings.TrimSpace(target.Query["href"])
	if fluxer.href == "" {
		fluxer.href = strings.TrimSpace(target.Query["url"])
	}

	// Naming the bot through a query parameter overrides the user field.
	if botname := strings.TrimSpace(target.Query["botname"]); botname != "" {
		fluxer.username = botname
	} else if name := strings.TrimSpace(target.Query["name"]); name != "" {
		fluxer.username = name
	}

	if raw := strings.TrimSpace(target.Query["flags"]); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return nil, fmt.Errorf("invalid flags: %s", raw)
		}
		fluxer.flags, fluxer.hasFlags = value, true
	}

	// Anything that only an embed can carry forces markdown.
	if format == "" && (fluxer.href != "" || fluxer.threadID != "") {
		format = "markdown"
	}
	if format == "" {
		format = "text"
	}
	fluxer.format = format

	return fluxer, nil
}

func (f *FluxerTarget) buildPayload(body, title string, notifyType NotifyType) (map[string]any, error) {
	payload := map[string]any{
		"tts": f.tts,
		// Text-to-speech means there is no reason to wait for the message.
		"wait": !f.tts,
	}

	// ?flags= is deliberately absent from the payload. Upstream validates it
	// and puts it back in url(), but never sends it — unlike Discord, which
	// does. Looks like an oversight there, but matching it is the job.

	imageURL := defaultImageURL(notifyType)
	if f.avatar {
		if f.avatarURL != "" {
			payload["avatar_url"] = f.avatarURL
		} else if imageURL != "" {
			payload["avatar_url"] = imageURL
		}
	}

	if f.username != "" {
		payload["username"] = f.username
	}
	if f.threadName != "" {
		payload["thread_name"] = f.threadName
	}

	if body != "" {
		if f.format == "markdown" {
			embed := map[string]any{
				"author": map[string]any{
					"name": "Apprise",
					"url":  appriseAppURL,
				},
				"description": body,
				"color":       appriseColorInt(notifyType),
			}
			// Fluxer validates strictly, so an empty title is left out.
			if title != "" {
				embed["title"] = title
			}
			if f.href != "" {
				embed["url"] = f.href
			}
			if f.footer {
				footer := map[string]any{"text": appriseAppDesc}
				if logoURL := appriseLogoURL(notifyType); f.footerLogo && logoURL != "" {
					footer["icon_url"] = logoURL
				}
				embed["footer"] = footer
			}
			if f.includeImage && imageURL != "" {
				embed["thumbnail"] = map[string]any{
					"url":    imageURL,
					"height": 256,
					"width":  256,
				}
			}

			payload["embeds"] = []any{embed}
		} else if title == "" {
			payload["content"] = body
		} else {
			payload["content"] = title + "\r\n" + body
		}
	}

	return payload, nil
}

func (f *FluxerTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload, err := f.buildPayload(body, title, notifyType)
	if err != nil {
		return RequestSpec{}, err
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return f.specFor(string(data), "application/json; charset=utf-8")
}

func (f *FluxerTarget) specFor(requestBody, contentType string) (RequestSpec, error) {
	targetURL := fmt.Sprintf("%s/webhooks/%s/%s", f.prefix(), f.webhookID, f.webhookToken)
	if f.threadID != "" {
		parsed, err := url.Parse(targetURL)
		if err != nil {
			return RequestSpec{}, err
		}
		query := parsed.Query()
		query.Set("thread_id", f.threadID)
		parsed.RawQuery = query.Encode()
		targetURL = parsed.String()
	}

	return RequestSpec{
		Method: "POST",
		URL:    targetURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": contentType,
		},
		Body: requestBody,
	}, nil
}

func (f *FluxerTarget) Send(body, title string, notifyType NotifyType) error {
	return f.SendWithAttachments(body, title, notifyType, nil)
}

// SendWithAttachments posts the message, then one further request per file.
// Fluxer does not carry files alongside the text the way Discord does: each
// attachment is its own post, with the embed dropped and text-to-speech off
// so the filename is not read aloud.
func (f *FluxerTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	spec, err := f.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}
	if err := SendRequest(spec); err != nil {
		return err
	}

	if len(attachments) == 0 {
		return nil
	}

	payload, err := f.buildPayload(body, title, notifyType)
	if err != nil {
		return err
	}
	payload["tts"] = false
	payload["wait"] = false
	delete(payload, "embeds")
	delete(payload, "allow_mentions")

	for _, attachment := range attachments {
		requestBody, contentType, err := discordStyleAttachmentBody(payload, []Attachment{attachment})
		if err != nil {
			return err
		}
		spec, err := f.specFor(requestBody, contentType)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

// prefix is the scheme and authority the webhook path hangs off: the cloud API
// unless a private server was configured.
func (f *FluxerTarget) prefix() string {
	if f.mode == "cloud" {
		return fluxerCloudHost
	}

	scheme := "http"
	if f.secure {
		scheme = "https"
	}

	prefix := fmt.Sprintf("%s://%s", scheme, f.host)
	if f.port > 0 {
		prefix += fmt.Sprintf(":%d", f.port)
	}

	return prefix
}
func init() {
	RegisterSchemaEntryOrdered(164, SchemaEntry{
		"attachment_support": true,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"avatar": map[string]any{
					"default":  true,
					"map_to":   "avatar",
					"name":     "Avatar Image",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"avatar_url": map[string]any{
					"map_to":   "avatar_url",
					"name":     "Avatar URL",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"cto": map[string]any{
					"default":  4.0,
					"map_to":   "cto",
					"name":     "Socket Connect Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"emojis": map[string]any{
					"default":  false,
					"map_to":   "emojis",
					"name":     "Interpret Emojis",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"fields": map[string]any{
					"default":  true,
					"map_to":   "fields",
					"name":     "Use Fields",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"flags": map[string]any{
					"map_to":   "flags",
					"min":      0,
					"name":     "Discord Flags",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"footer": map[string]any{
					"default":  false,
					"map_to":   "footer",
					"name":     "Display Footer",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"footer_logo": map[string]any{
					"default":  true,
					"map_to":   "footer_logo",
					"name":     "Footer Logo",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"format": map[string]any{
					"default":  "text",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"href": map[string]any{
					"map_to":   "href",
					"name":     "URL",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"image": map[string]any{
					"default":  false,
					"map_to":   "include_image",
					"name":     "Include Image",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"mode": map[string]any{
					"default":  "cloud",
					"map_to":   "mode",
					"name":     "Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"cloud", "private"},
				},
				"name": map[string]any{
					"alias_of": "botname",
				},
				"optional": map[string]any{
					"default":  false,
					"map_to":   "optional",
					"name":     "Optional Service",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"overflow": map[string]any{
					"default":  "upstream",
					"map_to":   "overflow",
					"name":     "Overflow Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"split", "truncate", "upstream"},
				},
				"ping": map[string]any{
					"delim":    []string{",", " "},
					"group":    []any{},
					"map_to":   "ping",
					"name":     "Ping Users/Roles",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"redirect": map[string]any{
					"default":  true,
					"map_to":   "redirect",
					"name":     "Follow Redirects",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"retry": map[string]any{
					"default":  0,
					"map_to":   "retry",
					"max":      10,
					"min":      0,
					"name":     "Service Retry",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"rto": map[string]any{
					"default":  4.0,
					"map_to":   "rto",
					"name":     "Socket Read Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"store": map[string]any{
					"default":  true,
					"map_to":   "store",
					"name":     "Persistent Storage",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"thread": map[string]any{
					"map_to":   "thread",
					"name":     "Thread ID",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"thread_name": map[string]any{
					"map_to":   "thread_name",
					"name":     "Thread Name",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"tts": map[string]any{
					"default":  false,
					"map_to":   "tts",
					"name":     "Text To Speech",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"tz": map[string]any{
					"default":  nil,
					"map_to":   "tz",
					"name":     "Timezone",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"url": map[string]any{
					"alias_of": "href",
				},
				"verify": map[string]any{
					"default":  true,
					"map_to":   "verify",
					"name":     "Verify SSL",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"wait": map[string]any{
					"default":  0.0,
					"map_to":   "wait",
					"max":      20.0,
					"min":      0.0,
					"name":     "Inter-Retry Wait",
					"private":  false,
					"required": false,
					"type":     "float",
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{webhook_id}/{webhook_token}", "{schema}://{host}/{webhook_id}/{webhook_token}", "{schema}://{host}:{port}/{webhook_id}/{webhook_token}", "{schema}://{botname}@{webhook_id}/{webhook_token}", "{schema}://{botname}@{host}:{port}/{webhook_id}/{webhook_token}"},
			"tokens": map[string]any{
				"botname": map[string]any{
					"map_to":   "user",
					"name":     "Bot Name",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"host": map[string]any{
					"map_to":   "host",
					"name":     "Hostname",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"port": map[string]any{
					"map_to":   "port",
					"max":      65535,
					"min":      1,
					"name":     "Port",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"fluxer", "fluxers"},
				},
				"webhook_id": map[string]any{
					"map_to":   "webhook_id",
					"name":     "Webhook ID",
					"private":  true,
					"regex":    []string{"^[0-9]{10,}$", "i"},
					"required": true,
					"type":     "string",
				},
				"webhook_token": map[string]any{
					"map_to":   "webhook_token",
					"name":     "Webhook Token",
					"private":  true,
					"regex":    []string{"^[A-Za-z0-9_\\-]{16,}$", "i"},
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"fluxer"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"fluxers"},
		"service_name":     "Fluxer",
		"service_url":      "https://fluxer.app/",
		"setup_url":        "https://appriseit.com/services/fluxer/",
	})
}
