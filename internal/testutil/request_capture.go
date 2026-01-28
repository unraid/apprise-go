package testutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
)

type captureTransport struct {
	requests []notify.RequestSpec
}

var captureMu sync.Mutex

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := ""
	if req.Body != nil {
		data, err := io.ReadAll(req.Body)
		if err == nil {
			body = string(data)
		}
		_ = req.Body.Close()
	}

	headers := map[string]string{}
	for key, values := range req.Header {
		headers[key] = strings.Join(values, ",")
	}

	c.requests = append(c.requests, notify.RequestSpec{
		Method:  req.Method,
		URL:     req.URL.String(),
		Headers: headers,
		Body:    body,
	})

	responseBody := "ok"
	contentType := "text/plain"
	if strings.Contains(req.URL.String(), "sendpulse.com/oauth/access_token") {
		responseBody = `{"access_token":"token","expires_in":3600}`
		contentType = "application/json"
	} else if strings.Contains(req.URL.String(), "reddit.com/api/v1/access_token") {
		responseBody = `{"access_token":"token","expires_in":3600}`
		contentType = "application/json"
	} else if req.URL.Host == "public.api.bsky.app" && strings.HasSuffix(req.URL.Path, "/xrpc/com.atproto.identity.resolveHandle") {
		responseBody = `{"did":"did:plc:123"}`
		contentType = "application/json"
	} else if req.URL.Host == "plc.directory" && strings.HasPrefix(req.URL.Path, "/did:plc:") {
		responseBody = `{"service":[{"type":"AtprotoPersonalDataServer","serviceEndpoint":"https://bsky.social"}]}`
		contentType = "application/json"
	} else if strings.Contains(req.URL.Host, "login.microsoftonline.com") && strings.HasSuffix(req.URL.Path, "/oauth2/v2.0/token") {
		responseBody = `{"access_token":"token","expires_in":3600}`
		contentType = "application/json"
	} else if req.URL.Host == "graph.microsoft.com" && strings.HasPrefix(req.URL.Path, "/v1.0/users/") && req.Method == http.MethodGet {
		responseBody = `{"mail":"user@example.com","userPrincipalName":"user@example.com","displayName":"Apprise"}`
		contentType = "application/json"
	} else if req.URL.Host == "api.twitter.com" && strings.HasSuffix(req.URL.Path, "/users/lookup.json") {
		names := []string{}
		if values, err := url.ParseQuery(body); err == nil {
			names = values["screen_name"]
			if len(names) == 0 {
				if value := values.Get("screen_name"); value != "" {
					names = []string{value}
				}
			}
		}
		if len(names) == 0 {
			names = []string{"user"}
		}
		entries := make([]map[string]string, 0, len(names))
		for _, name := range names {
			entries = append(entries, map[string]string{
				"screen_name": name,
				"id":          "123",
				"id_str":      "123",
			})
		}
		if data, err := json.Marshal(entries); err == nil {
			responseBody = string(data)
			contentType = "application/json"
		}
	} else if req.URL.Host == "api.twitter.com" && strings.HasSuffix(req.URL.Path, "/account/verify_credentials.json") {
		responseBody = `{"screen_name":"apprise","id":"123","id_str":"123"}`
		contentType = "application/json"
	} else if req.URL.Host == "slack.com" && req.URL.Path == "/api/users.lookupByEmail" {
		responseBody = `{"ok":true,"user":{"id":"U123"}}`
		contentType = "application/json"
	} else if req.URL.Host == "slack.com" && req.URL.Path == "/api/chat.postMessage" {
		responseBody = `{"ok":true,"ts":"123.456"}`
		contentType = "application/json"
	} else if strings.HasSuffix(req.URL.Path, "/xrpc/com.atproto.server.createSession") {
		responseBody = `{"accessJwt":"token","refreshJwt":"refresh"}`
		contentType = "application/json"
	} else if strings.HasSuffix(req.URL.Path, "/xrpc/com.atproto.repo.createRecord") {
		responseBody = `{"uri":"at://example/post"}`
		contentType = "application/json"
	} else if strings.Contains(req.URL.Path, "/Users/AuthenticateByName") {
		responseBody = `{"AccessToken":"token","Id":"user-id","User":{"Id":"user-id"}}`
		contentType = "application/json"
	} else if req.Method == http.MethodGet && req.URL.Path == "/Sessions" {
		responseBody = `[{"Id":"session-id"}]`
		contentType = "application/json"
	} else if req.URL.Host == "api.twist.com" && strings.HasSuffix(req.URL.Path, "/users/login") {
		responseBody = `{"token":"token","default_workspace":12345}`
		contentType = "application/json"
	} else if req.URL.Host == "api.twist.com" && strings.HasSuffix(req.URL.Path, "/channels/get") {
		responseBody = `[{"id":123,"name":"general","workspace_id":12345}]`
		contentType = "application/json"
	} else if req.URL.Host == "oauth2.googleapis.com" && req.URL.Path == "/token" {
		responseBody = `{"access_token":"token","expires_in":3600}`
		contentType = "application/json"
	} else if req.URL.Path == "/.well-known/matrix/client" {
		scheme := req.URL.Scheme
		if scheme == "" {
			scheme = "https"
		}
		baseURL := fmt.Sprintf("%s://%s", scheme, req.URL.Host)
		responseBody = fmt.Sprintf(`{"m.homeserver":{"base_url":"%s"}}`, baseURL)
		contentType = "application/json"
	} else if strings.HasSuffix(req.URL.Path, "/_matrix/client/versions") {
		responseBody = `{"versions":["r0"]}`
		contentType = "application/json"
	} else if strings.Contains(req.URL.Path, "/_matrix/client/") && strings.HasSuffix(req.URL.Path, "/login") {
		host := req.URL.Hostname()
		if host == "" {
			host = req.URL.Host
		}
		responseBody = fmt.Sprintf(`{"access_token":"token","home_server":"%s","user_id":"@user:%s"}`, host, host)
		contentType = "application/json"
	} else if strings.Contains(req.URL.Path, "/_matrix/client/") && strings.HasSuffix(req.URL.Path, "/register") {
		host := req.URL.Hostname()
		if host == "" {
			host = req.URL.Host
		}
		responseBody = fmt.Sprintf(`{"access_token":"token","home_server":"%s","user_id":"@user:%s"}`, host, host)
		contentType = "application/json"
	} else if strings.Contains(req.URL.Path, "/_matrix/client/") && strings.Contains(req.URL.Path, "/join/") {
		host := req.URL.Hostname()
		if host == "" {
			host = req.URL.Host
		}
		responseBody = fmt.Sprintf(`{"room_id":"!room:%s"}`, host)
		contentType = "application/json"
	} else if strings.Contains(req.URL.Path, "/_matrix/client/") && strings.HasSuffix(req.URL.Path, "/createRoom") {
		host := req.URL.Hostname()
		if host == "" {
			host = req.URL.Host
		}
		responseBody = fmt.Sprintf(`{"room_id":"!room:%s","room_alias":"#room:%s"}`, host, host)
		contentType = "application/json"
	} else if strings.Contains(req.URL.Path, "/_matrix/client/") && strings.Contains(req.URL.Path, "/directory/room/") {
		host := req.URL.Hostname()
		if host == "" {
			host = req.URL.Host
		}
		responseBody = fmt.Sprintf(`{"room_id":"!room:%s"}`, host)
		contentType = "application/json"
	} else if strings.Contains(req.URL.Path, "/_matrix/client/") && strings.HasSuffix(req.URL.Path, "/joined_rooms") {
		host := req.URL.Hostname()
		if host == "" {
			host = req.URL.Host
		}
		responseBody = fmt.Sprintf(`{"joined_rooms":["#room:%s"]}`, host)
		contentType = "application/json"
	} else if strings.Contains(req.URL.Path, "/_matrix/client/") && strings.Contains(req.URL.Path, "/send/m.room.message") {
		responseBody = `{"event_id":"$event"}`
		contentType = "application/json"
	} else if strings.Contains(req.URL.Path, "/_matrix/client/") && strings.HasSuffix(req.URL.Path, "/logout") {
		responseBody = `{}`
		contentType = "application/json"
	} else if strings.Contains(req.URL.Path, "/_matrix/media/") && strings.HasSuffix(req.URL.Path, "/upload") {
		host := req.URL.Hostname()
		if host == "" {
			host = req.URL.Host
		}
		responseBody = fmt.Sprintf(`{"content_uri":"mxc://%s/abc"}`, host)
		contentType = "application/json"
	} else if strings.HasSuffix(req.URL.Path, "/api/v1/login") {
		responseBody = `{"status":"success","data":{"authToken":"token","userId":"user-id"}}`
		contentType = "application/json"
	} else if strings.HasSuffix(req.URL.Path, "/api/v1/logout") {
		responseBody = `{}`
		contentType = "application/json"
	} else if req.URL.Host == "wxpusher.zjiecode.com" && req.URL.Path == "/api/send/message" {
		responseBody = `{"code":1000,"msg":"ok"}`
		contentType = "application/json"
	} else if strings.HasSuffix(req.URL.Host, "notificationapi.com") {
		responseBody = `{"ok":true}`
		contentType = "application/json"
	} else if req.URL.Host == "api.sendpulse.com" && strings.HasSuffix(req.URL.Path, "/smtp/emails") {
		responseBody = `{"result":true}`
		contentType = "application/json"
	} else if req.URL.Host == "www.pushsafer.com" && req.URL.Path == "/api" {
		responseBody = `{"status":1,"success":"ok"}`
		contentType = "application/json"
	} else if req.URL.Host == "oauth.reddit.com" && req.URL.Path == "/api/submit" {
		responseBody = `{"json":{"errors":[]}}`
		contentType = "application/json"
	} else if req.URL.Host == "api.simplepush.io" && req.URL.Path == "/send" {
		responseBody = `{"status":"OK","message":"OK"}`
		contentType = "application/json"
	} else if req.URL.Host == "voip.ms" && req.URL.Path == "/api/v1/rest.php" {
		responseBody = `{"status":"success","message":"ok"}`
		contentType = "application/json"
	} else if strings.HasSuffix(req.URL.Path, "/api/message") {
		responseBody = `{"result":true}`
		contentType = "application/json"
	} else if req.URL.Hostname() == "www.hampager.de" && req.URL.Path == "/calls" {
		responseBody = `{"ok":true}`
		contentType = "application/json"
	} else if req.URL.Host == "api.pushy.me" && req.URL.Path == "/push" {
		responseBody = `{"success":true,"id":"id","info":{"devices":1}}`
		contentType = "application/json"
	} else if req.URL.Path == "/jsonrpc/sms" {
		responseBody = `{"result":{"status":"ok"}}`
		contentType = "application/json"
	} else if req.URL.Path == "/v2/alerts" && (req.URL.Host == "api.opsgenie.com" || req.URL.Host == "api.eu.opsgenie.com") {
		responseBody = `{"requestId":"request"}`
		contentType = "application/json"
	} else if req.URL.Host == "www.dmc.sfr-sh.fr" && strings.HasSuffix(req.URL.Path, "/DmcWS/1.5.8/JsonService/MessagesUnitairesWS/addSingleCall") {
		responseBody = `{"success":true}`
		contentType = "application/json"
	} else if strings.Contains(req.URL.Host, "sns.") && strings.Contains(body, "Action=CreateTopic") {
		responseBody = `<CreateTopicResponse><CreateTopicResult><TopicArn>arn:aws:sns:us-east-1:000000000000:topic</TopicArn></CreateTopicResult></CreateTopicResponse>`
		contentType = "application/xml"
	}

	status := http.StatusOK
	if req.URL.Hostname() == "www.hampager.de" && req.URL.Path == "/calls" {
		status = http.StatusCreated
	}
	resp := &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(responseBody)),
		Header:     make(http.Header),
		Request:    req,
	}
	resp.Header.Set("Content-Type", contentType)

	return resp, nil
}

func CaptureGoRequests(t *testing.T, send func() error) []notify.RequestSpec {
	t.Helper()

	specs, err := CaptureGoRequestsResult(t, send)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	return specs
}

func CaptureGoRequestsResult(t *testing.T, send func() error) ([]notify.RequestSpec, error) {
	t.Helper()

	captureMu.Lock()
	defer captureMu.Unlock()

	capture := &captureTransport{}
	previous := http.DefaultTransport
	http.DefaultTransport = capture
	defer func() {
		http.DefaultTransport = previous
	}()

	err := send()
	return capture.requests, err
}
