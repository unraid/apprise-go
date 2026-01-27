package notify

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/unraid/apprise-go/internal/version"
)

const redditAuthURL = "https://www.reddit.com/api/v1/access_token"
const redditSubmitURL = "https://oauth.reddit.com/api/submit"
const redditDefaultTitle = "Apprise Notifications"

type RedditTarget struct {
	user       string
	password   string
	appID      string
	appSecret  string
	subreddits []string
	kind       string
	nsfw       bool
	advert     bool
	resubmit   bool
	sendreply  bool
	spoiler    bool
	flairID    string
	flairText  string
	token      string
}

func NewRedditTarget(target *ParsedURL) (*RedditTarget, error) {
	user := strings.TrimSpace(target.User)
	password := strings.TrimSpace(target.Password)
	if user == "" || password == "" {
		return nil, fmt.Errorf("missing reddit credentials")
	}

	appID := strings.TrimSpace(target.Query["app_id"])
	if appID == "" {
		appID = strings.TrimSpace(target.Host)
	}
	if appID == "" {
		return nil, fmt.Errorf("missing app id")
	}

	pathEntries := splitPath(target.Path)
	appSecret := strings.TrimSpace(target.Query["app_secret"])
	if appSecret == "" {
		if len(pathEntries) == 0 {
			return nil, fmt.Errorf("missing app secret")
		}
		appSecret = pathEntries[0]
		pathEntries = pathEntries[1:]
	}

	subreddits := make([]string, 0, len(pathEntries))
	for _, entry := range pathEntries {
		if trimmed := strings.TrimSpace(entry); trimmed != "" {
			subreddits = append(subreddits, trimmed)
		}
	}
	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		subreddits = append(subreddits, parseDelimitedList(toValue)...)
	}
	if len(subreddits) == 0 {
		return nil, fmt.Errorf("missing subreddits")
	}

	kind := strings.ToLower(strings.TrimSpace(target.Query["kind"]))
	if kind == "" {
		kind = "auto"
	}

	return &RedditTarget{
		user:       user,
		password:   password,
		appID:      appID,
		appSecret:  appSecret,
		subreddits: subreddits,
		kind:       kind,
		nsfw:       parseBoolValue(target.Query["nsfw"], false),
		resubmit:   parseBoolValue(target.Query["resubmit"], false),
		advert:     false,
		sendreply:  true,
		spoiler:    parseBoolValue(target.Query["spoiler"], false),
		flairID:    strings.TrimSpace(target.Query["flair_id"]),
		flairText:  strings.TrimSpace(target.Query["flair_text"]),
	}, nil
}

func (r *RedditTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(r.subreddits) == 0 {
		return RequestSpec{}, fmt.Errorf("missing subreddits")
	}
	if r.token == "" {
		return RequestSpec{}, fmt.Errorf("missing access token")
	}

	payload := r.buildSubmitPayload(body, title, r.subreddits[0])
	form := encodeRedditForm(payload)

	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    redditSubmitURL,
		Headers: map[string]string{
			"User-Agent":    redditUserAgent(),
			"Content-Type":  "application/x-www-form-urlencoded",
			"Authorization": "Bearer " + r.token,
		},
		Body: form,
	}, nil
}

