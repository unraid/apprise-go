package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	office365GraphURL = "https://graph.microsoft.com"

	// Anything past this has to be uploaded against a saved draft rather
	// than carried inline in the message.
	office365InlineAttachmentMax = 3145728

	office365UploadChunkSize = 5242880
)

// Personal (consumer) accounts authenticate through the consumers endpoint
// with a delegated refresh token rather than client credentials.
const office365PersonalAuthURL = "https://login.microsoftonline.com/consumers/oauth2/v2.0/token"
const office365PersonalScope = "https://graph.microsoft.com/Mail.Send offline_access"

// office365PersonalDomains routes a source address to the personal flow. Use
// ?mode= to override the guess.
var office365PersonalDomains = map[string]struct{}{}

func init() {
	for _, domain := range strings.Split(office365PersonalDomainList, " ") {
		office365PersonalDomains[domain] = struct{}{}
	}
}

const office365PersonalDomainList = "hotmail.at hotmail.be hotmail.ca hotmail.cl hotmail.co.uk hotmail.com " +
	"hotmail.com.ar hotmail.com.au hotmail.com.br hotmail.com.mx hotmail.cz hotmail.de hotmail.dk " +
	"hotmail.es hotmail.fi hotmail.fr hotmail.gr hotmail.hu hotmail.ie hotmail.it hotmail.nl " +
	"hotmail.no hotmail.pt hotmail.rs hotmail.se live.at live.be live.ca live.cl live.co.nz " +
	"live.co.uk live.com live.com.ar live.com.au live.com.mx live.de live.dk live.es live.fi " +
	"live.fr live.gr live.hu live.ie live.in live.it live.jp live.mx live.my live.nl live.no " +
	"live.ph live.pt live.rs live.se live.sg msn.com outlook.at outlook.be outlook.ca outlook.cl " +
	"outlook.co.nz outlook.co.uk outlook.com outlook.com.ar outlook.com.au outlook.com.br " +
	"outlook.com.mx outlook.cz outlook.de outlook.dk outlook.es outlook.fi outlook.fr outlook.hu " +
	"outlook.ie outlook.in outlook.it outlook.jp outlook.kr outlook.lv outlook.my outlook.nl " +
	"outlook.ph outlook.pt outlook.rs outlook.sa outlook.sg outlook.sk"

