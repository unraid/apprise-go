package notify

import (
	"net/url"
	"testing"
)

func TestPushoverFormat(t *testing.T) {
	build := func(raw, body string) url.Values {
		t.Helper()
		parsed, err := ParseURL(raw)
		if err != nil {
			t.Fatalf("ParseURL(%q): %v", raw, err)
		}
		tgt, err := NewPushoverTarget(parsed)
		if err != nil {
			t.Fatalf("NewPushoverTarget(%q): %v", raw, err)
		}
		spec, err := tgt.BuildRequest(body, "Title", NotifyInfo)
		if err != nil {
			t.Fatalf("BuildRequest(%q): %v", raw, err)
		}
		vals, err := url.ParseQuery(spec.Body)
		if err != nil {
			t.Fatalf("parse body: %v", err)
		}
		return vals
	}

	htmlVals := build("pover://user@token?format=html", "<b>hi</b>")
	if got := htmlVals.Get("html"); got != "1" {
		t.Errorf("format=html: html=%q, want 1", got)
	}
	if got := htmlVals.Get("message"); got != "<b>hi</b>" {
		t.Errorf("format=html: message=%q, want unchanged", got)
	}

	mdVals := build("pover://user@token?format=markdown", "**hi**")
	if got := mdVals.Get("html"); got != "1" {
		t.Errorf("format=markdown: html=%q, want 1", got)
	}
	if got, want := mdVals.Get("message"), markdownToHTML("**hi**"); got != want {
		t.Errorf("format=markdown: message=%q, want %q", got, want)
	}

	for _, raw := range []string{
		"pover://user@token",
		"pover://user@token?format=text",
	} {
		if v := build(raw, "<b>hi</b>"); v.Has("html") {
			t.Errorf("%s: html should be absent, got %q", raw, v.Get("html"))
		}
	}
}