func (r *RedditTarget) Send(body, title string, notifyType NotifyType) error {
	if r.token == "" {
		if err := r.login(); err != nil {
			return err
		}
	}

	for _, subreddit := range r.subreddits {
		payload := r.buildSubmitPayload(body, title, subreddit)
		form := encodeRedditForm(payload)
		spec := RequestSpec{
			Method: "POST",
			URL:    redditSubmitURL,
			Headers: map[string]string{
				"User-Agent":    redditUserAgent(),
				"Content-Type":  "application/x-www-form-urlencoded",
				"Authorization": "Bearer " + r.token,
			},
			Body: form,
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = notifyType

	return nil
}

func (r *RedditTarget) login() error {
	values := url.Values{}
	values.Set("grant_type", "password")
	values.Set("username", r.user)
	values.Set("password", r.password)

	spec := RequestSpec{
		Method: "POST",
		URL:    redditAuthURL,
		Headers: map[string]string{
			"User-Agent":    redditUserAgent(),
			"Content-Type":  "application/x-www-form-urlencoded",
			"Authorization": basicAuthHeader(r.appID, r.appSecret),
		},
		Body: values.Encode(),
	}

	var response struct {
		AccessToken string `json:"access_token"`
	}
	if err := doJSONRequest(spec, &response); err != nil {
		return err
	}
	if response.AccessToken == "" {
		return fmt.Errorf("missing access token")
	}
	r.token = response.AccessToken
	return nil
}

func (r *RedditTarget) buildSubmitPayload(body, title, subreddit string) map[string]string {
	kind := r.kind
	if kind == "auto" {
		if isURL(body) {
			kind = "link"
		} else {
			kind = "self"
		}
	}

	payload := map[string]string{
		"ad":          formatRedditBool(r.advert),
		"api_type":    "json",
		"extension":   "json",
		"sr":          subreddit,
		"title":       titleOrDefault(title),
		"kind":        kind,
		"nsfw":        formatRedditBool(r.nsfw),
		"resubmit":    formatRedditBool(r.resubmit),
		"sendreplies": formatRedditBool(r.sendreply),
		"spoiler":     formatRedditBool(r.spoiler),
	}

	if r.flairID != "" {
		payload["flair_id"] = r.flairID
	}
	if r.flairText != "" {
		payload["flair_text"] = r.flairText
	}

	if kind == "link" {
		payload["url"] = body
	} else {
		payload["text"] = body
	}

	return payload
}

func titleOrDefault(title string) string {
	if strings.TrimSpace(title) == "" {
		return redditDefaultTitle
	}
	return title
}

func isURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func formatRedditBool(value bool) string {
	if value {
		return "True"
	}
	return "False"
}

func encodeRedditForm(values map[string]string) string {
	form := url.Values{}
	for key, value := range values {
		form.Set(key, value)
	}
	return form.Encode()
}

func redditUserAgent() string {
	return fmt.Sprintf("%s v%s", version.Title, version.UpstreamVersion)
}

func init() {
	RegisterSchemaEntryOrdered(106, SchemaEntry{
		"attachment_support": false,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"ad": map[string]any{
					"default":  false,
					"map_to":   "advertisement",
					"name":     "Is Ad?",
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
				"flair_id": map[string]any{
					"map_to":   "flair_id",
					"name":     "Flair ID",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"flair_text": map[string]any{
					"map_to":   "flair_text",
					"name":     "Flair Text",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"format": map[string]any{
					"default":  "markdown",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"kind": map[string]any{
					"default":  "auto",
					"map_to":   "kind",
					"name":     "Kind",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"auto", "self", "link"},
				},
				"nsfw": map[string]any{
					"default":  false,
					"map_to":   "nsfw",
					"name":     "NSFW",
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
				"replies": map[string]any{
					"default":  true,
					"map_to":   "sendreplies",
					"name":     "Send Replies",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"resubmit": map[string]any{
					"default":  false,
					"map_to":   "resubmit",
					"name":     "Resubmit Flag",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"rto": map[string]any{
					"default":  4,
					"map_to":   "rto",
					"name":     "Socket Read Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"spoiler": map[string]any{
					"default":  false,
					"map_to":   "spoiler",
					"name":     "Is Spoiler",
					"private":  false,
					"required": false,
					"type":     "bool",
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
			"templates": []string{"{schema}://{user}:{password}@{app_id}/{app_secret}/{targets}"},
			"tokens": map[string]any{
				"app_id": map[string]any{
					"map_to":   "app_id",
					"name":     "Application ID",
					"private":  true,
					"regex":    []string{"^[a-z0-9_-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"app_secret": map[string]any{
					"map_to":   "app_secret",
					"name":     "Application Secret",
					"private":  true,
					"regex":    []string{"^[a-z0-9_-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"password": map[string]any{
					"map_to":   "password",
					"name":     "Password",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"default":  "reddit",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"reddit"},
				},
				"target_subreddit": map[string]any{
					"map_to":   "targets",
					"name":     "Target Subreddit",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_subreddit"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": true,
					"type":     "list:string",
				},
				"user": map[string]any{
					"map_to":   "user",
					"name":     "User Name",
					"private":  false,
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
		"secure_protocols": []string{"reddit"},
		"service_name":     "Reddit",
		"service_url":      "https://reddit.com",
		"setup_url":        "https://appriseit.com/services/reddit/",
	})
}