type Office365Target struct {
	tenant      string
	clientID    string
	secret      string
	source      string
	fromEmail   string
	fromName    string
	targets     []string
	replyTo     []string
	mode        string
	saveSent    bool
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

	// The mode decides how many credential segments the path carries, so it
	// has to be resolved before any of them are consumed: org mode leads with
	// a tenant, personal mode has none.
	mode := strings.ToLower(strings.TrimSpace(target.Query["mode"]))
	switch mode {
	case "org", "personal":
	case "":
		// A consumer domain in the source address means the personal flow.
		mode = "org"
		if at := strings.LastIndex(source, "@"); at >= 0 {
			domain := strings.ToLower(source[at+1:])
			if _, ok := office365PersonalDomains[domain]; ok {
				mode = "personal"
			}
		}
	default:
		return nil, fmt.Errorf("invalid mode: %s", target.Query["mode"])
	}

	entries := splitPath(target.Path)

	tenant := ""
	if mode == "org" {
		if len(entries) == 0 {
			return nil, fmt.Errorf("missing credentials")
		}
		tenant, entries = strings.TrimSpace(entries[0]), entries[1:]
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("missing credentials")
	}
	clientID := strings.TrimSpace(entries[0])
	remaining := entries[1:]

	// Reading from the end, every trailing email is a recipient; whatever is
	// left is the secret, which may itself contain slashes.
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

	if clientID == "" {
		return nil, fmt.Errorf("missing client id")
	}
	// Personal mode authenticates against the consumers endpoint, so it has
	// no tenant of its own.
	if mode == "org" && tenant == "" {
		return nil, fmt.Errorf("missing tenant")
	}

	replyTo := []string{}
	if raw := strings.TrimSpace(target.Query["reply_to"]); raw != "" {
		for _, entry := range parseDelimitedList(raw) {
			if isSimpleEmail(entry) {
				replyTo = append(replyTo, entry)
			}
		}
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
		replyTo:   replyTo,
		mode:      mode,
		saveSent:  parseBoolWithDefault(target.Query["savesent"], true),
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
	return o.SendWithAttachments(body, title, notifyType, nil)
}

// SendWithAttachments splits the files by size. Anything Graph will accept
// inline rides along in the message; anything larger forces the message to be
// saved as a draft first, so each file has a message to be uploaded against
// before the draft is sent.
func (o *Office365Target) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	if len(o.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	if !o.isAuthenticated() {
		if err := o.authenticate(); err != nil {
			return err
		}
	}
	// Personal mode always knows its own address; only org mode may have
	// been given an Object ID that needs resolving.
	if o.fromEmail == "" && o.mode != "personal" {
		o.resolveFromEmail()
	}

	var small, large []Attachment
	for _, attachment := range attachments {
		if len(attachment.Data) > office365InlineAttachmentMax {
			large = append(large, attachment)
			continue
		}
		small = append(small, attachment)
	}

	for _, target := range o.targets {
		payload, err := o.mailPayload(body, title, target, small)
		if err != nil {
			return err
		}

		// With a large file the same payload creates a draft instead of
		// sending, and the send happens once the uploads are done.
		url := office365GraphURL + o.mailboxPath() + "/sendMail"
		if len(large) > 0 {
			url = office365GraphURL + o.mailboxPath() + "/messages"
		}

		spec := RequestSpec{
			Method:  "POST",
			URL:     url,
			Headers: o.graphHeaders(),
			Body:    string(payload),
		}

		if len(large) == 0 {
			if err := SendRequest(spec); err != nil {
				return err
			}
			continue
		}

		var draft struct {
			ID string `json:"id"`
		}
		if err := doJSONRequest(spec, &draft); err != nil {
			return err
		}
		if draft.ID == "" {
			return fmt.Errorf("email draft id could not be retrieved")
		}

		for index, attachment := range large {
			if err := o.uploadAttachment(attachment, draft.ID, index); err != nil {
				return err
			}
		}

		if err := SendRequest(RequestSpec{
			Method: "POST",
			URL: fmt.Sprintf("%s%s/messages/%s/send",
				office365GraphURL, o.mailboxPath(), draft.ID),
			Headers: o.graphHeaders(),
			// The send takes no payload, but upstream still serializes the
			// absent one, so the body is the JSON literal null.
			Body: "null",
		}); err != nil {
			return err
		}
	}

	_ = notifyType
	return nil
}

func (o *Office365Target) graphHeaders() map[string]string {
	return map[string]string{
		"User-Agent":    "Apprise",
		"Accept":        "*/*",
		"Content-Type":  "application/json",
		"Authorization": fmt.Sprintf("Bearer %s", o.token),
	}
}

// uploadAttachment opens an upload session for one file and streams it to the
// URL the session returns. Note the session path is /message/, not /messages/
// as everywhere else; that is upstream's spelling.
func (o *Office365Target) uploadAttachment(attachment Attachment, messageID string, index int) error {
	payload, err := json.Marshal(map[string]any{
		"AttachmentItem": map[string]any{
			"attachmentType": "file",
			"name":           attachment.FileName(index, ".dat"),
			"contentType":    attachment.MIMEType,
			"size":           len(attachment.Data),
		},
	})
	if err != nil {
		return err
	}

	var session struct {
		UploadURL string `json:"uploadUrl"`
	}
	if err := doJSONRequest(RequestSpec{
		Method: "POST",
		URL: fmt.Sprintf("%s%s/message/%s/attachments/createUploadSession",
			office365GraphURL, o.mailboxPath(), messageID),
		Headers: o.graphHeaders(),
		Body:    string(payload),
	}, &session); err != nil {
		return err
	}
	if session.UploadURL == "" {
		return fmt.Errorf("no upload url for attachment %s", attachment.Name)
	}

	size := len(attachment.Data)
	for start := 0; start < size; start += office365UploadChunkSize {
		end := start + office365UploadChunkSize
		if end > size {
			end = size
		}

		chunk := attachment.Data[start:end]
		if err := SendRequest(RequestSpec{
			Method: "PUT",
			URL:    session.UploadURL,
			Headers: map[string]string{
				"User-Agent": "Apprise",
				"Accept":     "*/*",
				// No content type: the chunk is a slice of the file, not a
				// document of its own. Length is left to the transport.
				"Content-Range": fmt.Sprintf("bytes %d-%d/%d",
					start, end-1, size),
				"Authorization": fmt.Sprintf("Bearer %s", o.token),
			},
			Body: string(chunk),
		}); err != nil {
			return err
		}
	}

	return nil
}

func (o *Office365Target) authURL() string {
	if o.mode == "personal" {
		return office365PersonalAuthURL
	}

	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", o.tenant)
}

func (o *Office365Target) authPayload() url.Values {
	values := url.Values{}
	if o.mode == "personal" {
		// The secret is a seed refresh token in this mode, not a client
		// secret, and the grant rotates it.
		values.Set("grant_type", "refresh_token")
		values.Set("client_id", o.clientID)
		values.Set("refresh_token", o.secret)
		values.Set("scope", office365PersonalScope)

		return values
	}

	values.Set("grant_type", "client_credentials")
	values.Set("client_id", o.clientID)
	values.Set("client_secret", o.secret)
	values.Set("scope", office365GraphURL+"/.default")

	return values
}

// mailboxPath is the graph path for the sending mailbox; a personal account
// only ever addresses its own.
func (o *Office365Target) mailboxPath() string {
	if o.mode == "personal" {
		return "/v1.0/me"
	}

	return "/v1.0/users/" + o.source
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

func (o *Office365Target) mailPayload(body, title, target string, attachments []Attachment) ([]byte, error) {
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

	if len(o.replyTo) > 0 {
		addresses := make([]map[string]any, 0, len(o.replyTo))
		for _, address := range o.replyTo {
			addresses = append(addresses, map[string]any{
				"emailAddress": map[string]string{"address": address},
			})
		}
		message["replyTo"] = addresses
	}

	if len(attachments) > 0 {
		entries := make([]map[string]any, 0, len(attachments))
		for index, attachment := range attachments {
			entries = append(entries, map[string]any{
				"@odata.type": "#microsoft.graph.fileAttachment",
				"name":        attachment.FileName(index, ".dat"),
				// Upstream sends the literal string rather than the file's
				// type here. Matching it is the point of this port.
				"contentType":  "attachment.mimetype",
				"contentBytes": attachment.Base64(),
			})
		}
		message["attachments"] = entries
	}

	// The API wants the string "true"/"false" here, not a boolean.
	payload := map[string]any{
		"message":         message,
		"saveToSentItems": strconv.FormatBool(o.saveSent),
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
