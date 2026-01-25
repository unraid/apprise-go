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
		advert:     parseBoolValue(target.Query["ad"], false),
		resubmit:   parseBoolValue(target.Query["resubmit"], false),
		sendreply:  parseBoolValue(target.Query["replies"], true),
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
