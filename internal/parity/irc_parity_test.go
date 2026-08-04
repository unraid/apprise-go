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

type ircSendResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

// TestIRCParity compares the command stream this port writes against
// upstream's, by running both against the same server.
//
// irc and ircs were declared unsupported for most of this port's life, on the
// reasoning that verifying them needed a stateful fake server. That was true
// when it was written and stopped being true once xmpp_capture.go existed —
// the same shape of harness serves both.
//
// What is compared is the sequence of commands, not the socket: registration,
// the join handshake, and the messages. Nick collision has a case of its own
// because it is the one branch a happy-path fixture would never reach.
func TestIRCParity(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		query  string
		title  string
		body   string
		refuse string
		expect []string
	}{
		{
			name:  "channel",
			path:  "%23apprise",
			body:  "irc body",
			title: "irc title",
			// IRC is not a command either side meant to send: the title is
			// folded into the body with a CRLF, which ends the PRIVMSG line,
			// so the server reads "irc body" as a fresh command whose first
			// word is IRC. Both implementations do it, and the fixture
			// records it rather than hiding it.
			expect: []string{"NICK", "USER", "JOIN", "PRIVMSG", "IRC", "QUIT"},
		},
		{
			name:   "user target",
			path:   "someone",
			body:   "direct body",
			expect: []string{"NICK", "USER", "PRIVMSG", "QUIT"},
		},
		{
			name:   "several channels",
			path:   "%23one/%23two",
			body:   "multi body",
			expect: []string{"NICK", "USER", "JOIN", "PRIVMSG", "JOIN", "PRIVMSG", "QUIT"},
		},
		{
			// ?join=no messages the channel without entering it first.
			name:   "join disabled",
			path:   "%23apprise",
			query:  "&join=no",
			body:   "no join body",
			expect: []string{"NICK", "USER", "PRIVMSG", "QUIT"},
		},
		{
			// A taken nick has to be retried under another name, which a
			// happy-path fixture never exercises.
			name:   "nick collision",
			path:   "%23apprise",
			body:   "collision body",
			refuse: "apprise",
			// The retry lands after USER because registration sends both
			// before it waits for a reply.
			expect: []string{"NICK", "USER", "NICK", "JOIN", "PRIVMSG", "QUIT"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capture := testutil.StartIRCCapture(t)
			if tc.refuse != "" {
				capture.RefuseNick(tc.refuse)
			}

			host, port, err := net.SplitHostPort(capture.Addr())
			if err != nil {
				t.Fatalf("split capture address: %v", err)
			}

			// The nick is pinned so both sides register under the same name;
			// left to itself each generates its own.
			url := fmt.Sprintf("irc://apprise:secret@%s:%s/%s?nick=apprise%s",
				host, port, tc.path, tc.query)

			t.Setenv("PYTHONPATH", testutil.AppriseSourceRoot(t))
			requireUpstreamIRC(t)

			script := filepath.Join(testutil.RepoRoot(t),
				"internal", "testutil", "scripts", "capture_irc.py")
			stdout, stderr, err := testutil.RunPythonScript(t, script,
				"--url", url, "--body", tc.body, "--title", tc.title)
			if err != nil {
				t.Fatalf("python irc send failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
			}

			var result ircSendResult
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatalf("parse python result: %v (stdout: %s)", err, stdout)
			}
			if !result.Success {
				t.Fatalf("python irc send reported failure: %s\nstderr: %s",
					strings.TrimSpace(stdout), strings.TrimSpace(stderr))
			}

			capture.WaitForCommand(t, "QUIT", 1, 20*time.Second)
			pythonLines := capture.Lines()
			capture.Reset()
			if tc.refuse != "" {
				capture.RefuseNick(tc.refuse)
			}

			parsed, err := notify.ParseURL(url)
			if err != nil {
				t.Fatalf("parse irc url: %v", err)
			}
			target, err := notify.NewIRCTarget(parsed)
			if err != nil {
				t.Fatalf("build irc target: %v", err)
			}
			if err := target.Send(tc.body, tc.title, notify.NotifyInfo); err != nil {
				t.Fatalf("go irc send failed: %v", err)
			}

			capture.WaitForCommand(t, "QUIT", 1, 20*time.Second)
			goLines := capture.Lines()

			assertIRCCommandsEqual(t, pythonLines, goLines)

			// Pin the expected shape too, so both sides degrading the same
			// way cannot pass as agreement.
			if got := ircCommands(goLines); !equalStrings(got, tc.expect) {
				t.Fatalf("command sequence:\n got %v\nwant %v", got, tc.expect)
			}
		})
	}
}

// assertIRCCommandsEqual compares the two command streams line by line.
//
// PASS carries the password and USER carries the real name, both of which the
// two sides may spell differently without it meaning anything; what has to
// agree is the sequence of commands and the messages actually delivered.
func assertIRCCommandsEqual(t *testing.T, python, goLines []testutil.IRCLine) {
	t.Helper()

	pythonCommands := ircCommands(python)
	goCommands := ircCommands(goLines)

	if !equalStrings(pythonCommands, goCommands) {
		t.Fatalf("command sequence mismatch:\npython %v\ngo     %v\n\npython lines:\n  %s\n\ngo lines:\n  %s",
			pythonCommands, goCommands,
			strings.Join(ircRaw(python), "\n  "), strings.Join(ircRaw(goLines), "\n  "))
	}

	// The messages are the payload; everything else is ceremony around them.
	pythonMessages := ircMessages(python)
	goMessages := ircMessages(goLines)
	if !equalStrings(pythonMessages, goMessages) {
		t.Fatalf("delivered messages mismatch:\npython %q\ngo     %q",
			pythonMessages, goMessages)
	}

	if len(goMessages) == 0 {
		t.Fatal("no message was delivered; the comparison would pass on two empty streams")
	}
}

func ircCommands(lines []testutil.IRCLine) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		// PASS is registration detail rather than part of the shape: whether
		// it is sent at all is covered by its own test.
		if line.Command == "PASS" {
			continue
		}
		out = append(out, line.Command)
	}

	return out
}

// ircMessages renders each PRIVMSG as "target :text".
func ircMessages(lines []testutil.IRCLine) []string {
	out := []string{}
	for _, line := range lines {
		if line.Command != "PRIVMSG" {
			continue
		}
		target := ""
		if len(line.Params) > 0 {
			target = line.Params[0]
		}
		out = append(out, target+" :"+line.Trailing)
	}

	return out
}

func ircRaw(lines []testutil.IRCLine) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, line.Raw())
	}

	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// requireUpstreamIRC skips when upstream cannot load its IRC plugin.
func requireUpstreamIRC(t *testing.T) {
	t.Helper()

	script := filepath.Join(testutil.RepoRoot(t),
		"internal", "testutil", "scripts", "capture_irc.py")
	stdout, stderr, err := testutil.RunPythonScript(t, script, "--check")
	if err != nil {
		t.Skipf("upstream irc plugin unavailable: %v (stderr: %s)",
			err, strings.TrimSpace(stderr))
	}

	var result ircSendResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil || !result.Success {
		t.Skipf("upstream irc plugin unavailable: %s", strings.TrimSpace(stdout))
	}
}
