package notify

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/unraid/apprise-go/internal/version"
)

const embyDeviceID = "48df9504-6843-49be-9f2d-a685e25a0bc8"
const embyDefaultPort = 8096
const embyTimeoutMs = 60000

type EmbyTarget struct {
	host        string
	port        int
	secure      bool
	user        string
	password    string
	modal       bool
	accessToken string
	userID      string
}

func NewEmbyTarget(target *ParsedURL) (*EmbyTarget, error) {
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	user := strings.TrimSpace(target.User)
	if user == "" {
		return nil, fmt.Errorf("missing user")
	}

	secure := strings.EqualFold(target.Scheme, "embys")
	port := target.Port
	if port == 0 {
		port = embyDefaultPort
	}

	modal := parseBoolWithDefault(target.Query["modal"], false)

	return &EmbyTarget{
		host:     host,
		port:     port,
		secure:   secure,
		user:     user,
		password: target.Password,
		modal:    modal,
	}, nil
}

func (e *EmbyTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	payload := e.loginPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	_ = body
	_ = title
	_ = notifyType

	return RequestSpec{
		Method: "POST",
		URL:    e.baseURL() + "/Users/AuthenticateByName",
		Headers: map[string]string{
			"User-Agent":           "Apprise",
			"Content-Type":         "application/json",
			"X-Emby-Authorization": e.embyAuthHeader(),
		},
		Body: string(data),
	}, nil
}

func (e *EmbyTarget) Send(body, title string, notifyType NotifyType) error {
	if !e.isAuthenticated() {
		if err := e.login(); err != nil {
			return err
		}
	}

	sessions, err := e.sessions()
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		return nil
	}

	payload := map[string]any{
		"Header": title,
		"Text":   body,
	}
	if !e.modal {
		payload["TimeoutMs"] = embyTimeoutMs
	}
	payloadData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		requestURL := fmt.Sprintf("%s/Sessions/%s/Message", e.baseURL(), session)
		spec := RequestSpec{
			Method: "POST",
			URL:    requestURL,
			Headers: map[string]string{
				"User-Agent":           "Apprise",
				"Content-Type":         "application/json",
				"X-Emby-Authorization": e.embyAuthHeader(),
				"X-MediaBrowser-Token": e.accessToken,
			},
			Body: string(payloadData),
		}
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	_ = notifyType

	return nil
}

func (e *EmbyTarget) isAuthenticated() bool {
	return e.accessToken != "" && e.userID != ""
}

func (e *EmbyTarget) login() error {
	payload := e.loginPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	spec := RequestSpec{
		Method: "POST",
		URL:    e.baseURL() + "/Users/AuthenticateByName",
		Headers: map[string]string{
			"User-Agent":           "Apprise",
			"Content-Type":         "application/json",
			"X-Emby-Authorization": e.embyAuthHeader(),
		},
		Body: string(data),
	}

	req, err := spec.HTTPRequest()
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPStatusError{StatusCode: resp.StatusCode}
	}

	var response struct {
		AccessToken string `json:"AccessToken"`
		ID          string `json:"Id"`
		User        struct {
			ID string `json:"Id"`
		} `json:"User"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return err
	}

	e.accessToken = response.AccessToken
	e.userID = response.ID
	if e.userID == "" {
		e.userID = response.User.ID
	}

	if !e.isAuthenticated() {
		return fmt.Errorf("authentication failed")
	}

	return nil
}

func (e *EmbyTarget) sessions() ([]string, error) {
	if !e.isAuthenticated() {
		if err := e.login(); err != nil {
			return nil, err
		}
	}

	requestURL := e.baseURL() + "/Sessions"
	if e.userID != "" {
		params := url.Values{}
		params.Set("ControllableByUserId", e.userID)
		requestURL += "?" + params.Encode()
	}

	spec := RequestSpec{
		Method: "GET",
		URL:    requestURL,
		Headers: map[string]string{
			"User-Agent":           "Apprise",
			"Content-Type":         "application/json",
			"X-Emby-Authorization": e.embyAuthHeader(),
			"X-MediaBrowser-Token": e.accessToken,
		},
	}

	req, err := spec.HTTPRequest()
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode}
	}

	var response []struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	sessions := make([]string, 0, len(response))
	for _, entry := range response {
		if strings.TrimSpace(entry.ID) == "" {
			continue
		}
		sessions = append(sessions, entry.ID)
	}

	return sessions, nil
}

func (e *EmbyTarget) baseURL() string {
	scheme := "http"
	if e.secure {
		scheme = "https"
	}

	base := fmt.Sprintf("%s://%s", scheme, e.host)
	if e.port > 0 {
		base += fmt.Sprintf(":%d", e.port)
	}

	return base
}

func (e *EmbyTarget) embyAuthHeader() string {
	parts := []string{
		fmt.Sprintf(`MediaBrowser Client="%s"`, "Apprise"),
		fmt.Sprintf(`Device="%s"`, "Apprise"),
		fmt.Sprintf(`DeviceId="%s"`, embyDeviceID),
		fmt.Sprintf(`Version="%s"`, version.UpstreamVersion),
	}

	if e.userID != "" {
		parts = append(parts, fmt.Sprintf(`UserId="%s"`, e.user))
	}

	return strings.Join(parts, ", ")
}

func (e *EmbyTarget) loginPayload() map[string]string {
	payload := map[string]string{
		"Username": e.user,
	}

	if e.password != "" {
		payload["pw"] = e.password
		payload["passwordMd5"] = md5Hash(e.password)
		payload["password"] = sha1Hash(e.password)
	} else {
		payload["pw"] = ""
		payload["passwordMd5"] = ""
		payload["password"] = ""
	}

	return payload
}

func md5Hash(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func sha1Hash(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}
