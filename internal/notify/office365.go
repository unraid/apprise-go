package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const office365GraphURL = "https://graph.microsoft.com"

type Office365Target struct {
	tenant      string
	clientID    string
	secret      string
	source      string
	fromEmail   string
	fromName    string
	targets     []string
	token       string
	tokenExpiry time.Time
}

func NewOffice365Target(target *ParsedURL) (*Office365Target, error) {
	source := strings.TrimSpace(target.Query["from"])
	if source == "" {
		if target.User != "" && target.Host != "" {
			source = target.User + "@" + target.Host
		} else {
			source = target.Host
		}
	}
	if source == "" {
		return nil, fmt.Errorf("missing source")
	}

	entries := splitPath(target.Path)
	if len(entries) < 3 {
		return nil, fmt.Errorf("missing credentials")
	}

	tenant := strings.TrimSpace(entries[0])
	clientID := strings.TrimSpace(entries[1])
	remaining := entries[2:]

	targets := []string{}
	for len(remaining) > 0 {
		last := strings.TrimSpace(remaining[len(remaining)-1])
		if last == "" {
			remaining = remaining[:len(remaining)-1]
			continue
		}
		if !isSimpleEmail(last) {
			break
		}
		targets = append(targets, last)
		remaining = remaining[:len(remaining)-1]
	}
	for i, j := 0, len(targets)-1; i < j; i, j = i+1, j-1 {
		targets[i], targets[j] = targets[j], targets[i]
	}

	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		for _, entry := range parseDelimitedList(toValue) {
			if isSimpleEmail(entry) {
				targets = append(targets, entry)
			}
		}
	}

	secret := strings.TrimSpace(target.Query["oauth_secret"])
	if secret == "" {
		secret = strings.TrimSpace(strings.Join(remaining, "/"))
	}
	if secret == "" {
		return nil, fmt.Errorf("missing secret")
	}

	if rawTenant := strings.TrimSpace(target.Query["tenant"]); rawTenant != "" {
		tenant = rawTenant
	}
	if rawClient := strings.TrimSpace(target.Query["oauth_id"]); rawClient != "" {
		clientID = rawClient
	}

	if tenant == "" || clientID == "" {
		return nil, fmt.Errorf("missing tenant or client id")
	}

	fromEmail := ""
	if isSimpleEmail(source) {
		fromEmail = source
	}
	if len(targets) == 0 && fromEmail != "" {
		targets = append(targets, fromEmail)
	}

	return &Office365Target{
		tenant:    tenant,
		clientID:  clientID,
		secret:    secret,
		source:    source,
		fromEmail: fromEmail,
		fromName:  "Apprise",
		targets:   targets,
	}, nil
}

func (o *Office365Target) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	form := o.authPayload()
	return RequestSpec{
		Method: "POST",
		URL:    o.authURL(),
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/x-www-form-urlencoded",
		},
		Body: form.Encode(),
	}, nil
}

func (o *Office365Target) Send(body, title string, notifyType NotifyType) error {
	if len(o.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	if !o.isAuthenticated() {
		if err := o.authenticate(); err != nil {
			return err
		}
	}
	if o.fromEmail == "" {
		o.resolveFromEmail()
	}

	for _, target := range o.targets {
		payload, err := o.mailPayload(body, title, target)
		if err != nil {
			return err
		}
		spec := RequestSpec{
			Method: "POST",
			URL:    fmt.Sprintf("%s/v1.0/users/%s/sendMail", office365GraphURL, o.source),
			Headers: map[string]string{
				"User-Agent":   "Apprise",
				"Accept":       "*/*",
				"Content-Type": "application/json",
				"Authorization": fmt.Sprintf(
					"Bearer %s",
					o.token,
				),
			},
			Body: string(payload),
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = notifyType
	return nil
}

func (o *Office365Target) authURL() string {
	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", o.tenant)
}

func (o *Office365Target) authPayload() url.Values {
	values := url.Values{}
	values.Set("grant_type", "client_credentials")
	values.Set("client_id", o.clientID)
	values.Set("client_secret", o.secret)
	values.Set("scope", office365GraphURL+"/.default")
	return values
}

func (o *Office365Target) isAuthenticated() bool {
	if o.token == "" {
		return false
	}
	return fixedTime().Before(o.tokenExpiry)
}

func (o *Office365Target) authenticate() error {
	payload := o.authPayload().Encode()
	spec := RequestSpec{
		Method: "POST",
		URL:    o.authURL(),
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/x-www-form-urlencoded",
		},
		Body: payload,
	}

	var response struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := doJSONRequest(spec, &response); err != nil {
		return err
	}

	if response.AccessToken == "" {
		return fmt.Errorf("missing token")
	}

	expiry := fixedTime().Add(time.Duration(response.ExpiresIn-10) * time.Second)
	o.token = response.AccessToken
	o.tokenExpiry = expiry
	return nil
}

func (o *Office365Target) mailPayload(body, title, target string) ([]byte, error) {
	message := map[string]any{
		"subject": title,
		"body": map[string]string{
			"contentType": "HTML",
			"content":     body,
		},
		"toRecipients": []map[string]any{
			{
				"emailAddress": map[string]string{
					"address": target,
				},
			},
		},
	}

	if o.fromEmail != "" {
		message["from"] = map[string]any{
			"emailAddress": map[string]string{
				"address": o.fromEmail,
				"name":    o.fromName,
			},
		}
	}

	payload := map[string]any{
		"message":         message,
		"saveToSentItems": "true",
	}
	return json.Marshal(payload)
}

func (o *Office365Target) resolveFromEmail() {
	spec := RequestSpec{
		Method: "GET",
		URL:    fmt.Sprintf("%s/v1.0/users/%s", office365GraphURL, o.source),
		Headers: map[string]string{
			"User-Agent":    "Apprise",
			"Accept":        "*/*",
			"Content-Type":  "application/json",
			"Authorization": fmt.Sprintf("Bearer %s", o.token),
		},
		Body: "null",
	}

	var response struct {
		Mail              string `json:"mail"`
		UserPrincipalName string `json:"userPrincipalName"`
		DisplayName       string `json:"displayName"`
	}
	if err := doJSONRequest(spec, &response); err != nil {
		return
	}

	email := strings.TrimSpace(response.Mail)
	if email == "" {
		email = strings.TrimSpace(response.UserPrincipalName)
	}
	if !isSimpleEmail(email) {
		return
	}

	o.fromEmail = email
	if response.DisplayName != "" {
		o.fromName = response.DisplayName
	}
}
