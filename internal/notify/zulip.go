package notify

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const zulipDefaultHostname = "zulipchat.com"

var zulipTargetDelims = regexp.MustCompile(`[ \t\r\n,#\\/]+`)

type ZulipTarget struct {
	organization string
	hostname     string
	botname      string
	token        string
	targets      []string
}

func NewZulipTarget(target *ParsedURL) (*ZulipTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing organization")
	}

	organization, hostname := splitZulipHost(host)

	botname := strings.TrimSpace(target.User)
	if botname == "" {
		return nil, fmt.Errorf("missing botname")
	}
	if strings.HasSuffix(strings.ToLower(botname), "-bot") {
		botname = botname[:len(botname)-4]
	}

	segments := splitPath(target.Path)
	if len(segments) == 0 {
		return nil, fmt.Errorf("missing token")
	}
	token := segments[0]

	targets := splitZulipTargets(strings.Join(segments[1:], "/"))
	if targetValue, ok := target.Query["to"]; ok && strings.TrimSpace(targetValue) != "" {
		targets = append(targets, splitZulipTargets(targetValue)...)
	}
	if len(targets) == 0 {
		targets = []string{"general"}
	}

	return &ZulipTarget{
		organization: organization,
		hostname:     hostname,
		botname:      botname,
		token:        token,
		targets:      targets,
	}, nil
}

func (z *ZulipTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := z.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (z *ZulipTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(z.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	target := z.targets[0]

	payload := url.Values{}
	payload.Set("subject", title)
	payload.Set("content", body)
	if strings.Contains(target, "@") {
		payload.Set("type", "private")
		payload.Set("to", target)
	} else {
		payload.Set("type", "stream")
		payload.Set("to", target)
	}

	authUser := fmt.Sprintf("%s-bot@%s.%s", z.botname, z.organization, z.hostname)
	headers := map[string]string{
		"User-Agent":   "Apprise",
		"Accept":       "*/*",
		"Content-Type": "application/x-www-form-urlencoded; charset=utf-8",
		"Authorization": basicAuthHeader(
			authUser,
			z.token,
		),
	}

	urlHost := strings.ToLower(fmt.Sprintf("%s.%s", z.organization, z.hostname))
	url := fmt.Sprintf("https://%s/api/v1/messages", urlHost)

	return RequestSpec{
		Method:  "POST",
		URL:     url,
		Headers: headers,
		Body:    payload.Encode(),
	}, nil
}

func splitZulipHost(host string) (string, string) {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" {
		return "", zulipDefaultHostname
	}
	if idx := strings.Index(trimmed, "."); idx != -1 {
		return trimmed[:idx], trimmed[idx+1:]
	}
	return trimmed, zulipDefaultHostname
}

func splitZulipTargets(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	parts := zulipTargetDelims.Split(trimmed, -1)
	targets := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		decoded, err := url.PathUnescape(part)
		if err == nil {
			part = decoded
		}
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		targets = append(targets, part)
	}

	if len(targets) == 0 {
		return nil
	}
	return targets
}

func init() {
	RegisterSchemaEntryOrdered(6, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"cto": map[string]any{
					"default":  4,
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
				"format": map[string]any{
					"default":  "text",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
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
				"rto": map[string]any{
					"default":  4,
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
				"to": map[string]any{
					"alias_of": "targets",
					"delim":    []string{",", " "},
				},
				"token": map[string]any{
					"alias_of": "token",
				},
				"tz": map[string]any{
					"default":  nil,
					"map_to":   "tz",
					"name":     "Timezone",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"verify": map[string]any{
					"default":  true,
					"map_to":   "verify",
					"name":     "Verify SSL",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{botname}@{organization}/{token}", "{schema}://{botname}@{organization}/{token}/{targets}"},
			"tokens": map[string]any{
				"botname": map[string]any{
					"map_to":   "botname",
					"name":     "Bot Name",
					"private":  false,
					"regex":    []string{"^[A-Z0-9_-]{1,32}$", "i"},
					"required": true,
					"type":     "string",
				},
				"organization": map[string]any{
					"map_to":   "organization",
					"name":     "Organization",
					"private":  false,
					"regex":    []string{"^[A-Z0-9_-]{1,32})$", "i"},
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "zulip",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"zulip"},
				},
				"target_stream": map[string]any{
					"map_to":   "targets",
					"name":     "Target Stream",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_user": map[string]any{
					"map_to":   "targets",
					"name":     "Target User",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_stream", "target_user"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "Token",
					"private":  true,
					"regex":    []string{"^[A-Z0-9]{32}$", "i"},
					"required": true,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": nil,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"zulip"},
		"service_name":     "Zulip",
		"service_url":      "https://zulipchat.com/",
		"setup_url":        "https://appriseit.com/services/zulip/",
	})
}
