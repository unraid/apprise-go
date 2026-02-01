package testutil

import (
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"testing"
)

const (
	smppBindTransmitter    = 0x00000002
	smppBindTransmitterRes = 0x80000002
	smppSubmitSM           = 0x00000004
	smppSubmitSMRes        = 0x80000004
	smppUnbind             = 0x00000006
	smppUnbindRes          = 0x80000006
	smppEnquireLink        = 0x00000015
	smppEnquireLinkRes     = 0x80000015
)

type SMPPMessage struct {
	Source      string
	Destination string
	Body        string
}

type SMPPCapture struct {
	addr     string
	listener net.Listener
	mu       sync.Mutex
	messages []SMPPMessage
}

func StartSMPPCapture(t *testing.T) *SMPPCapture {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("smpp listen failed: %v", err)
	}

	capture := &SMPPCapture{
		addr:     listener.Addr().String(),
		listener: listener,
	}
	go capture.acceptLoop()
	return capture
}

func (s *SMPPCapture) Addr() string {
	return s.addr
}

func (s *SMPPCapture) Close() error {
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}

func (s *SMPPCapture) Messages() []SMPPMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]SMPPMessage, len(s.messages))
	copy(out, s.messages)
	return out
}

func (s *SMPPCapture) Reset() {
	s.mu.Lock()
	s.messages = nil
	s.mu.Unlock()
}

func (s *SMPPCapture) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *SMPPCapture) handleConn(conn net.Conn) {
	defer conn.Close()

	for {
		cmdID, seq, payload, err := readSMPPPacket(conn)
		if err != nil {
			return
		}

		if os.Getenv("SMPP_CAPTURE_DEBUG") != "" {
			log.Printf("smpp recv cmd=0x%08x seq=%d len=%d", cmdID, seq, len(payload))
		}

		switch cmdID {
		case smppBindTransmitter:
			writeSMPPResponse(conn, smppBindTransmitterRes, seq, smppBindRespPayload())
		case smppSubmitSM:
			message := parseSubmitSM(payload)
			if message.Destination != "" {
				s.mu.Lock()
				s.messages = append(s.messages, message)
				s.mu.Unlock()
			}
			writeSMPPResponse(conn, smppSubmitSMRes, seq, []byte("1\x00"))
		case smppUnbind:
			writeSMPPResponse(conn, smppUnbindRes, seq, nil)
		case smppEnquireLink:
			writeSMPPResponse(conn, smppEnquireLinkRes, seq, nil)
		default:
			return
		}
	}
}

func readSMPPPacket(conn net.Conn) (uint32, uint32, []byte, error) {
	header := make([]byte, 16)
	if _, err := io.ReadFull(conn, header); err != nil {
		return 0, 0, nil, err
	}
	length := binary.BigEndian.Uint32(header[0:4])
	if length < 16 {
		return 0, 0, nil, io.ErrUnexpectedEOF
	}
	cmdID := binary.BigEndian.Uint32(header[4:8])
	seq := binary.BigEndian.Uint32(header[12:16])

	payload := make([]byte, length-16)
	if len(payload) > 0 {
		if _, err := io.ReadFull(conn, payload); err != nil {
			return 0, 0, nil, err
		}
	}
	return cmdID, seq, payload, nil
}

func writeSMPPResponse(conn net.Conn, cmdID uint32, seq uint32, payload []byte) {
	length := uint32(16 + len(payload))
	buf := make([]byte, 16)
	binary.BigEndian.PutUint32(buf[0:4], length)
	binary.BigEndian.PutUint32(buf[4:8], cmdID)
	binary.BigEndian.PutUint32(buf[8:12], 0)
	binary.BigEndian.PutUint32(buf[12:16], seq)
	if len(payload) == 0 {
		if os.Getenv("SMPP_CAPTURE_DEBUG") != "" {
			log.Printf("smpp send cmd=0x%08x seq=%d len=%d", cmdID, seq, len(payload))
		}
		_, _ = conn.Write(buf)
		return
	}
	if os.Getenv("SMPP_CAPTURE_DEBUG") != "" {
		log.Printf("smpp send cmd=0x%08x seq=%d len=%d", cmdID, seq, len(payload))
	}
	_, _ = conn.Write(append(buf, payload...))
}

func parseSubmitSM(payload []byte) SMPPMessage {
	reader := bytes.NewReader(payload)
	skipCString(reader)  // service_type
	_ = readByte(reader) // source_addr_ton
	_ = readByte(reader) // source_addr_npi
	source := readCString(reader)
	_ = readByte(reader) // dest_addr_ton
	_ = readByte(reader) // dest_addr_npi
	destination := readCString(reader)
	_ = readByte(reader) // esm_class
	_ = readByte(reader) // protocol_id
	_ = readByte(reader) // priority_flag
	skipCString(reader)  // schedule_delivery_time
	skipCString(reader)  // validity_period
	_ = readByte(reader) // registered_delivery
	_ = readByte(reader) // replace_if_present_flag
	_ = readByte(reader) // data_coding
	_ = readByte(reader) // sm_default_msg_id
	msgLen := int(readByte(reader))
	msg := make([]byte, msgLen)
	_, _ = io.ReadFull(reader, msg)

	return SMPPMessage{
		Source:      source,
		Destination: destination,
		Body:        string(msg),
	}
}

func smppBindRespPayload() []byte {
	// system_id is a C-string; omit optional sc_interface_version TLV.
	return []byte{0x00}
}

func readCString(reader *bytes.Reader) string {
	var buf bytes.Buffer
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return buf.String()
		}
		if b == 0x00 {
			return buf.String()
		}
		buf.WriteByte(b)
	}
}

func skipCString(reader *bytes.Reader) {
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return
		}
		if b == 0x00 {
			return
		}
	}
}

func readByte(reader *bytes.Reader) byte {
	b, _ := reader.ReadByte()
	return b
}
