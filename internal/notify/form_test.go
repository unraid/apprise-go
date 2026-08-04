package notify

import "testing"

func TestParseFormAttachAsRejectsMultipleWildcards(t *testing.T) {
	_, _, err := parseFormAttachAs("file.$")
	if err == nil {
		t.Fatalf("expected multi-wildcard attach-as to fail")
	}
}

func TestParseFormAttachAsExpandsSingleWildcard(t *testing.T) {
	attachAs, multi, err := parseFormAttachAs("file*")
	if err != nil {
		t.Fatalf("parse attach-as: %v", err)
	}
	if attachAs != "file%02d" || !multi {
		t.Fatalf("attachAs=%q multi=%v, want file%%02d true", attachAs, multi)
	}
}

func TestParseFormAttachAsAllowsExplicitPlaceholder(t *testing.T) {
	attachAs, multi, err := parseFormAttachAs("upload-%02d.txt")
	if err != nil {
		t.Fatalf("parse attach-as: %v", err)
	}
	if attachAs != "upload-%02d.txt" || !multi {
		t.Fatalf("attachAs=%q multi=%v, want upload-%%02d.txt true", attachAs, multi)
	}
}

func TestNewFormTargetRejectsInvalidAttachAs(t *testing.T) {
	parsed, err := ParseURL("form://example.com/notify?attach-as=file.$")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if _, err := NewFormTarget(parsed); err == nil {
		t.Fatalf("expected invalid attach-as to fail")
	}
}
