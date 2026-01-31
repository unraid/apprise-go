package notify

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

const (
	mailtoModeInsecure = "insecure"
	mailtoModeSSL      = "ssl"
	mailtoModeStartTLS = "starttls"
)

var mailtoModePorts = map[string]int{
	mailtoModeInsecure: 25,
	mailtoModeStartTLS: 587,
	mailtoModeSSL:      465,
}

type MailtoTarget struct {
	smtpHost     string
	port         int
	secureMode   string
	user         string
	password     string
	fromName     string
	fromAddr     string
	targets      []string
	cc           []string
	bcc          []string
	replyTo      []string
	headers      map[string]string
	notifyFormat string
	verifyTLS    bool
}

type mailtoMessage struct {
	recipient string
	toAddrs   []string
	body      string
}

func NewMailtoTarget(target *ParsedURL) (*MailtoTarget, error) {
	if target == nil {
		return nil, fmt.Errorf("missing target")
	}

	schema := strings.ToLower(strings.TrimSpace(target.Scheme))
	if schema != "mailto" && schema != "mailtos" {
		return nil, fmt.Errorf("invalid schema")
	}

	mode := strings.ToLower(strings.TrimSpace(target.Query["mode"]))
	if mode == "" {
		if schema == "mailtos" {
			mode = mailtoModeStartTLS
		} else {
			mode = mailtoModeInsecure
		}
	}
	if _, ok := mailtoModePorts[mode]; !ok {
		return nil, fmt.Errorf("invalid secure mode")
	}

	port := target.Port
	if !target.HasPort {
		port = mailtoModePorts[mode]
	}

	smtpHost := strings.TrimSpace(target.Query["smtp"])
	if smtpHost == "" {
		smtpHost = strings.TrimSpace(target.Host)
	}
	if smtpHost != "" && !target.HasPort {
		if host, portStr, err := net.SplitHostPort(smtpHost); err == nil {
			if parsedPort, err := strconv.Atoi(portStr); err == nil && parsedPort > 0 {
				port = parsedPort
			}
			smtpHost = host
		}
	}
	if smtpHost == "" {
		return nil, fmt.Errorf("missing smtp host")
	}

	fromName, fromAddr, err := parseMailtoFrom(target)
	if err != nil {
		return nil, err
	}

	rawTargets := splitPath(target.Path)
	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		rawTargets = append(rawTargets, toValue)
	}
	targets := parseMailtoEmailList(rawTargets)
	if len(targets) == 0 {
		targets = append(targets, fromAddr)
	}

	cc := parseMailtoEmailList([]string{target.Query["cc"]})
	bcc := parseMailtoEmailList([]string{target.Query["bcc"]})
	replyTo := parseMailtoEmailList([]string{target.Query["reply"]})

	format := normalizeNotifyFormat(target.Query["format"])
	if format == "" {
		format = "html"
	}
	switch format {
	case "html", "markdown", "text":
	default:
		return nil, fmt.Errorf("invalid format")
	}

	verifyTLS := true
	if rawVerify := strings.TrimSpace(target.Query["verify"]); rawVerify != "" {
		verifyTLS = parseBool(rawVerify, true)
	}

	headers := map[string]string{}
	for key, value := range target.QueryAdd {
		if strings.TrimSpace(key) == "" {
			continue
		}
		headers[key] = value
	}

	return &MailtoTarget{
		smtpHost:     smtpHost,
		port:         port,
		secureMode:   mode,
		user:         strings.TrimSpace(target.User),
		password:     strings.TrimSpace(target.Password),
		fromName:     fromName,
		fromAddr:     fromAddr,
		targets:      targets,
		cc:           cc,
		bcc:          bcc,
		replyTo:      replyTo,
		headers:      headers,
		notifyFormat: format,
		verifyTLS:    verifyTLS,
	}, nil
}

func parseMailtoFrom(target *ParsedURL) (string, string, error) {
	fromRaw := strings.TrimSpace(target.Query["from"])
	nameRaw := strings.TrimSpace(target.Query["name"])

	fromName := ""
	fromAddr := ""

	if fromRaw != "" {
		if parsed, err := mail.ParseAddress(fromRaw); err == nil {
			fromName = parsed.Name
			fromAddr = parsed.Address
		} else {
			fromAddr = fromRaw
		}
	}

	if nameRaw != "" {
		fromName = nameRaw
	}

	if fromAddr == "" {
		user := strings.TrimSpace(target.User)
		host := strings.TrimSpace(target.Host)
		if user != "" {
			if strings.Contains(user, "@") {
				fromAddr = user
			} else if host != "" {
				fromAddr = user + "@" + host
			}
		}
	}

	if !isSimpleEmail(fromAddr) {
		return "", "", fmt.Errorf("invalid from email")
	}

	return fromName, fromAddr, nil
}

func parseMailtoEmailList(inputs []string) []string {
	entries := []string{}
	for _, input := range inputs {
		for _, entry := range parseDelimitedList(input) {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			if isSimpleEmail(entry) {
				entries = append(entries, entry)
			}
		}
	}
	return entries
}

