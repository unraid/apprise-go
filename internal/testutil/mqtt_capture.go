package testutil

import (
	"crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

type MQTTMessage struct {
	Topic   string
	Payload string
	QoS     int
	Retain  bool
}

type MQTTCapture struct {
	addr     string
	listener net.Listener
	mu       sync.Mutex
	messages []MQTTMessage
}

func StartMQTTCapture(t *testing.T) *MQTTCapture {
	t.Helper()

	return startMQTTCapture(t, false)
}

func StartMQTTSCapture(t *testing.T) *MQTTCapture {
	t.Helper()

	return startMQTTCapture(t, true)
}

func startMQTTCapture(t *testing.T, secure bool) *MQTTCapture {
	t.Helper()

	var (
		listener net.Listener
		err      error
	)

	if secure {
		listener, err = tls.Listen("tcp", "127.0.0.1:0", TestTLSConfig(t))
	} else {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatalf("mqtt listen failed: %v", err)
	}

	capture := &MQTTCapture{
		addr:     listener.Addr().String(),
		listener: listener,
	}

	go capture.acceptLoop()
	return capture
}

func (m *MQTTCapture) Addr() string {
	return m.addr
}

func (m *MQTTCapture) Messages() []MQTTMessage {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]MQTTMessage, len(m.messages))
	copy(out, m.messages)
	return out
}

func (m *MQTTCapture) Reset() {
	m.mu.Lock()
	m.messages = nil
	m.mu.Unlock()
}

func (m *MQTTCapture) Close() error {
	if m.listener == nil {
		return nil
	}
	return m.listener.Close()
}

func (m *MQTTCapture) acceptLoop() {
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			return
		}
		go m.handleConn(conn)
	}
}

func (m *MQTTCapture) handleConn(conn net.Conn) {
	defer conn.Close()

	for {
		packetType, flags, payload, err := readMQTTPacket(conn)
		if err != nil {
			return
		}

		switch packetType {
		case 0x10: // CONNECT
			protocolLevel, err := parseMQTTConnectProtocol(payload)
			if err != nil {
				return
			}
			_ = protocolLevel
			writeMQTTConnAck(conn, protocolLevel == 5)
		case 0x30: // PUBLISH
			message, packetID, qos, err := parseMQTTPublish(flags, payload)
			if err != nil {
				return
			}
			m.mu.Lock()
			m.messages = append(m.messages, message)
			m.mu.Unlock()

			if qos == 1 {
				writeMQTTPubAck(conn, packetID)
			} else if qos == 2 {
				writeMQTTPubRec(conn, packetID)
			}
		case 0x60: // PUBREL
			packetID, err := parseMQTTPacketID(payload)
			if err != nil {
				return
			}
			writeMQTTPubComp(conn, packetID)
		case 0xC0: // PINGREQ
			_, _ = conn.Write([]byte{0xD0, 0x00})
		case 0xE0: // DISCONNECT
			return
		default:
			return
		}
	}
}

func readMQTTPacket(conn net.Conn) (byte, byte, []byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header[:1]); err != nil {
		return 0, 0, nil, err
	}

	remaining, err := decodeMQTTRemaining(conn)
	if err != nil {
		return 0, 0, nil, err
	}
	payload := make([]byte, remaining)
	if remaining > 0 {
		if _, err := io.ReadFull(conn, payload); err != nil {
			return 0, 0, nil, err
		}
	}
	return header[0] & 0xF0, header[0] & 0x0F, payload, nil
}

func decodeMQTTRemaining(conn net.Conn) (int, error) {
	multiplier := 1
	value := 0
	for i := 0; i < 4; i++ {
		buf := make([]byte, 1)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return 0, err
		}
		digit := int(buf[0])
		value += (digit & 127) * multiplier
		if digit&128 == 0 {
			return value, nil
		}
		multiplier *= 128
	}
	return 0, errors.New("invalid remaining length")
}

func parseMQTTConnectProtocol(payload []byte) (int, error) {
	if len(payload) < 4 {
		return 0, errors.New("invalid connect payload")
	}
	length := int(binary.BigEndian.Uint16(payload[0:2]))
	if len(payload) < 2+length+1 {
		return 0, errors.New("invalid connect payload")
	}
	offset := 2 + length
	return int(payload[offset]), nil
}

func writeMQTTConnAck(conn net.Conn, v5 bool) {
	if v5 {
		_, _ = conn.Write([]byte{0x20, 0x03, 0x00, 0x00, 0x00})
		return
	}
	_, _ = conn.Write([]byte{0x20, 0x02, 0x00, 0x00})
}

func parseMQTTPublish(flags byte, payload []byte) (MQTTMessage, uint16, int, error) {
	if len(payload) < 2 {
		return MQTTMessage{}, 0, 0, errors.New("invalid publish")
	}
	topicLen := int(binary.BigEndian.Uint16(payload[0:2]))
	if len(payload) < 2+topicLen {
		return MQTTMessage{}, 0, 0, errors.New("invalid publish")
	}
	topic := string(payload[2 : 2+topicLen])
	offset := 2 + topicLen
	qos := int((flags >> 1) & 0x03)
	var packetID uint16
	if qos > 0 {
		if len(payload) < offset+2 {
			return MQTTMessage{}, 0, 0, errors.New("invalid publish")
		}
		packetID = binary.BigEndian.Uint16(payload[offset : offset+2])
		offset += 2
	}
	if qos >= 0 && len(payload) > offset {
		if payload[offset] == 0x00 {
			offset++
		}
	}
	message := MQTTMessage{
		Topic:   topic,
		Payload: string(payload[offset:]),
		QoS:     qos,
		Retain:  flags&0x01 == 0x01,
	}
	return message, packetID, qos, nil
}

func parseMQTTPacketID(payload []byte) (uint16, error) {
	if len(payload) < 2 {
		return 0, errors.New("invalid packet id")
	}
	return binary.BigEndian.Uint16(payload[0:2]), nil
}

func writeMQTTPubAck(conn net.Conn, packetID uint16) {
	buf := []byte{0x40, 0x02, byte(packetID >> 8), byte(packetID)}
	_, _ = conn.Write(buf)
}

func writeMQTTPubRec(conn net.Conn, packetID uint16) {
	buf := []byte{0x50, 0x02, byte(packetID >> 8), byte(packetID)}
	_, _ = conn.Write(buf)
}

func writeMQTTPubComp(conn net.Conn, packetID uint16) {
	buf := []byte{0x70, 0x02, byte(packetID >> 8), byte(packetID)}
	_, _ = conn.Write(buf)
}

func (m *MQTTCapture) WaitForMessages(count int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if len(m.Messages()) >= count {
			return true
		}
		if time.Now().After(deadline) {
			return len(m.Messages()) >= count
		}
		time.Sleep(10 * time.Millisecond)
	}
}
