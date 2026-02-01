package testutil

import (
	"net"
	"sync"
	"testing"
	"time"
)

type UDPPacket struct {
	Addr string
	Data string
}

type UDPCapture struct {
	addr    string
	conn    *net.UDPConn
	mu      sync.Mutex
	packets []UDPPacket
	notify  chan struct{}
}

func StartUDPCapture(t *testing.T) *UDPCapture {
	t.Helper()

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("udp resolve failed: %v", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("udp listen failed: %v", err)
	}

	capture := &UDPCapture{
		addr:   conn.LocalAddr().String(),
		conn:   conn,
		notify: make(chan struct{}, 1),
	}
	go capture.readLoop()
	return capture
}

func (u *UDPCapture) Addr() string {
	return u.addr
}

func (u *UDPCapture) Reset() {
	u.mu.Lock()
	u.packets = nil
	u.mu.Unlock()
	u.drainNotify()
}

func (u *UDPCapture) Packets() []UDPPacket {
	u.mu.Lock()
	defer u.mu.Unlock()

	out := make([]UDPPacket, len(u.packets))
	copy(out, u.packets)
	return out
}

func (u *UDPCapture) WaitForPackets(count int, timeout time.Duration) bool {
	if count <= 0 {
		return true
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		if u.packetCount() >= count {
			return true
		}

		select {
		case <-u.notify:
			continue
		case <-deadline.C:
			return u.packetCount() >= count
		}
	}
}

func (u *UDPCapture) Close() error {
	if u.conn == nil {
		return nil
	}
	return u.conn.Close()
}

func (u *UDPCapture) readLoop() {
	buf := make([]byte, 65535)
	for {
		n, addr, err := u.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		payload := make([]byte, n)
		copy(payload, buf[:n])

		u.mu.Lock()
		u.packets = append(u.packets, UDPPacket{
			Addr: addr.String(),
			Data: string(payload),
		})
		u.mu.Unlock()

		select {
		case u.notify <- struct{}{}:
		default:
		}
	}
}

func (u *UDPCapture) packetCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.packets)
}

func (u *UDPCapture) drainNotify() {
	for {
		select {
		case <-u.notify:
			continue
		default:
			return
		}
	}
}
