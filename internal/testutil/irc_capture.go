package testutil

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// IRCLine is one command a client sent, split into its parts.
type IRCLine struct {
	Command  string
	Params   []string
	Trailing string
}

// Raw renders the line back the way it arrived, which is what a failure
// message wants to show.
func (l IRCLine) Raw() string {
	out := l.Command
	if len(l.Params) > 0 {
		out += " " + strings.Join(l.Params, " ")
	}
	if l.Trailing != "" {
		out += " :" + l.Trailing
	}

	return out
}

// IRCCapture is an IRC server that completes registration and records what a
// client sends, so the two implementations can be run against the same
// endpoint and compared.
//
// IRC is a stateful line protocol over a raw socket, which the HTTP capture
// harness cannot see. Both implementations write the same command stream, so
// the recorded lines are what they are compared on.
type IRCCapture struct {
	listener net.Listener

	mu          sync.Mutex
	lines       []IRCLine
	connections int

	// nickTaken makes the server refuse the first nick a client asks for, so
	// the collision path can be exercised.
	nickTaken string
}

func StartIRCCapture(t *testing.T) *IRCCapture {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for irc capture: %v", err)
	}

	capture := &IRCCapture{listener: listener}
	go capture.serve()

	t.Cleanup(func() {
		_ = capture.Close()
	})

	return capture
}

func (c *IRCCapture) Addr() string {
	return c.listener.Addr().String()
}

// RefuseNick makes the server answer the first NICK for this name with 433,
// which is how a real server reports a collision.
func (c *IRCCapture) RefuseNick(nick string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.nickTaken = nick
}

func (c *IRCCapture) Lines() []IRCLine {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]IRCLine(nil), c.lines...)
}

func (c *IRCCapture) Connections() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.connections
}

func (c *IRCCapture) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lines = nil
	c.connections = 0
}

func (c *IRCCapture) Close() error {
	return c.listener.Close()
}

// WaitForCommand blocks until the named command has been seen at least count
// times, so a test does not race a client that is still writing.
func (c *IRCCapture) WaitForCommand(t *testing.T, command string, count int, timeout time.Duration) []IRCLine {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		lines := c.Lines()
		seen := 0
		for _, line := range lines {
			if strings.EqualFold(line.Command, command) {
				seen++
			}
		}
		if seen >= count {
			return lines
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected %d %s lines, saw %d:\n  %s",
				count, command, seen, strings.Join(rawLines(lines), "\n  "))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func rawLines(lines []IRCLine) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, line.Raw())
	}

	return out
}

func (c *IRCCapture) serve() {
	for {
		conn, err := c.listener.Accept()
		if err != nil {
			return
		}

		c.mu.Lock()
		c.connections++
		c.mu.Unlock()

		go c.handle(conn)
	}
}

func (c *IRCCapture) record(line IRCLine) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lines = append(c.lines, line)
}

func (c *IRCCapture) handle(conn net.Conn) {
	defer func() {
		_ = conn.Close()
	}()

	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	reader := bufio.NewReader(conn)
	registered := false
	nick := ""
	refused := map[string]bool{}

	for {
		raw, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		line := parseIRCCaptureLine(strings.TrimRight(raw, "\r\n"))
		if line.Command == "" {
			continue
		}
		c.record(line)

		switch line.Command {
		case "NICK":
			candidate := line.param(0)
			if candidate == "" {
				candidate = line.Trailing
			}

			c.mu.Lock()
			taken := c.nickTaken
			c.mu.Unlock()

			if candidate == taken && !refused[candidate] {
				// 433 is nickname-in-use; a client is expected to pick
				// another and try again rather than give up.
				refused[candidate] = true
				if _, err := fmt.Fprintf(conn,
					":capture 433 * %s :Nickname is already in use\r\n",
					candidate); err != nil {
					return
				}

				continue
			}

			nick = candidate
			if !registered && nick != "" {
				registered = true
				if err := writeWelcome(conn, nick); err != nil {
					return
				}
			}

		case "USER":
			if !registered && nick != "" {
				registered = true
				if err := writeWelcome(conn, nick); err != nil {
					return
				}
			}

		case "PING":
			if _, err := fmt.Fprintf(conn, ":capture PONG capture :%s\r\n", line.Trailing); err != nil {
				return
			}

		case "JOIN":
			channel := line.param(0)
			if channel == "" {
				channel = line.Trailing
			}
			// The echo plus the end-of-names numeric is what tells a client
			// the join actually landed.
			if _, err := fmt.Fprintf(conn,
				":%s!apprise@capture JOIN :%s\r\n"+
					":capture 353 %s = %s :%s\r\n"+
					":capture 366 %s %s :End of /NAMES list.\r\n",
				nick, channel, nick, channel, nick, nick, channel); err != nil {
				return
			}

		case "QUIT":
			return
		}
	}
}

// writeWelcome sends the numerics a client waits for before it will consider
// itself registered.
func writeWelcome(conn net.Conn, nick string) error {
	_, err := fmt.Fprintf(conn,
		":capture 001 %s :Welcome to the capture network %s\r\n"+
			":capture 002 %s :Your host is capture\r\n"+
			":capture 003 %s :This server was created today\r\n"+
			":capture 004 %s capture 1.0 o o\r\n",
		nick, nick, nick, nick, nick)

	return err
}

func (l IRCLine) param(index int) string {
	if index < 0 || index >= len(l.Params) {
		return ""
	}

	return l.Params[index]
}

// parseIRCCaptureLine splits one line into command, params and trailing. A
// client's lines carry no prefix, so none is parsed.
func parseIRCCaptureLine(line string) IRCLine {
	parsed := IRCLine{}

	if head, tail, found := strings.Cut(line, " :"); found {
		parsed.Trailing = tail
		line = head
	}

	fields := strings.Fields(line)
	if len(fields) == 0 {
		return parsed
	}

	parsed.Command = strings.ToUpper(fields[0])
	parsed.Params = fields[1:]

	return parsed
}
