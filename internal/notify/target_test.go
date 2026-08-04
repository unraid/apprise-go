package notify

import (
	"errors"
	"testing"
)

func TestTargetBuildersCoverSupportedSchemas(t *testing.T) {
	for _, schema := range SupportedSchemas() {
		if _, ok := targetBuilders[schema]; !ok {
			t.Fatalf("missing target builder for supported schema %s", schema)
		}
	}
}

func TestDispatchSendRejectsUnsupportedAttachments(t *testing.T) {
	target := &recordingSender{}

	err := DispatchSend(target, "body", "title", NotifyInfo, []Attachment{{Name: "report.txt"}})
	if !errors.Is(err, ErrAttachmentsUnsupported) {
		t.Fatalf("error = %v, want ErrAttachmentsUnsupported", err)
	}
	if target.sent {
		t.Fatalf("target Send called despite unsupported attachments")
	}
}

func TestDispatchSendWithoutAttachmentsUsesSender(t *testing.T) {
	target := &recordingSender{}

	if err := DispatchSend(target, "body", "title", NotifyInfo, nil); err != nil {
		t.Fatalf("dispatch send: %v", err)
	}
	if !target.sent {
		t.Fatalf("target Send was not called")
	}
}

type recordingSender struct {
	sent bool
}

func (r *recordingSender) Send(body, title string, notifyType NotifyType) error {
	r.sent = true
	return nil
}
