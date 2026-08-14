package notify_test

import (
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

// TestMatrixAccessTokenTransactionIDIsUnique guards the Matrix
// /send/m.room.message transaction id on the bare access-token path.
//
// That endpoint is idempotent and keyed on the transaction id: a homeserver
// that has already seen an id replies 200 with the original event and creates
// nothing new. So reusing one id makes every send after the first vanish
// without an error anywhere — the notification simply never arrives.
func TestMatrixAccessTokenTransactionIDIsUnique(t *testing.T) {
	t.Setenv("APPRISE_FIXED_TIME", "")
	notify.ConfigureStorage("", 8, nil)
	t.Cleanup(func() { notify.ConfigureStorage("", 8, nil) })

	const url = "matrixs://tokenabc123@matrix.example.com/%23room:example.com?e2ee=no"

	send := func() string {
		t.Helper()
		specs := testutil.CaptureGoRequests(t, func() error {
			return notify.SendTargetURL(url, "body", "title", "", notify.NotifyInfo)
		})
		for _, spec := range specs {
			if idx := strings.Index(spec.URL, "/send/m.room.message/"); idx >= 0 {
				return spec.URL[idx+len("/send/m.room.message/"):]
			}
		}
		t.Fatal("no m.room.message send was issued")
		return ""
	}

	first := send()
	second := send()

	if first == "" || second == "" {
		t.Fatal("transaction id missing from send path")
	}
	if first == second {
		t.Fatalf("transaction id reused across sends (%q); the homeserver will "+
			"treat the second send as a retransmission and drop it", first)
	}
	if first == "00000000-0000-4000-8000-000000000000" {
		t.Fatalf("transaction id is the deterministic placeholder %q rather than a generated value", first)
	}
}

// sendTransactionIDs returns the transaction id of every m.room.message send
// issued by one notification, in order.
func sendTransactionIDs(t *testing.T, specs []notify.RequestSpec) []string {
	t.Helper()

	const marker = "/send/m.room.message/"
	ids := []string{}
	for _, spec := range specs {
		if idx := strings.Index(spec.URL, marker); idx >= 0 {
			ids = append(ids, spec.URL[idx+len(marker):])
		}
	}
	return ids
}

// assertUniqueTransactionIDs fails when a notification reuses an id, naming
// the collision rather than only reporting that one happened.
func assertUniqueTransactionIDs(t *testing.T, ids []string, want int) {
	t.Helper()

	if len(ids) != want {
		t.Fatalf("expected %d m.room.message sends, got %d", want, len(ids))
	}

	seen := map[string]int{}
	for i, id := range ids {
		if first, ok := seen[id]; ok {
			t.Fatalf("sends %d and %d share transaction id %q; a homeserver is "+
				"entitled to treat the later one as a retransmission and drop it",
				first, i, id)
		}
		seen[id] = i
	}
}

// TestMatrixMultiRoomTransactionIDsAreUnique covers a notification addressed
// to more than one room.
//
// Synapse tolerates a repeat here, because it keys idempotency on the request
// path and that carries the room id. The spec is the stricter of the two --
// it asks for an id unique across requests sharing an access token -- so this
// pins the behavior a homeserver keying on the token alone would need.
func TestMatrixMultiRoomTransactionIDsAreUnique(t *testing.T) {
	t.Setenv("APPRISE_FIXED_TIME", "")
	notify.ConfigureStorage("", 8, nil)
	t.Cleanup(func() { notify.ConfigureStorage("", 8, nil) })

	specs := testutil.CaptureGoRequests(t, func() error {
		return notify.SendTargetURL(
			"matrixs://tokenabc123@matrix.example.com/%23room1:example.com/%23room2:example.com?e2ee=no",
			"body", "title", "", notify.NotifyInfo)
	})

	assertUniqueTransactionIDs(t, sendTransactionIDs(t, specs), 2)
}

// TestMatrixAttachmentTransactionIDsAreUnique covers the several events a
// single room receives when files are attached: one per file, then the text.
//
// This is the case that fails against a real homeserver: same room, so same
// request path, so a repeated id is a retransmission. It costs the message
// body itself, since the text is sent last and is what gets discarded.
func TestMatrixAttachmentTransactionIDsAreUnique(t *testing.T) {
	t.Setenv("APPRISE_FIXED_TIME", "")
	notify.ConfigureStorage("", 8, nil)
	t.Cleanup(func() { notify.ConfigureStorage("", 8, nil) })

	specs := testutil.CaptureGoRequests(t, func() error {
		target, err := notify.ParseURL("matrixs://tokenabc123@matrix.example.com/%23room1:example.com?e2ee=no")
		if err != nil {
			return err
		}
		sender, err := notify.NewTarget(target)
		if err != nil {
			return err
		}

		return notify.DispatchSend(sender, "body", "title", notify.NotifyInfo,
			[]notify.Attachment{
				{Name: "one.txt", MIMEType: "text/plain", Data: []byte("first")},
				{Name: "two.txt", MIMEType: "text/plain", Data: []byte("second")},
			})
	})

	// Two attachment events plus the text message.
	assertUniqueTransactionIDs(t, sendTransactionIDs(t, specs), 3)
}
