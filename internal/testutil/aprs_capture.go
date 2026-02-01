package testutil

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
)

type APRSCapture struct {
	addr     string
	listener net.Listener
	mu       sync.Mutex
	messages []string
}

func StartAPRSCapture(t *testing.T) *APRSCapture {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("aprs listen failed: %v", err)
	}

	capture := &APRSCapture{
		addr:     listener.Addr().String(),
		listener: listener,
	}

	go capture.acceptLoop()
	return capture
}

func (a *APRSCapture) Addr() string {
	return a.addr
}

func (a *APRSCapture) Reset() {
	a.mu.Lock()
	a.messages = nil
	a.mu.Unlock()
}

func (a *APRSCapture) Messages() []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	out := make([]string, len(a.messages))
	copy(out, a.messages)
	return out
}

func (a *APRSCapture) Close() error {
	if a.listener == nil {
		return nil
	}
	return a.listener.Close()
}

func (a *APRSCapture) acceptLoop() {
	for {
		conn, err := a.listener.Accept()
		if err != nil {
			return
		}
		go a.handleConn(conn)
	}
}

func (a *APRSCapture) handleConn(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	loginLine, err := reader.ReadString('\n')
	if err != nil {
		return
	}

	user := parseAprsLoginUser(loginLine)
	response := fmt.Sprintf("# aprsc 2.1.0\r\n# logresp %s verified, server test\r\n", user)
	_, _ = conn.Write([]byte(response))

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		a.mu.Lock()
		a.messages = append(a.messages, line)
		a.mu.Unlock()
	}
}

func parseAprsLoginUser(loginLine string) string {
	fields := strings.Fields(loginLine)
	if len(fields) >= 2 && strings.EqualFold(fields[0], "user") {
		return strings.ToUpper(fields[1])
	}
	return "UNKNOWN"
}
