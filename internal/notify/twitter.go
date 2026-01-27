package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const twitterTweetURL = "https://api.twitter.com/1.1/statuses/update.json"
const twitterWhoamiURL = "https://api.twitter.com/1.1/account/verify_credentials.json"
const twitterLookupURL = "https://api.twitter.com/1.1/users/lookup.json"
const twitterDMURL = "https://api.twitter.com/1.1/direct_messages/events/new.json"

type TwitterTarget struct {
	consumerKey    string
	consumerSecret string
	accessKey      string
	accessSecret   string
	mode           string
	targets        []string
}

func NewTwitterTarget(target *ParsedURL) (*TwitterTarget, error) {
	consumerKey := strings.TrimSpace(target.Host)
	entries := splitPath(target.Path)
	if consumerKey == "" || len(entries) < 3 {
		return nil, fmt.Errorf("missing credentials")
	}

	consumerSecret := strings.TrimSpace(entries[0])
	accessKey := strings.TrimSpace(entries[1])
	accessSecret := strings.TrimSpace(entries[2])
	if consumerSecret == "" || accessKey == "" || accessSecret == "" {
		return nil, fmt.Errorf("missing credentials")
	}

	targets := []string{}
	if len(entries) > 3 {
		for _, entry := range entries[3:] {
			if normalized, ok := normalizeTwitterTarget(entry); ok {
				targets = append(targets, normalized)
			}
		}
	}
	if target.User != "" {
		if normalized, ok := normalizeTwitterTarget(target.User); ok {
			targets = append(targets, normalized)
		}
	}
	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		for _, entry := range parseDelimitedList(toValue) {
			if normalized, ok := normalizeTwitterTarget(entry); ok {
				targets = append(targets, normalized)
			}
		}
	}

	mode := strings.TrimSpace(target.Query["mode"])
	if mode == "" {
		if strings.HasPrefix(strings.ToLower(target.Scheme), "tweet") {
			mode = "tweet"
		} else {
			mode = "dm"
		}
	}

	return &TwitterTarget{
		consumerKey:    consumerKey,
		consumerSecret: consumerSecret,
		accessKey:      accessKey,
		accessSecret:   accessSecret,
		mode:           strings.ToLower(mode),
		targets:        targets,
	}, nil
}

func (t *TwitterTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if t.mode != "tweet" {
		return RequestSpec{}, fmt.Errorf("unsupported mode")
	}
	return t.tweetRequest(body, title)
}

func (t *TwitterTarget) Send(body, title string, notifyType NotifyType) error {
	if t.mode == "tweet" {
		spec, err := t.tweetRequest(body, title)
		if err != nil {
			return err
		}
		return SendRequest(spec)
	}
	if t.mode == "dm" {
		return t.sendDM(body, title)
	}
	return fmt.Errorf("unsupported mode")
}

func (t *TwitterTarget) tweetRequest(body, title string) (RequestSpec, error) {
	message := mergeTitleBody(title, body)
	payload := url.Values{}
	payload.Set("status", message)

	auth, err := buildOAuth1Header(
		"POST",
		twitterTweetURL,
		payload,
		t.consumerKey,
		t.consumerSecret,
		t.accessKey,
		t.accessSecret,
	)
	if err != nil {
		return RequestSpec{}, err
	}

	return RequestSpec{
		Method: "POST",
		URL:    twitterTweetURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Accept":        "*/*",
			"Content-Type":  "application/x-www-form-urlencoded",
			"Authorization": auth,
		},
		Body: payload.Encode(),
	}, nil
}

type twitterWhoamiResponse struct {
	ScreenName string      `json:"screen_name"`
	ID         json.Number `json:"id"`
	IDStr      string      `json:"id_str"`
}

type twitterDMRequest struct {
	Event twitterDMEvent `json:"event"`
}

type twitterDMEvent struct {
	Type          string               `json:"type"`
	MessageCreate twitterDMMessageBody `json:"message_create"`
}

type twitterDMMessageBody struct {
	Target      twitterDMTarget `json:"target"`
	MessageData twitterDMText   `json:"message_data"`
}

type twitterDMTarget struct {
	RecipientID string `json:"recipient_id"`
}

type twitterDMText struct {
	Text string `json:"text"`
}

var twitterUserPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func normalizeTwitterTarget(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	trimmed = strings.TrimPrefix(trimmed, "@")
	if trimmed == "" || !twitterUserPattern.MatchString(trimmed) {
		return "", false
	}
	return trimmed, true
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

type twitterLookupEntry struct {
	ScreenName string      `json:"screen_name"`
	ID         json.Number `json:"id"`
	IDStr      string      `json:"id_str"`
}

type twitterRecipient struct {
	ScreenName string
	ID         string
}

func (t *TwitterTarget) sendDM(body, title string) error {
	message := mergeTitleBody(title, body)
	recipients := t.resolveRecipients()
	if len(recipients) == 0 {
		return nil
	}

	for _, recipient := range recipients {
		payload := twitterDMRequest{
			Event: twitterDMEvent{
				Type: "message_create",
				MessageCreate: twitterDMMessageBody{
					Target:      twitterDMTarget{RecipientID: recipient.ID},
					MessageData: twitterDMText{Text: message},
				},
			},
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		auth, err := buildOAuth1Header(
			"POST",
			twitterDMURL,
			nil,
			t.consumerKey,
			t.consumerSecret,
			t.accessKey,
			t.accessSecret,
		)
		if err != nil {
			return err
		}

		spec := RequestSpec{
			Method: "POST",
			URL:    twitterDMURL,
			Headers: map[string]string{
				"User-Agent":    "Apprise",
				"Accept":        "*/*",
				"Authorization": auth,
				"Content-Type":  "application/json",
			},
			Body: string(data),
		}

		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func (t *TwitterTarget) resolveRecipients() []twitterRecipient {
	if len(t.targets) == 0 {
		return t.resolveWhoami()
	}
	return t.lookupUsers(t.targets)
}

