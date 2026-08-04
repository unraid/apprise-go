package notify

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"regexp"
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

	// inline embeds image attachments in the body rather than leaving them
	// as downloads, which turns the message into multipart/related.
	inline bool
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

	// PGP signing and encryption are not implemented here. Accepting the
	// request and sending plaintext would be worse than refusing it: someone
	// who asked for encryption would believe they had it.
	if pgpMode := strings.ToLower(strings.TrimSpace(target.Query["pgp"])); pgpMode != "" {
		switch {
		case strings.HasPrefix("no", pgpMode):
		case strings.HasPrefix("sign", pgpMode), strings.HasPrefix("encrypt", pgpMode):
			return nil, fmt.Errorf("pgp %s is not supported", pgpMode)
		default:
			return nil, fmt.Errorf("invalid pgp mode: %s", target.Query["pgp"])
		}
	}
	// wkd=yes implies encryption upstream, so it is refused for the same
	// reason.
	if parseBoolWithDefault(target.Query["wkd"], false) {
		return nil, fmt.Errorf("pgp web key directory is not supported")
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
		inline:       parseBoolWithDefault(target.Query["inline"], false),
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
	return m.SendWithAttachments(body, title, notifyType, nil)
}

func (m *MailtoTarget) SendWithAttachments(body, title string, notifyType NotifyType, attachments []Attachment) error {
	client, err := m.connect()
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Quit()
	}()

	messages, err := m.buildMessages(body, title, attachments)
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
		if m.verifyTLS {
			if pool, ok, err := loadCertPoolFromEnv(); err != nil {
				return nil, err
			} else if ok {
				tlsConfig.RootCAs = pool
			}
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
		if m.verifyTLS {
			if pool, ok, err := loadCertPoolFromEnv(); err != nil {
				return nil, err
			} else if ok {
				tlsConfig.RootCAs = pool
			}
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

func (m *MailtoTarget) buildMessages(body, title string, attachments []Attachment) ([]mailtoMessage, error) {
	if len(m.targets) == 0 {
		return nil, fmt.Errorf("missing targets")
	}

	format := m.notifyFormat
	if format == "markdown" {
		format = "text"
	}

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

		contentTypeHeader, transferHeader, messageBody := buildMailtoBody(
			body, format, m.inline, attachments)
		headers := []string{
			fmt.Sprintf("Subject: %s", subject),
			fmt.Sprintf("From: %s", fromHeader),
			fmt.Sprintf("To: %s", formatMIMEAddress("", target)),
			fmt.Sprintf("Date: %s", time.Now().Format(time.RFC1123Z)),
			fmt.Sprintf("Message-ID: %s", mailtoMessageID(m.smtpHost)),
			"MIME-Version: 1.0",
			fmt.Sprintf("Content-Type: %s", contentTypeHeader),
			"X-Application: Apprise",
		}
		if transferHeader != "" {
			headers = append(headers, fmt.Sprintf("Content-Transfer-Encoding: %s", transferHeader))
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

		data := strings.Join(headers, "\r\n") + "\r\n\r\n" + messageBody
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

// buildMailtoBody returns the message's content type, transfer encoding and
// body. Attachments wrap whatever the body would otherwise have been in one
// more multipart layer, so the text keeps its own structure inside it.
func buildMailtoBody(body, format string, inline bool, attachments []Attachment) (string, string, string) {
	// Inline mode rewrites the body before it is encoded, appending an anchor
	// per image and deciding whether the message is related or mixed.
	cidRefs := map[string]struct{}{}
	if inline && len(attachments) > 0 {
		body, cidRefs = applyMailtoInline(body, format, attachments)
	}

	contentType, transfer, encoded := buildMailtoContentBody(body, format)
	if len(attachments) == 0 {
		return contentType, transfer, encoded
	}

	// A message carrying an inline image is related rather than mixed: the
	// parts are one document, not a document plus downloads.
	subtype := "mixed"
	if len(cidRefs) > 0 {
		subtype = "related"
	}

	boundary := mailtoBoundary()
	var builder strings.Builder
	builder.WriteString("--" + boundary + "\r\n")

	// The text's own headers travel with it, one layer down.
	builder.WriteString("Content-Type: " + contentType + "\r\n")
	builder.WriteString("MIME-Version: 1.0\r\n")
	if transfer != "" {
		builder.WriteString("Content-Transfer-Encoding: " + transfer + "\r\n")
	}
	builder.WriteString("\r\n" + encoded + "\r\n")

	for index, attachment := range attachments {
		name := attachment.FileName(index, ".dat")
		mimeType := attachment.MimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		builder.WriteString("--" + boundary + "\r\n")
		builder.WriteString("Content-Transfer-Encoding: base64\r\n")
		builder.WriteString("MIME-Version: 1.0\r\n")
		builder.WriteString("Content-Type: " + mimeType + "\r\n")

		if _, embedded := cidRefs[name]; embedded {
			builder.WriteString(fmt.Sprintf(
				"Content-Disposition: inline; filename=%q\r\n", name))
			// Spaces are escaped so the id still matches the cid: URI in
			// the body, which cannot carry a raw space.
			builder.WriteString(fmt.Sprintf("Content-ID: <%s>\r\n",
				strings.ReplaceAll(name, " ", "%20")))
		} else {
			builder.WriteString(fmt.Sprintf(
				"Content-Disposition: attachment; filename=%q\r\n", name))
		}

		builder.WriteString("\r\n")
		builder.WriteString(normalizeCRLF(wrapBase64(attachment.Base64(), 76)))
	}

	builder.WriteString("\r\n--" + boundary + "--\r\n")

	return fmt.Sprintf("multipart/%s; boundary=%q", subtype, boundary), "", builder.String()
}

// applyMailtoInline rewrites the body so images are referenced from it, and
// reports which filenames ended up embedded.
//
// A cid: URI the caller already wrote is honoured for any attachment type —
// it can only resolve inside the same message, so writing one is a deliberate
// act. Images not already referenced get an anchor appended.
func applyMailtoInline(body, format string, attachments []Attachment) (string, map[string]struct{}) {
	names := make([]string, len(attachments))
	nameSet := map[string]struct{}{}
	for index, attachment := range attachments {
		names[index] = attachment.FileName(index, ".dat")
		nameSet[names[index]] = struct{}{}
	}

	if format != "html" {
		// Plain text cannot embed anything, so images are named instead.
		var listed []string
		for index, attachment := range attachments {
			if strings.HasPrefix(strings.ToLower(attachment.MimeType), "image/") {
				listed = append(listed, names[index])
			}
		}
		if len(listed) > 0 {
			for _, name := range listed {
				body += "\n[Image: " + name + "]"
			}
		}

		return body, nil
	}

	refs := map[string]struct{}{}
	for _, match := range mailtoCIDPattern.FindAllStringSubmatch(body, -1) {
		ref := strings.ReplaceAll(match[1], "%20", " ")
		// A reference with no matching file cannot resolve, so it is left
		// out rather than making the message related for nothing.
		if _, ok := nameSet[ref]; ok {
			refs[ref] = struct{}{}
		}
	}

	for index, attachment := range attachments {
		if !strings.HasPrefix(strings.ToLower(attachment.MimeType), "image/") {
			continue
		}
		if _, ok := refs[names[index]]; ok {
			continue
		}
		refs[names[index]] = struct{}{}
		body += fmt.Sprintf(`<br/><img src="cid:%s">`,
			strings.ReplaceAll(names[index], " ", "%20"))
	}

	return body, refs
}

// mailtoCIDPattern finds the filenames a body already references inline.
var mailtoCIDPattern = regexp.MustCompile(`cid:([^\s"'>)]+)`)

func mailtoBoundary() string {
	return fmt.Sprintf("===============%d==", time.Now().UnixNano())
}

func buildMailtoContentBody(body, format string) (string, string, string) {
	if format == "html" {
		plain := htmlToText(body)
		html := body

		plainEncoded, err := encodeQuotedPrintable(plain)
		if err != nil {
			plainEncoded = plain
		}
		plainEncoded = normalizeCRLF(plainEncoded)

		htmlEncoded, err := encodeQuotedPrintable(html)
		if err != nil {
			htmlEncoded = html
		}
		htmlEncoded = normalizeCRLF(htmlEncoded)

		boundary := fmt.Sprintf("===============%d==", time.Now().UnixNano())
		bodyPayload := buildMultipartAlternative(boundary, plainEncoded, htmlEncoded)
		return fmt.Sprintf("multipart/alternative; boundary=\"%s\"", boundary), "", bodyPayload
	}

	encodedBody, err := encodeQuotedPrintable(body)
	if err != nil {
		encodedBody = body
	}
	encodedBody = normalizeCRLF(encodedBody)

	return "text/plain; charset=\"utf-8\"", "quoted-printable", encodedBody
}

func buildMultipartAlternative(boundary, plain, html string) string {
	lines := []string{
		"--" + boundary,
		"Content-Type: text/plain; charset=\"utf-8\"",
		"MIME-Version: 1.0",
		"Content-Transfer-Encoding: quoted-printable",
		"",
		plain,
		"--" + boundary,
		"Content-Type: text/html; charset=\"utf-8\"",
		"MIME-Version: 1.0",
		"Content-Transfer-Encoding: quoted-printable",
		"",
		html,
		"--" + boundary + "--",
		"",
	}
	return strings.Join(lines, "\r\n")
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
