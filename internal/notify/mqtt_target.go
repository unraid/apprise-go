package notify

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	mqttDefaultPort      = 1883
	mqttDefaultTLSPort   = 8883
	mqttKeepAliveSeconds = 30
)

type mqttProtocol int

const (
	mqttProtocol31  mqttProtocol = 3
	mqttProtocol311 mqttProtocol = 4
	mqttProtocol5   mqttProtocol = 5
)

type MQTTTarget struct {
	host       string
	port       int
	secure     bool
	user       string
	password   string
	topics     []string
	qos        int
	retain     bool
	clientID   string
	cleanStart bool
	protocol   mqttProtocol
	verifyTLS  bool
}

func NewMQTTTarget(target *ParsedURL) (*MQTTTarget, error) {
	if target == nil {
		return nil, fmt.Errorf("missing target")
	}

	schema := strings.ToLower(strings.TrimSpace(target.Scheme))
	if schema != "mqtt" && schema != "mqtts" {
		return nil, fmt.Errorf("invalid schema")
	}

	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	user := strings.TrimSpace(target.User)
	password := strings.TrimSpace(target.Password)
	if user == "" {
		return nil, fmt.Errorf("missing user")
	}

	secure := schema == "mqtts"
	port := target.Port
	if !target.HasPort {
		if secure {
			port = mqttDefaultTLSPort
		} else {
			port = mqttDefaultPort
		}
	}

	topics := parseMQTTTargets(target)
	if len(topics) == 0 {
		return nil, fmt.Errorf("missing topics")
	}

	qos := 0
	if raw := strings.TrimSpace(target.Query["qos"]); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 || parsed > 2 {
			return nil, fmt.Errorf("invalid qos")
		}
		qos = parsed
	}

	version := strings.TrimSpace(target.Query["version"])
	if version == "" {
		version = "v3.1.1"
	}
	protocol, err := parseMQTTProtocol(version)
	if err != nil {
		return nil, err
	}

	clientID := strings.TrimSpace(target.Query["client_id"])
	if clientID == "" {
		clientID = "apprise-go"
	}

	session := parseBoolWithDefault(target.Query["session"], false)
	retain := parseBoolWithDefault(target.Query["retain"], false)
	verifyTLS := parseBoolWithDefault(target.Query["verify"], true)

	return &MQTTTarget{
		host:       host,
		port:       port,
		secure:     secure,
		user:       user,
		password:   password,
		topics:     topics,
		qos:        qos,
		retain:     retain,
		clientID:   clientID,
		cleanStart: !session,
		protocol:   protocol,
		verifyTLS:  verifyTLS,
	}, nil
}

func (m *MQTTTarget) Send(body, title string, _ NotifyType) error {
	if len(m.topics) == 0 {
		return fmt.Errorf("no mqtt topics")
	}

	payload := body
	if strings.TrimSpace(title) != "" {
		payload = mergeTitleBody(title, body)
	}

	conn, err := m.connect()
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()

	packetID := uint16(1)
	for _, topic := range m.topics {
		if err := m.publish(conn, topic, payload, packetID); err != nil {
			return err
		}
		if m.qos > 0 {
			packetID++
		}
	}

	return nil
}

