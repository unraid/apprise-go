package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNewWPushTarget(t *testing.T) {
	t.Parallel()

	parsed, err := ParseURL("wpush://WPUSHabc123?channel=feishu&topic_code=topic1")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	target, err := NewWPushTarget(parsed)
	if err != nil {
		t.Fatalf("new target: %v", err)
	}
	if target.apiKey != "WPUSHabc123" {
		t.Fatalf("apikey=%q", target.apiKey)
	}
	if target.channel != "feishu" {
		t.Fatalf("channel=%q", target.channel)
	}
	if target.topicCode != "topic1" {
		t.Fatalf("topic=%q", target.topicCode)
	}
}

func TestNewWPushTargetQueryAPIKey(t *testing.T) {
	t.Parallel()

	parsed, err := ParseURL("wpush://?apikey=WPUSHxyz99&channel=wechat")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	target, err := NewWPushTarget(parsed)
	if err != nil {
		t.Fatalf("new target: %v", err)
	}
	if target.apiKey != "WPUSHxyz99" {
		t.Fatalf("apikey=%q", target.apiKey)
	}
}

func TestNewWPushTargetRejectsBadKey(t *testing.T) {
	t.Parallel()

	parsed, err := ParseURL("wpush://notavalidkey")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if _, err := NewWPushTarget(parsed); err == nil {
		t.Fatal("expected invalid apikey error")
	}
}

func TestWPushBuildRequest(t *testing.T) {
	t.Parallel()

	parsed, err := ParseURL("wpush://WPUSHabc123?channel=app&to=mytopic")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	target, err := NewWPushTarget(parsed)
	if err != nil {
		t.Fatalf("new target: %v", err)
	}
	spec, err := target.BuildRequest("body text", "", NotifyInfo)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if spec.URL != wpushURL {
		t.Fatalf("url=%q", spec.URL)
	}
	if spec.Method != "POST" {
		t.Fatalf("method=%q", spec.Method)
	}
	if got := spec.Headers["Content-Type"]; got != "application/json" {
		t.Fatalf("content-type=%q", got)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(spec.Body), &payload); err != nil {
		t.Fatalf("json: %v body=%s", err, spec.Body)
	}
	if payload["apikey"] != "WPUSHabc123" {
		t.Fatalf("apikey=%v", payload["apikey"])
	}
	if payload["title"] != "body text" {
		t.Fatalf("title fallback=%v", payload["title"])
	}
	if payload["content"] != "body text" {
		t.Fatalf("content=%v", payload["content"])
	}
	if payload["channel"] != "app" {
		t.Fatalf("channel=%v", payload["channel"])
	}
	if payload["topic_code"] != "mytopic" {
		t.Fatalf("topic_code=%v", payload["topic_code"])
	}
	if !strings.Contains(spec.Body, `"apikey"`) {
		t.Fatalf("body missing apikey: %s", spec.Body)
	}
}

func withMockTransport(t *testing.T, roundTrip func(*http.Request) (*http.Response, error)) func() {
	t.Helper()
	previous := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(roundTrip)
	return func() { http.DefaultTransport = previous }
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestWPushSendCode0DespiteNon2xx(t *testing.T) {
	restore := withMockTransport(t, func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusCreated,
			Status:     "201 Created",
			Body:       io.NopCloser(strings.NewReader(`{"code":0,"message":"ok"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	defer restore()

	parsed, err := ParseURL("wpush://WPUSHabc123")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	target, err := NewWPushTarget(parsed)
	if err != nil {
		t.Fatalf("target: %v", err)
	}
	if err := target.Send("body", "title", NotifyInfo); err != nil {
		t.Fatalf("expected success on non-2xx with code 0: %v", err)
	}
}

func TestWPushSendRejectsBooleanCode(t *testing.T) {
	// Regression: Python treats False == 0; Go must still fail on {"code":false}.
	restore := withMockTransport(t, func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"code":false,"message":"nope"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	defer restore()

	parsed, err := ParseURL("wpush://WPUSHabc123")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	target, err := NewWPushTarget(parsed)
	if err != nil {
		t.Fatalf("target: %v", err)
	}
	if err := target.Send("body", "title", NotifyInfo); err == nil {
		t.Fatal("expected failure for boolean code")
	}
}

func TestWPushSendAcceptsExplicitCode0(t *testing.T) {
	restore := withMockTransport(t, func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"code":0,"message":"ok"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	defer restore()

	parsed, err := ParseURL("wpush://WPUSHabc123")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	target, err := NewWPushTarget(parsed)
	if err != nil {
		t.Fatalf("target: %v", err)
	}
	if err := target.Send("body", "title", NotifyInfo); err != nil {
		t.Fatalf("expected success for explicit code 0: %v", err)
	}
}

func TestWPushSendRejectsMissingCode(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"message":"rejected"}`,
		`{"code":null,"message":"null code"}`,
		`null`,
	} {
		body := body
		t.Run(body, func(t *testing.T) {
			restore := withMockTransport(t, func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
					Request:    req,
				}, nil
			})
			defer restore()

			parsed, err := ParseURL("wpush://WPUSHabc123")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			target, err := NewWPushTarget(parsed)
			if err != nil {
				t.Fatalf("target: %v", err)
			}
			if err := target.Send("body", "title", NotifyInfo); err == nil {
				t.Fatalf("expected failure for body %s", body)
			}
		})
	}
}

func TestWPushSendRejectsNonZeroCode(t *testing.T) {
	restore := withMockTransport(t, func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"code":401,"message":"bad key"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	defer restore()

	parsed, err := ParseURL("wpush://WPUSHabc123")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	target, err := NewWPushTarget(parsed)
	if err != nil {
		t.Fatalf("target: %v", err)
	}
	if err := target.Send("body", "title", NotifyInfo); err == nil {
		t.Fatal("expected failure for nonzero code")
	}
}
