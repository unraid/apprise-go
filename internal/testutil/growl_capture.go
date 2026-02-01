package testutil

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

type GrowlMessage struct {
	Type    string
	Headers map[string]string
}

type GrowlCapture struct {
	addr     string
	listener net.Listener
	mu       sync.Mutex
	messages []GrowlMessage
}

func StartGrowlCapture(t *testing.T) *GrowlCapture {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("growl listen failed: %v", err)
	}

	capture := &GrowlCapture{
		addr:     listener.Addr().String(),
		listener: listener,
	}
	go capture.acceptLoop()
	return capture
}

func (g *GrowlCapture) Addr() string {
	return g.addr
}

func (g *GrowlCapture) Close() error {
	if g.listener == nil {
		return nil
	}
	return g.listener.Close()
}

func (g *GrowlCapture) Messages() []GrowlMessage {
	g.mu.Lock()
	defer g.mu.Unlock()

	out := make([]GrowlMessage, len(g.messages))
	copy(out, g.messages)
	return out
}

func (g *GrowlCapture) Reset() {
	g.mu.Lock()
	g.messages = nil
	g.mu.Unlock()
}

func (g *GrowlCapture) acceptLoop() {
	for {
		conn, err := g.listener.Accept()
		if err != nil {
			return
		}
		go g.handleConn(conn)
	}
}

func (g *GrowlCapture) handleConn(conn net.Conn) {
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	reader := bufio.NewReader(conn)
	var builder strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		builder.WriteString(line)
		if strings.HasSuffix(builder.String(), "\r\n\r\n") {
			// Keep reading; a second timeout will end collection.
			_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		}
	}

	message := parseGrowlMessage(builder.String())
	if message.Type != "" {
		g.mu.Lock()
		g.messages = append(g.messages, message)
		g.mu.Unlock()
	}

	_, _ = conn.Write([]byte("GNTP/1.0 -OK NONE\r\n\r\n"))
}

func parseGrowlMessage(raw string) GrowlMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return GrowlMessage{}
	}
	lines := strings.Split(raw, "\r\n")
	if len(lines) == 0 {
		return GrowlMessage{}
	}

	first := strings.TrimSpace(lines[0])
	msgType := ""
	if strings.HasPrefix(strings.ToUpper(first), "GNTP/") {
		parts := strings.Fields(first)
		if len(parts) >= 2 {
			msgType = parts[1]
		}
	}

	headers := map[string]string{}
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}

	return GrowlMessage{
		Type:    strings.ToUpper(msgType),
		Headers: headers,
	}
}