func (m *MailtoTarget) Send(body, title string, notifyType NotifyType) error {
	client, err := m.connect()
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Quit()
	}()

	messages, err := m.buildMessages(body, title)
	if err != nil {
		return err
	}

	for _, message := range messages {
		if err := sendSMTPMessage(client, m.fromAddr, message.toAddrs, message.body); err != nil {
			return err
		}
	}

	return nil
}

func (m *MailtoTarget) connect() (*smtp.Client, error) {
	addr := net.JoinHostPort(m.smtpHost, fmt.Sprintf("%d", m.port))

	if m.secureMode == mailtoModeSSL {
		tlsConfig := &tls.Config{
			ServerName:         m.smtpHost,
			InsecureSkipVerify: !m.verifyTLS,
		}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return nil, err
		}
		client, err := smtp.NewClient(conn, m.smtpHost)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		return m.authenticate(client)
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	client, err := smtp.NewClient(conn, m.smtpHost)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if m.secureMode == mailtoModeStartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			_ = client.Close()
			return nil, fmt.Errorf("server does not support STARTTLS")
		}
		tlsConfig := &tls.Config{
			ServerName:         m.smtpHost,
			InsecureSkipVerify: !m.verifyTLS,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			_ = client.Close()
			return nil, err
		}
	}

	return m.authenticate(client)
}

func (m *MailtoTarget) authenticate(client *smtp.Client) (*smtp.Client, error) {
	if m.user == "" || m.password == "" {
		return client, nil
	}

	auth := smtp.PlainAuth("", m.user, m.password, m.smtpHost)
	if err := client.Auth(auth); err != nil {
		_ = client.Close()
		return nil, err
	}

	return client, nil
}

func (m *MailtoTarget) buildMessages(body, title string) ([]mailtoMessage, error) {
	if len(m.targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	format := m.notifyFormat
	if format == "markdown" {
		format = "text"
	}

	contentType := "text/plain"
	if format == "html" {
		contentType = "text/html"
	}

	encodedBody, err := encodeQuotedPrintable(body)
	if err != nil {
		encodedBody = body
	}
	encodedBody = normalizeCRLF(encodedBody)

	subject := ""
	if strings.TrimSpace(title) != "" {
		subject = encodeRFC2047(title)
	}

	fromHeader := formatMIMEAddress(m.fromName, m.fromAddr)
	messages := make([]mailtoMessage, 0, len(m.targets))

	for _, target := range m.targets {
		cc := filterEmailList(m.cc, append([]string{}, m.bcc...), target)
		bcc := filterEmailList(m.bcc, nil, target)
		reply := filterEmailList(m.replyTo, nil, target)

		headers := []string{
			fmt.Sprintf("Subject: %s", subject),
			fmt.Sprintf("From: %s", fromHeader),
			fmt.Sprintf("To: %s", formatMIMEAddress("", target)),
			fmt.Sprintf("Date: %s", time.Now().Format(time.RFC1123Z)),
			fmt.Sprintf("Message-ID: %s", mailtoMessageID(m.smtpHost)),
			"MIME-Version: 1.0",
			fmt.Sprintf("Content-Type: %s; charset=\"utf-8\"", contentType),
			"Content-Transfer-Encoding: quoted-printable",
			"X-Application: Apprise",
		}

		if len(cc) > 0 {
			headers = append(headers, "Cc: "+joinMailtoAddresses(cc))
		}
		if len(reply) > 0 {
			headers = append(headers, "Reply-To: "+joinMailtoAddresses(reply))
		}
		for key, value := range m.headers {
			headers = append(headers, fmt.Sprintf("%s: %s", key, value))
		}

		data := strings.Join(headers, "\r\n") + "\r\n\r\n" + encodedBody
		toAddrs := append([]string{target}, cc...)
		toAddrs = append(toAddrs, bcc...)
		messages = append(messages, mailtoMessage{
			recipient: target,
			toAddrs:   toAddrs,
			body:      data,
		})
	}

	return messages, nil
}

func filterEmailList(source, remove []string, target string) []string {
	removeSet := map[string]struct{}{}
	for _, entry := range remove {
		removeSet[strings.ToLower(strings.TrimSpace(entry))] = struct{}{}
	}
	if target != "" {
		removeSet[strings.ToLower(strings.TrimSpace(target))] = struct{}{}
	}

	out := []string{}
	seen := map[string]struct{}{}
	for _, entry := range source {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		key := strings.ToLower(entry)
		if _, ok := removeSet[key]; ok {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, entry)
	}
	return out
}

func joinMailtoAddresses(addresses []string) string {
	formatted := make([]string, 0, len(addresses))
	for _, entry := range addresses {
		formatted = append(formatted, formatMIMEAddress("", entry))
	}
	return strings.Join(formatted, ", ")
}

func mailtoMessageID(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "localhost"
	}
	return fmt.Sprintf("<%d@%s>", time.Now().UnixNano(), host)
}

func normalizeCRLF(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.ReplaceAll(value, "\n", "\r\n")
}

func sendSMTPMessage(client *smtp.Client, from string, to []string, body string) error {
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err := client.Rcpt(addr); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte(body)); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}
