package parity

import (
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

type xmppSendResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

// TestXMPPParity compares the stanzas the port sends against upstream's, by
// running both against the same server.
//
// XMPP was the only provider with nothing checking its wire format. It is a
// stateful XML stream over a raw socket, so the HTTP capture harness cannot
// see it, and it sits in non_http_schemas.go for that reason — which meant
// "the arguments parse correctly" was the whole of its coverage.
//
// Three differences turned up as soon as there was something to compare
// against. Two were in how the connection is set up rather than in a stanza,
// and are pinned by xmpp_mode_test.go. The third is the newline between a
// folded title and the body, which this test caught directly.
//
// Not covered: a multi-user chat target. Upstream joins the room and waits for
// the join to be confirmed before sending, which the capture server does not
// implement, so a groupchat case fails on upstream's side before any stanza is
// sent. Direct messages are what these cases exercise.
func TestXMPPParity(t *testing.T) {
	cases := []struct {
		name   string
		query  string
		target string
		title  string
		body   string
	}{
		{
			name: "body and title",
			body: "apprise parity body",
			// Without ?subject= the title is folded into the body rather
			// than becoming a subject.
			title: "apprise parity title",
		},
		{
			name:  "subject carries the title",
			query: "&subject=yes",
			body:  "apprise parity body",
			title: "apprise parity title",
		},
		{
			name: "body only",
			body: "just a body",
		},
		{
			// A # marks a multi-user chat, which changes the stanza type and
			// makes the client join the room before it sends.
			name:   "groupchat target",
			target: "%23room@conference.localhost",
			body:   "room body",
			title:  "room title",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capture := testutil.StartXMPPCapture(t, "localhost")

			_, port, err := net.SplitHostPort(capture.Addr())
			if err != nil {
				t.Fatalf("split capture address: %v", err)
			}

			// starttls rather than none: neither client will authenticate
			// over a socket in the clear. verify=no because the capture
			// server mints its certificate per run.
			recipient := tc.target
			if recipient == "" {
				recipient = "target@localhost"
			}

			url := fmt.Sprintf(
				"xmpp://user:pass@localhost:%s/%s?mode=starttls&verify=no%s",
				port, recipient, tc.query)

			t.Setenv("PYTHONPATH", testutil.AppriseSourceRoot(t))
			requireUpstreamXMPP(t)
			script := filepath.Join(testutil.RepoRoot(t),
				"internal", "testutil", "scripts", "capture_xmpp.py")
			stdout, stderr, err := testutil.RunPythonScript(t, script,
				"--url", url, "--body", tc.body, "--title", tc.title)
			if err != nil {
				t.Fatalf("python xmpp send failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
			}

			var result xmppSendResult
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatalf("parse python result: %v (stdout: %s)", err, stdout)
			}
			if !result.Success {
				t.Fatalf("python xmpp send reported failure: %s\nstderr: %s",
					strings.TrimSpace(stdout), strings.TrimSpace(stderr))
			}

			pythonMessages := capture.WaitForMessages(t, 1, 15*time.Second)
			capture.Reset()

			parsed, err := notify.ParseURL(url)
			if err != nil {
				t.Fatalf("parse xmpp url: %v", err)
			}
			target, err := notify.NewXMPPTarget(parsed)
			if err != nil {
				t.Fatalf("build xmpp target: %v", err)
			}
			if err := target.Send(tc.body, tc.title, notify.NotifyInfo); err != nil {
				t.Fatalf("go xmpp send failed: %v", err)
			}

			goMessages := capture.WaitForMessages(t, 1, 15*time.Second)

			if len(pythonMessages) != len(goMessages) {
				t.Fatalf("stanza count mismatch: python=%d go=%d",
					len(pythonMessages), len(goMessages))
			}

			for i := range pythonMessages {
				python, goMsg := pythonMessages[i], goMessages[i]
				if python != goMsg {
					t.Fatalf("stanza %d mismatch:\n"+
						"python: to=%q type=%q subject=%q body=%q\n"+
						"go:     to=%q type=%q subject=%q body=%q",
						i,
						python.To, python.Type, python.Subject, python.Body,
						goMsg.To, goMsg.Type, goMsg.Subject, goMsg.Body)
				}
			}

			// A matching pair of empty stanzas would also compare equal.
			if goMessages[0].Body == "" {
				t.Fatal("the captured stanza carries no body")
			}
		})
	}
}

// requireUpstreamXMPP skips when upstream cannot load its XMPP plugin. The
// dependency ships in apprise[all-plugins], so this is about reporting the
// difference between "not installed" and "does not agree" rather than letting
// a missing package look like a parity failure.
func requireUpstreamXMPP(t *testing.T) {
	t.Helper()

	script := filepath.Join(testutil.RepoRoot(t),
		"internal", "testutil", "scripts", "capture_xmpp.py")
	stdout, stderr, err := testutil.RunPythonScript(t, script, "--check")
	if err != nil {
		t.Skipf("upstream xmpp plugin unavailable: %v (stderr: %s)",
			err, strings.TrimSpace(stderr))
	}

	var result xmppSendResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil || !result.Success {
		t.Skipf("upstream xmpp plugin unavailable: %s", strings.TrimSpace(stdout))
	}
}