func (t *TwitterTarget) resolveWhoami() []twitterRecipient {
	auth, err := buildOAuth1Header(
		"GET",
		twitterWhoamiURL,
		nil,
		t.consumerKey,
		t.consumerSecret,
		t.accessKey,
		t.accessSecret,
	)
	if err != nil {
		return nil
	}

	spec := RequestSpec{
		Method: "GET",
		URL:    twitterWhoamiURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Accept":        "*/*",
			"Authorization": auth,
		},
	}

	var response twitterWhoamiResponse
	if err := doJSONRequest(spec, &response); err != nil {
		return nil
	}
	id := response.IDStr
	if id == "" {
		id = response.ID.String()
	}
	if id == "" || response.ScreenName == "" {
		return nil
	}
	return []twitterRecipient{{ScreenName: response.ScreenName, ID: id}}
}

func (t *TwitterTarget) lookupUsers(targets []string) []twitterRecipient {
	names := uniqueStrings(targets)
	if len(names) == 0 {
		return nil
	}

	results := map[string]string{}
	for i := 0; i < len(names); i += 100 {
		end := i + 100
		if end > len(names) {
			end = len(names)
		}
		payload := url.Values{}
		for _, name := range names[i:end] {
			payload.Add("screen_name", name)
		}

		auth, err := buildOAuth1Header(
			"POST",
			twitterLookupURL,
			payload,
			t.consumerKey,
			t.consumerSecret,
			t.accessKey,
			t.accessSecret,
		)
		if err != nil {
			continue
		}

		spec := RequestSpec{
			Method: "POST",
			URL:    twitterLookupURL,
			Headers: map[string]string{
				"User-Agent":    "Apprise",
				"Accept":        "*/*",
				"Content-Type":  "application/x-www-form-urlencoded",
				"Authorization": auth,
			},
			Body: payload.Encode(),
		}

		var response []twitterLookupEntry
		if err := doJSONRequest(spec, &response); err != nil {
			continue
		}
		for _, entry := range response {
			if entry.ScreenName == "" {
				continue
			}
			id := entry.IDStr
			if id == "" {
				id = entry.ID.String()
			}
			if id == "" {
				continue
			}
			results[entry.ScreenName] = id
		}
	}

	recipients := make([]twitterRecipient, 0, len(results))
	for _, name := range names {
		if id, ok := results[name]; ok {
			recipients = append(recipients, twitterRecipient{ScreenName: name, ID: id})
		}
	}
	return recipients
}

func init() {
	RegisterSchemaEntryOrdered(74, SchemaEntry{
		"attachment_support": true,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"batch": map[string]any{
					"default":  true,
					"map_to":   "batch",
					"name":     "Batch Mode",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"cache": map[string]any{
					"default":  true,
					"map_to":   "cache",
					"name":     "Cache Results",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
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
				"mode": map[string]any{
					"default":  "dm",
					"map_to":   "mode",
					"name":     "Message Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"dm", "tweet"},
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
			"templates": []string{"{schema}://{ckey}/{csecret}/{akey}/{asecret}", "{schema}://{ckey}/{csecret}/{akey}/{asecret}/{targets}"},
			"tokens": map[string]any{
				"akey": map[string]any{
					"map_to":   "akey",
					"name":     "Access Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"asecret": map[string]any{
					"map_to":   "asecret",
					"name":     "Access Secret",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"ckey": map[string]any{
					"map_to":   "ckey",
					"name":     "Consumer Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"csecret": map[string]any{
					"map_to":   "csecret",
					"name":     "Consumer Secret",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"tweet", "twitter", "x"},
				},
				"target_user": map[string]any{
					"map_to":   "targets",
					"name":     "Target User",
					"prefix":   "@",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_user"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
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
		"secure_protocols": []string{"x", "twitter", "tweet"},
		"service_name":     "Twitter",
		"service_url":      "https://twitter.com/",
		"setup_url":        "https://appriseit.com/services/twitter/",
	})
}
