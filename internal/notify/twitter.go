package notify

import (
	"fmt"
	"net/url"
	"strings"
)

const twitterTweetURL = "https://api.twitter.com/1.1/statuses/update.json"

type TwitterTarget struct {
	consumerKey    string
	consumerSecret string
	accessKey      string
	accessSecret   string
	mode           string
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

	mode := strings.TrimSpace(target.Query["mode"])
	if mode == "" {
		if strings.HasPrefix(target.Scheme, "tweet") {
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
	}, nil
}

func (t *TwitterTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if t.mode != "tweet" {
		return RequestSpec{}, fmt.Errorf("unsupported mode")
	}
	return t.tweetRequest(body)
}

func (t *TwitterTarget) Send(body, title string, notifyType NotifyType) error {
	if t.mode != "tweet" {
		return fmt.Errorf("unsupported mode")
	}
	spec, err := t.tweetRequest(body)
	if err != nil {
		return err
	}
	return SendRequest(spec)
}

func (t *TwitterTarget) tweetRequest(body string) (RequestSpec, error) {
	payload := url.Values{}
	payload.Set("status", body)

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
			"Content-Type":  "application/x-www-form-urlencoded",
			"Authorization": auth,
		},
		Body: payload.Encode(),
	}, nil
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
