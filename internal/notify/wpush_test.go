package notify

import (
	"encoding/json"
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
