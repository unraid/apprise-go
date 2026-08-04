package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// X API v2 endpoints; v1.1 was retired upstream in 1.12.0.
const twitterTweetURL = "https://api.twitter.com/2/tweets"

// Media is uploaded first and the tweet references the ids it gets back.
const twitterMediaURL = "https://api.x.com/2/media/upload"

// Twitter batches up to four still images into one tweet; a gif has to stand
// alone, which breaks a run of images into separate tweets around it.
const twitterImageBatchSize = 4
const twitterWhoamiURL = "https://api.twitter.com/2/users/me"
const twitterLookupURL = "https://api.twitter.com/2/users/by"
const twitterDMURLTemplate = "https://api.twitter.com/2/dm_conversations/with/%s/messages"

type TwitterTarget struct {
	consumerKey    string
	consumerSecret string
	accessKey      string
	accessSecret   string
	mode           string
	targets        []string
	batch          bool
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
		batch:          parseBoolWithDefault(target.Query["batch"], true),
	}, nil
}

func (t *TwitterTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if t.mode != "tweet" {
		return RequestSpec{}, fmt.Errorf("unsupported mode")
	}
	return t.tweetRequest(body, title)
}

func (t *TwitterTarget) Send(body, title string, notifyType NotifyType) error {
	return t.SendWithAttachments(body, title, notifyType, nil)
}

func (t *TwitterTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	_ = notifyType

	if t.mode == "dm" {
		return t.sendDM(body, title)
	}
	if t.mode != "tweet" {
		return fmt.Errorf("unsupported mode")
	}

	batches, err := t.uploadMedia(attachments)
	if err != nil {
		return err
	}

	if len(batches) == 0 {
		spec, err := t.tweetRequest(body, title)
		if err != nil {
			return err
		}

		return SendRequest(spec)
	}

	message := mergeTitleBody(title, body)
	for index, mediaIDs := range batches {
		text := message
		// Only the first tweet carries the message; the rest are numbered so
		// a reader can tell they belong together.
		if index > 0 || message == "" {
			text = fmt.Sprintf("%02d/%02d", index+1, len(batches))
		}

		spec, err := t.tweetRequestWithMedia(text, mediaIDs)
		if err != nil {
			return err
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

// uploadMedia uploads each image and groups the ids into the tweets they will
// be posted in.
func (t *TwitterTarget) uploadMedia(attachments []Attachment) ([][]string, error) {
	batchSize := 1
	if t.batch {
		batchSize = twitterImageBatchSize
	}

	batches := [][]string{}
	current := []string{}

	for index, attachment := range attachments {
		// Images only; anything else is ignored rather than refused.
		if !strings.HasPrefix(strings.ToLower(attachment.MimeType), "image/") {
			continue
		}

		mediaID, err := t.uploadOne(attachment, index)
		if err != nil {
			return nil, err
		}

		// Only PNG and JPEG batch; a gif stands alone and splits the run.
		if !twitterBatchablePattern.MatchString(attachment.MimeType) {
			if len(current) > 0 {
				batches = append(batches, current)
				current = nil
			}
			batches = append(batches, []string{mediaID})
			continue
		}

		current = append(current, mediaID)
		if len(current) >= batchSize {
			batches = append(batches, current)
			current = nil
		}
	}

	if len(current) > 0 {
		batches = append(batches, current)
	}

	return batches, nil
}

var twitterBatchablePattern = regexp.MustCompile(`(?i)^image/(png|jpe?g)`)

// uploadOne posts a single image and returns the id a tweet references.
func (t *TwitterTarget) uploadOne(attachment Attachment, index int) (string, error) {
	category := "tweet_image"
	if t.mode == "dm" {
		category = "dm_image"
	}

	fields := formFields{}
	fields.Set("media_category", category)

	// Twitter is handed a filename and a handle with no type, so the part
	// carries no content type of its own.
	requestBody, contentType, err := singleFileAttachmentBody(
		fields, "media",
		Attachment{
			Name: attachment.FileName(index, ".dat"),
			Data: attachment.Data,
		}, false)
	if err != nil {
		return "", err
	}

	auth, err := buildOAuth1Header(
		"POST",
		twitterMediaURL,
		nil,
		t.consumerKey, t.consumerSecret, t.accessKey, t.accessSecret,
	)
	if err != nil {
		return "", err
	}

	var response struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := doJSONRequest(RequestSpec{
		Method: "POST",
		URL:    twitterMediaURL,
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Content-Type":  contentType,
			"Authorization": auth,
		},
		Body: requestBody,
	}, &response); err != nil {
		return "", err
	}
	if response.Data.ID == "" {
		return "", fmt.Errorf("twitter media upload returned no id")
	}

	return response.Data.ID, nil
}

func (t *TwitterTarget) tweetRequest(body, title string) (RequestSpec, error) {
	return t.tweetRequestWithMedia(mergeTitleBody(title, body), nil)
}

// tweetRequestWithMedia posts already prepared text, optionally referencing
// media that has been uploaded.
func (t *TwitterTarget) tweetRequestWithMedia(message string, mediaIDs []string) (RequestSpec, error) {
	payload := map[string]any{"text": message}
	if len(mediaIDs) > 0 {
		// The ids are nested rather than sitting at the top level.
		payload["media"] = map[string]any{"media_ids": mediaIDs}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	auth, err := buildOAuth1Header(
		"POST",
		twitterTweetURL,
		nil,
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
			"Content-Type":  "application/json",
			"Authorization": auth,
		},
		Body: string(data),
	}, nil
}

// twitterUser is the v2 user representation returned under a data envelope.
type twitterUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type twitterWhoamiResponse struct {
	Data twitterUser `json:"data"`
}

type twitterLookupResponse struct {
	Data []twitterUser `json:"data"`
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
		dmURL := fmt.Sprintf(twitterDMURLTemplate, recipient.ID)
		payload := map[string]string{"text": message}
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		auth, err := buildOAuth1Header(
			"POST",
			dmURL,
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
			URL:    dmURL,
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
	if response.Data.ID == "" || response.Data.Username == "" {
		return nil
	}
	return []twitterRecipient{{ScreenName: response.Data.Username, ID: response.Data.ID}}
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
		lookupURL := twitterLookupURL + "?usernames=" + url.QueryEscape(strings.Join(names[i:end], ","))

		auth, err := buildOAuth1Header(
			"GET",
			lookupURL,
			nil,
			t.consumerKey,
			t.consumerSecret,
			t.accessKey,
			t.accessSecret,
		)
		if err != nil {
			continue
		}

		spec := RequestSpec{
			Method: "GET",
			URL:    lookupURL,
			Headers: map[string]string{
				"User-Agent":    "Apprise",
				"Accept":        "*/*",
				"Authorization": auth,
			},
		}

		var response twitterLookupResponse
		if err := doJSONRequest(spec, &response); err != nil {
			continue
		}
		for _, entry := range response.Data {
			if entry.Username == "" || entry.ID == "" {
				continue
			}
			results[entry.Username] = entry.ID
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