func (m *MQTTTarget) connect() (net.Conn, error) {
	addr := net.JoinHostPort(m.host, strconv.Itoa(m.port))
	var conn net.Conn
	var err error
	if m.secure {
		tlsConfig := &tls.Config{
			ServerName:         m.host,
			InsecureSkipVerify: !m.verifyTLS,
		}
		if m.verifyTLS {
			if pool, ok, err := loadCertPoolFromEnv(); err != nil {
				return nil, err
			} else if ok {
				tlsConfig.RootCAs = pool
			} else {
				caCert, err := mqttFindCACert()
				if err != nil {
					return nil, err
				}
				tlsConfig.RootCAs, err = loadCertPoolFromFile(caCert)
				if err != nil {
					return nil, err
				}
			}
		}
		conn, err = tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return nil, err
		}
	} else {
		conn, err = net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			return nil, err
		}
	}

	if err := m.sendConnect(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := m.readConnAck(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return conn, nil
}

func (m *MQTTTarget) sendConnect(conn net.Conn) error {
	varHeader := make([]byte, 0, 16)
	switch m.protocol {
	case mqttProtocol31:
		varHeader = append(varHeader, encodeMQTTString("MQIsdp")...)
		varHeader = append(varHeader, byte(mqttProtocol31))
	case mqttProtocol5:
		varHeader = append(varHeader, encodeMQTTString("MQTT")...)
		varHeader = append(varHeader, byte(mqttProtocol5))
	default:
		varHeader = append(varHeader, encodeMQTTString("MQTT")...)
		varHeader = append(varHeader, byte(mqttProtocol311))
	}

	connectFlags := byte(0)
	if m.cleanStart {
		connectFlags |= 0x02
	}
	if m.user != "" {
		connectFlags |= 0x80
	}
	if m.password != "" {
		connectFlags |= 0x40
	}
	varHeader = append(varHeader, connectFlags)
	varHeader = append(varHeader, byte(mqttKeepAliveSeconds>>8), byte(mqttKeepAliveSeconds))
	if m.protocol == mqttProtocol5 {
		varHeader = append(varHeader, 0x00) // properties length
	}

	payload := make([]byte, 0, 64)
	payload = append(payload, encodeMQTTString(m.clientID)...)
	if m.user != "" {
		payload = append(payload, encodeMQTTString(m.user)...)
	}
	if m.password != "" {
		payload = append(payload, encodeMQTTString(m.password)...)
	}

	remaining := len(varHeader) + len(payload)
	packet := []byte{0x10}
	packet = append(packet, encodeMQTTRemaining(remaining)...)
	packet = append(packet, varHeader...)
	packet = append(packet, payload...)

	_, err := conn.Write(packet)
	return err
}

func (m *MQTTTarget) readConnAck(conn net.Conn) error {
	packetType, remaining, payload, err := readMQTTPacket(conn)
	if err != nil {
		return err
	}
	if packetType != 0x20 {
		return fmt.Errorf("unexpected connack packet: 0x%02x", packetType)
	}
	if remaining < 2 {
		return fmt.Errorf("invalid connack length")
	}
	if m.protocol == mqttProtocol5 && len(payload) >= 3 {
		if payload[1] != 0 {
			return fmt.Errorf("mqtt connect failed: %d", payload[1])
		}
		return nil
	}
	if payload[1] != 0 {
		return fmt.Errorf("mqtt connect failed: %d", payload[1])
	}
	return nil
}

func (m *MQTTTarget) publish(conn net.Conn, topic, payload string, packetID uint16) error {
	flags := byte(0x30)
	if m.qos == 1 {
		flags = 0x32
	} else if m.qos == 2 {
		flags = 0x34
	}
	if m.retain {
		flags |= 0x01
	}

	varHeader := make([]byte, 0, len(topic)+2)
	varHeader = append(varHeader, encodeMQTTString(topic)...)
	if m.qos > 0 {
		varHeader = append(varHeader, byte(packetID>>8), byte(packetID))
	}
	if m.protocol == mqttProtocol5 {
		varHeader = append(varHeader, 0x00)
	}

	remaining := len(varHeader) + len(payload)
	packet := []byte{flags}
	packet = append(packet, encodeMQTTRemaining(remaining)...)
	packet = append(packet, varHeader...)
	packet = append(packet, []byte(payload)...)

	if _, err := conn.Write(packet); err != nil {
		return err
	}

	if m.qos == 1 {
		return awaitMQTTPubAck(conn, packetID)
	}
	if m.qos == 2 {
		return awaitMQTTPubRec(conn, packetID)
	}
	return nil
}

func awaitMQTTPubAck(conn net.Conn, packetID uint16) error {
	packetType, _, payload, err := readMQTTPacket(conn)
	if err != nil {
		return err
	}
	if packetType != 0x40 {
		return fmt.Errorf("unexpected puback packet: 0x%02x", packetType)
	}
	if len(payload) < 2 {
		return fmt.Errorf("invalid puback")
	}
	got := uint16(payload[0])<<8 | uint16(payload[1])
	if got != packetID {
		return fmt.Errorf("puback packet id mismatch")
	}
	return nil
}

func awaitMQTTPubRec(conn net.Conn, packetID uint16) error {
	packetType, _, payload, err := readMQTTPacket(conn)
	if err != nil {
		return err
	}
	if packetType != 0x50 {
		return fmt.Errorf("unexpected pubrec packet: 0x%02x", packetType)
	}
	if len(payload) < 2 {
		return fmt.Errorf("invalid pubrec")
	}
	got := uint16(payload[0])<<8 | uint16(payload[1])
	if got != packetID {
		return fmt.Errorf("pubrec packet id mismatch")
	}

	pubrel := []byte{0x62, 0x02, payload[0], payload[1]}
	if _, err := conn.Write(pubrel); err != nil {
		return err
	}

	packetType, _, payload, err = readMQTTPacket(conn)
	if err != nil {
		return err
	}
	if packetType != 0x70 {
		return fmt.Errorf("unexpected pubcomp packet: 0x%02x", packetType)
	}
	if len(payload) < 2 {
		return fmt.Errorf("invalid pubcomp")
	}
	got = uint16(payload[0])<<8 | uint16(payload[1])
	if got != packetID {
		return fmt.Errorf("pubcomp packet id mismatch")
	}
	return nil
}

func parseMQTTTargets(target *ParsedURL) []string {
	if target == nil {
		return nil
	}
	path := strings.TrimLeft(target.Path, "/")
	if path == "" && target.Host == "" {
		return nil
	}
	values := parseDelimitedList(path)
	if raw := strings.TrimSpace(target.Query["to"]); raw != "" {
		values = append(values, parseDelimitedList(raw)...)
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func parseMQTTProtocol(raw string) (mqttProtocol, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "v3.1":
		return mqttProtocol31, nil
	case "v5.0":
		return mqttProtocol5, nil
	case "v3.1.1", "":
		return mqttProtocol311, nil
	default:
		return mqttProtocol311, fmt.Errorf("invalid mqtt protocol")
	}
}

func encodeMQTTString(value string) []byte {
	out := make([]byte, 0, len(value)+2)
	out = append(out, byte(len(value)>>8), byte(len(value)))
	out = append(out, []byte(value)...)
	return out
}

func encodeMQTTRemaining(value int) []byte {
	out := []byte{}
	for {
		encoded := byte(value % 128)
		value /= 128
		if value > 0 {
			encoded |= 128
		}
		out = append(out, encoded)
		if value <= 0 {
			break
		}
	}
	return out
}

func readMQTTPacket(conn net.Conn) (byte, int, []byte, error) {
	header := make([]byte, 1)
	if _, err := io.ReadFull(conn, header); err != nil {
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
	return header[0] & 0xF0, remaining, payload, nil
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

func mqttFindCACert() (string, error) {
	candidates := []string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/pki/tls/certs/ca-bundle.crt",
		"/etc/ssl/ca-bundle.pem",
		"/etc/pki/tls/cacert.pem",
		"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem",
		"/usr/local/etc/ca-certificates/cert.pem",
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("mqtt ca certificates not found")
}
