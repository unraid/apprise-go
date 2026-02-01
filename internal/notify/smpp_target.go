package notify

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	smppDefaultPort        = 2775
	smppDefaultTLSPort     = 3550
	smppCommandBindTX      = 0x00000002
	smppCommandBindTXRes   = 0x80000002
	smppCommandSubmitSM    = 0x00000004
	smppCommandSubmitSMRes = 0x80000004
	smppCommandUnbind      = 0x00000006
	smppCommandUnbindRes   = 0x80000006
)

type SMPPTarget struct {
	host     string
	port     int
	user     string
	password string
	source   string
	targets  []string
}

func NewSMPPTarget(target *ParsedURL) (*SMPPTarget, error) {
	if target == nil {
		return nil, fmt.Errorf("missing target")
	}

	schema := strings.ToLower(strings.TrimSpace(target.Scheme))
	if schema != "smpp" && schema != "smpps" {
		return nil, fmt.Errorf("invalid schema")
	}

	user := strings.TrimSpace(target.User)
	password := strings.TrimSpace(target.Password)
	if user == "" || password == "" {
		return nil, fmt.Errorf("missing smpp credentials")
	}

	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	port := target.Port
	if !target.HasPort {
		if schema == "smpps" {
			port = smppDefaultTLSPort
		} else {
			port = smppDefaultPort
		}
	}

	source, targets := parseSMPPSourceTargets(target)
	if source == "" {
		return nil, fmt.Errorf("missing source")
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	return &SMPPTarget{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		source:   source,
		targets:  targets,
	}, nil
}

func (s *SMPPTarget) Send(body, title string, _ NotifyType) error {
	if len(s.targets) == 0 {
		return fmt.Errorf("no smpp targets")
	}

	message := body
	if strings.TrimSpace(title) != "" {
		message = mergeTitleBody(title, body)
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(s.host, strconv.Itoa(s.port)), 5*time.Second)
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()

	seq := uint32(1)
	if err := smppBindTransmitter(conn, s.user, s.password, seq); err != nil {
		return err
	}
	seq++

	for _, target := range s.targets {
		if err := smppSubmitSM(conn, s.source, target, message, seq); err != nil {
			return err
		}
		seq++
	}

	_ = smppUnbind(conn, seq)
	return nil
}

func parseSMPPSourceTargets(target *ParsedURL) (string, []string) {
	var rawTargets []string

	if raw := strings.TrimSpace(target.Query["to"]); raw != "" {
		rawTargets = append(rawTargets, parseDelimitedList(raw)...)
	}
	if entries := splitPath(target.Path); len(entries) > 0 {
		rawTargets = append(rawTargets, entries...)
	}

	source := strings.TrimSpace(target.Query["from"])
	if source == "" && len(rawTargets) > 0 {
		source = rawTargets[0]
		rawTargets = rawTargets[1:]
	}
	if source == "" {
		source = strings.TrimSpace(target.Host)
	}

	source = normalizePhoneDigits(source)
	targets := []string{}
	for _, entry := range rawTargets {
		if normalized := normalizePhoneDigits(entry); normalized != "" {
			targets = append(targets, normalized)
		}
	}

	return source, targets
}

func normalizePhoneDigits(raw string) string {
	normalized, ok := normalizePhone(raw)
	if !ok {
		return ""
	}
	return normalized
}

func smppBindTransmitter(conn net.Conn, user, password string, seq uint32) error {
	payload := []byte{}
	payload = append(payload, smppCString(user)...)
	payload = append(payload, smppCString(password)...)
	payload = append(payload, smppCString("")...)
	payload = append(payload, 0x34, 0x00, 0x00)
	payload = append(payload, smppCString("")...)

	if err := smppWritePDU(conn, smppCommandBindTX, seq, payload); err != nil {
		return err
	}
	_, status, err := smppReadPDU(conn)
	if err != nil {
		return err
	}
	if status != 0 {
		return fmt.Errorf("smpp bind failed")
	}
	return nil
}

func smppSubmitSM(conn net.Conn, source, destination, message string, seq uint32) error {
	payload := []byte{}
	payload = append(payload, smppCString("")...) // service_type
	payload = append(payload, 0x01, 0x01)         // source_addr_ton/npi
	payload = append(payload, smppCString(source)...)
	payload = append(payload, 0x01, 0x01) // dest_addr_ton/npi
	payload = append(payload, smppCString(destination)...)
	payload = append(payload, 0x00)               // esm_class
	payload = append(payload, 0x00)               // protocol_id
	payload = append(payload, 0x00)               // priority_flag
	payload = append(payload, smppCString("")...) // schedule_delivery_time
	payload = append(payload, smppCString("")...) // validity_period
	payload = append(payload, 0x01)               // registered_delivery
	payload = append(payload, 0x00)               // replace_if_present_flag
	payload = append(payload, 0x00)               // data_coding
	payload = append(payload, 0x00)               // sm_default_msg_id

	msgBytes := []byte(message)
	if len(msgBytes) > 255 {
		msgBytes = msgBytes[:255]
	}
	payload = append(payload, byte(len(msgBytes)))
	payload = append(payload, msgBytes...)

	if err := smppWritePDU(conn, smppCommandSubmitSM, seq, payload); err != nil {
		return err
	}
	_, status, err := smppReadPDU(conn)
	if err != nil {
		return err
	}
	if status != 0 {
		return fmt.Errorf("smpp submit failed")
	}
	return nil
}

func smppUnbind(conn net.Conn, seq uint32) error {
	if err := smppWritePDU(conn, smppCommandUnbind, seq, nil); err != nil {
		return err
	}
	_, _, err := smppReadPDU(conn)
	return err
}

func smppWritePDU(conn net.Conn, cmdID uint32, seq uint32, payload []byte) error {
	length := uint32(16 + len(payload))
	header := make([]byte, 16)
	binary.BigEndian.PutUint32(header[0:4], length)
	binary.BigEndian.PutUint32(header[4:8], cmdID)
	binary.BigEndian.PutUint32(header[8:12], 0)
	binary.BigEndian.PutUint32(header[12:16], seq)
	packet := append(header, payload...)
	_, err := conn.Write(packet)
	return err
}

func smppReadPDU(conn net.Conn) (uint32, uint32, error) {
	header := make([]byte, 16)
	if _, err := io.ReadFull(conn, header); err != nil {
		return 0, 0, err
	}
	length := binary.BigEndian.Uint32(header[0:4])
	cmdID := binary.BigEndian.Uint32(header[4:8])
	status := binary.BigEndian.Uint32(header[8:12])
	if length > 16 {
		body := make([]byte, length-16)
		if _, err := io.ReadFull(conn, body); err != nil {
			return cmdID, status, err
		}
	}
	return cmdID, status, nil
}

func smppCString(value string) []byte {
	return append([]byte(value), 0x00)
}
