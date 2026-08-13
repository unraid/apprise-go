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
