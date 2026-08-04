package parity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

// TestOffice365LargeAttachmentParity covers the path a file only reaches once
// it is too big to ride inline in the message: the send becomes a saved draft,
// each file is uploaded against it through its own upload session, and the
// draft is sent afterwards. Five requests where an ordinary send makes one.
//
// It compares against upstream live rather than against a stored golden. The
// threshold is 3MB, so pinning this as a golden would mean committing a file
// past that and a golden holding its bytes; the file is generated here and
// thrown away instead.
func TestOffice365LargeAttachmentParity(t *testing.T) {
	const url = "azure://sender@example.com/tenant123/client123/secret123/target@example.com"

	path := filepath.Join(t.TempDir(), "big.bin")
	// Just past the inline ceiling, and under the 5MB chunk size, so the
	// upload is a single chunk covering the whole file.
	if err := os.WriteFile(path, make([]byte, 4*1024*1024), 0o600); err != nil {
		t.Fatalf("write attachment: %v", err)
	}

	pythonSpecs := testutil.CapturePythonRequestsWithAttachments(
		t, url, "body", "title", notify.NotifyInfo, []string{path})

	parsed, err := notify.ParseURL(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	target, err := notify.NewTarget(parsed)
	if err != nil {
		t.Fatalf("build target: %v", err)
	}

	attachment, err := notify.ParseAttachment(path)
	if err != nil {
		t.Fatalf("load attachment: %v", err)
	}

	goSpecs := testutil.CaptureGoRequests(t, func() error {
		return notify.DispatchSend(
			target, "body", "title", notify.NotifyInfo, []notify.Attachment{attachment})
	})

	assertRequestSpecSequenceMatches(t, pythonSpecs, goSpecs)

	// The sequence above would still look right if the file never moved, so
	// name the request that carries it and check it carries all of it.
	upload := 0
	for _, spec := range goSpecs {
		if spec.Method != "PUT" {
			continue
		}
		upload++
		if len(spec.Body) != 4*1024*1024 {
			t.Fatalf("upload carried %d bytes of a %d byte file",
				len(spec.Body), 4*1024*1024)
		}
		if spec.Headers["Content-Range"] != "bytes 0-4194303/4194304" {
			t.Fatalf("unexpected content range: %q", spec.Headers["Content-Range"])
		}
	}
	if upload != 1 {
		t.Fatalf("expected one upload request, got %d", upload)
	}
}
