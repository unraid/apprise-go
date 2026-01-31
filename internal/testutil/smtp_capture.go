package testutil

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"testing"
)

type SMTPMessage struct {
	MailFrom string
	RcptTo   []string
	Data     string
}

type SMTPCapture struct {
	addr     string
	listener net.Listener
	mu       sync.Mutex
	messages []SMTPMessage
}

func StartSMTPCapture(t *testing.T) *SMTPCapture {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("smtp listen failed: %v", err)
	}

	capture := &SMTPCapture{
		addr:     listener.Addr().String(),
		listener: listener,
	}

	go capture.acceptLoop()
	return capture
}

func (s *SMTPCapture) Addr() string {
	return s.addr
}

func (s *SMTPCapture) Reset() {
	s.mu.Lock()
	s.messages = nil
	s.mu.Unlock()
}

func (s *SMTPCapture) Messages() []SMTPMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]SMTPMessage, len(s.messages))
	for i, msg := range s.messages {
		rcpt := make([]string, len(msg.RcptTo))
		copy(rcpt, msg.RcptTo)
		out[i] = SMTPMessage{
			MailFrom: msg.MailFrom,
			RcptTo:   rcpt,
			Data:     msg.Data,
		}
	}
	return out
}

func (s *SMTPCapture) Close() error {
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}

func (s *SMTPCapture) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *SMTPCapture) handleConn(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	writeLine := func(line string) {
		_, _ = writer.WriteString(line + "\r\n")
		_ = writer.Flush()
	}

	writeLine("220 localhost ESMTP")

	mailFrom := ""
	rcptTo := []string{}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(upper, "EHLO") || strings.HasPrefix(upper, "HELO"):
			writeLine("250-localhost")
			writeLine("250 OK")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			mailFrom = strings.TrimSpace(line[len("MAIL FROM:"):])
			mailFrom = strings.Trim(mailFrom, "<>")
			writeLine("250 OK")
		case strings.HasPrefix(upper, "RCPT TO:"):
			addr := strings.TrimSpace(line[len("RCPT TO:"):])
			addr = strings.Trim(addr, "<>")
			rcptTo = append(rcptTo, addr)
			writeLine("250 OK")
		case strings.HasPrefix(upper, "DATA"):
			writeLine("354 End data with <CR><LF>.<CR><LF>")
			dataLines := []string{}
			for {
				raw, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				raw = strings.TrimRight(raw, "\r\n")
				if raw == "." {
					break
				}
				if strings.HasPrefix(raw, "..") {
					raw = raw[1:]
				}
				dataLines = append(dataLines, raw)
			}
			data := strings.Join(dataLines, "\r\n")
			s.mu.Lock()
			rcptCopy := make([]string, len(rcptTo))
			copy(rcptCopy, rcptTo)
			s.messages = append(s.messages, SMTPMessage{
				MailFrom: mailFrom,
				RcptTo:   rcptCopy,
				Data:     data,
			})
			s.mu.Unlock()
			mailFrom = ""
			rcptTo = nil
			writeLine("250 OK")
		case strings.HasPrefix(upper, "RSET"):
			mailFrom = ""
			rcptTo = nil
			writeLine("250 OK")
		case strings.HasPrefix(upper, "NOOP"):
			writeLine("250 OK")
		case strings.HasPrefix(upper, "QUIT"):
			writeLine("221 Bye")
			return
		case strings.HasPrefix(upper, "AUTH") || strings.HasPrefix(upper, "STARTTLS"):
			writeLine("502 Command not implemented")
		default:
			writeLine("250 OK")
		}
	}
}
