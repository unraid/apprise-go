package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const wxpusherURL = "https://wxpusher.zjiecode.com/api/send/message"

var wxpusherTokenRe = regexp.MustCompile(`(?i)^AT_[^\s]+$`)
var wxpusherTopicRe = regexp.MustCompile(`^[1-9][0-9]{0,20}$`)
var wxpusherUserRe = regexp.MustCompile(`(?i)^UID_[^\s]+$`)

const (
	wxPusherContentText = 1
	wxPusherContentHTML = 2
	wxPusherContentMD   = 3
)

type WxPusherTarget struct {
	token       string
	contentType int
	topics      []int
	users       []string
}

func NewWxPusherTarget(target *ParsedURL) (*WxPusherTarget, error) {
	token := strings.TrimSpace(target.Host)
	hostTarget := ""
	if rawToken := strings.TrimSpace(target.Query["token"]); rawToken != "" {
		token = rawToken
		hostTarget = strings.TrimSpace(target.Host)
	}
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}
	if !wxpusherTokenRe.MatchString(token) {
		return nil, fmt.Errorf("invalid token")
	}

	contentType := wxPusherContentText
	if rawFormat := strings.ToLower(strings.TrimSpace(target.Query["format"])); rawFormat != "" {
		switch rawFormat {
		case "html":
			contentType = wxPusherContentHTML
		case "markdown", "md":
			contentType = wxPusherContentMD
		default:
			contentType = wxPusherContentText
		}
	}

	entries := []string{}
	if hostTarget != "" {
		entries = append(entries, hostTarget)
	}
	entries = append(entries, splitPath(target.Path)...)
	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		entries = append(entries, parseDelimitedList(toValue)...)
	}

	sortedEntries := normalizeWxPusherEntries(entries)

	users := []string{}
	topics := []int{}
	for _, entry := range sortedEntries {
		if wxpusherUserRe.MatchString(entry) {
			users = append(users, entry)
			continue
		}
		if wxpusherTopicRe.MatchString(entry) {
			value, err := strconv.Atoi(entry)
			if err != nil {
				continue
			}
			topics = append(topics, value)
		}
	}

	if len(users) == 0 && len(topics) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	return &WxPusherTarget{
		token:       token,
		contentType: contentType,
		topics:      topics,
		users:       users,
	}, nil
}

func (w *WxPusherTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	spec, err := w.buildRequest(body, title)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = notifyType

	return spec, nil
}

func (w *WxPusherTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := w.buildRequest(body, title)
	if err != nil {
		return err
	}

	_ = notifyType

	return SendRequest(spec)
}

func (w *WxPusherTarget) buildRequest(body, title string) (RequestSpec, error) {
	payload := map[string]any{
		"appToken":    w.token,
		"content":     body,
		"summary":     title,
		"contentType": w.contentType,
		"topicIds":    w.topics,
		"uids":        w.users,
		"url":         nil,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    wxpusherURL,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		Body: string(data),
	}, nil
}

func normalizeWxPusherEntries(entries []string) []string {
	if len(entries) == 0 {
		return nil
	}

	unique := map[string]struct{}{}
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		unique[entry] = struct{}{}
	}

	if len(unique) == 0 {
		return nil
	}

	sorted := make([]string, 0, len(unique))
	for entry := range unique {
		sorted = append(sorted, entry)
	}
	sort.Strings(sorted)
	return sorted
}

func init() {
	RegisterSchemaEntryOrdered(112, SchemaEntry{
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
			"templates": []string{"{schema}://{token}/{targets}"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "wxpusher",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"wxpusher"},
				},
				"target_topic": map[string]any{
					"map_to":   "targets",
					"name":     "Target Topic",
					"private":  false,
					"required": false,
					"type":     "int",
				},
				"target_user": map[string]any{
					"map_to":   "targets",
					"name":     "Target User ID",
					"private":  false,
					"regex":    []string{"^UID_[^\\s]+$", "i"},
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_topic", "target_user"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
				"token": map[string]any{
					"map_to":   "token",
					"name":     "App Token",
					"private":  true,
					"regex":    []string{"^AT_[^\\s]+$", "i"},
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
		"secure_protocols": []string{"wxpusher"},
		"service_name":     "WxPusher",
		"service_url":      "https://wxpusher.zjiecode.com/",
		"setup_url":        "https://appriseit.com/services/wxpusher/",
	})
}
