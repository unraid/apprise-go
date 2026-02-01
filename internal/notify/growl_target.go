package notify

import (
	"crypto/md5"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const growlDefaultPort = 23053

var growlPriorityMap = map[string]int{
	"l":  -2,
	"m":  -1,
	"n":  0,
	"h":  1,
	"e":  2,
	"-2": -2,
	"-1": -1,
	"0":  0,
	"1":  1,
	"2":  2,
}

type GrowlTarget struct {
	host         string
	port         int
	password     string
	priority     int
	version      int
	includeImage bool
	sticky       bool
	registered   bool
}

func NewGrowlTarget(target *ParsedURL) (*GrowlTarget, error) {
	if target == nil {
		return nil, fmt.Errorf("missing target")
	}
	if strings.ToLower(strings.TrimSpace(target.Scheme)) != "growl" {
		return nil, fmt.Errorf("invalid schema")
	}

	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	port := target.Port
	if !target.HasPort {
		port = growlDefaultPort
	}

	password := strings.TrimSpace(target.Password)
	if password == "" {
		password = strings.TrimSpace(target.User)
	}

	version := 2
	if raw := strings.TrimSpace(target.Query["version"]); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			version = parsed
		}
	}

	priority := 0
	if raw := strings.TrimSpace(target.Query["priority"]); raw != "" {
		priority = growlPriorityFromRaw(raw)
	}

	includeImage := parseBoolWithDefault(target.Query["image"], true)
	sticky := parseBoolWithDefault(target.Query["sticky"], false)

	return &GrowlTarget{
		host:         host,
		port:         port,
		password:     password,
		priority:     priority,
		version:      version,
		includeImage: includeImage,
		sticky:       sticky,
	}, nil
}

func (g *GrowlTarget) Send(body, title string, notifyType NotifyType) error {
	if !g.registered {
		if err := g.register(); err != nil {
			return err
		}
		g.registered = true
	}

	noticeHeaders := map[string]string{
		"Application-Name":   "Apprise",
		"Notification-Name":  "New Messages",
		"Notification-Title": title,
		"Notification-Text":  body,
	}

	if g.priority != 0 {
		noticeHeaders["Notification-Priority"] = strconv.Itoa(g.priority)
	}
	if g.sticky {
		noticeHeaders["Notification-Sticky"] = "True"
	}

	if g.includeImage {
		if iconURL := appriseImageURL(notifyType, "72x72"); iconURL != "" {
			noticeHeaders["Notification-Icon"] = iconURL
		}
	}

	return g.sendMessage("NOTIFY", noticeHeaders, nil)
}

func (g *GrowlTarget) register() error {
	headers := map[string]string{
		"Application-Name":    "Apprise",
		"Notifications-Count": "1",
	}

	notifications := []map[string]string{
		{
			"Notification-Name":    "New Messages",
			"Notification-Enabled": "True",
		},
	}

	if g.includeImage {
		if iconURL := appriseImageURL(NotifyInfo, "72x72"); iconURL != "" {
			headers["Application-Icon"] = iconURL
		}
	}

	return g.sendMessage("REGISTER", headers, notifications)
}

func (g *GrowlTarget) sendMessage(messageType string, headers map[string]string, notifications []map[string]string) error {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(g.host, strconv.Itoa(g.port)), 3*time.Second)
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()

	infoLine, err := g.gntpInfoLine(messageType)
	if err != nil {
		return err
	}

	var builder strings.Builder
	builder.WriteString(infoLine)
	builder.WriteString("\r\n")
	for key, value := range headers {
		builder.WriteString(key)
		builder.WriteString(": ")
		builder.WriteString(value)
		builder.WriteString("\r\n")
	}
	builder.WriteString("\r\n")
	for _, notice := range notifications {
		for key, value := range notice {
			builder.WriteString(key)
			builder.WriteString(": ")
			builder.WriteString(value)
			builder.WriteString("\r\n")
		}
		builder.WriteString("\r\n")
	}

	if _, err := conn.Write([]byte(builder.String())); err != nil {
		return err
	}

	resp := make([]byte, 128)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, err := conn.Read(resp)
	if err != nil {
		return err
	}
	if !strings.Contains(strings.ToUpper(string(resp[:n])), "-OK") {
		return fmt.Errorf("growl server rejected notification")
	}
	return nil
}

func (g *GrowlTarget) gntpInfoLine(messageType string) (string, error) {
	info := fmt.Sprintf("GNTP/1.0 %s NONE", messageType)
	if g.password == "" {
		return info, nil
	}

	seed := []byte(time.Now().Format(time.ANSIC))
	// codeql[go/weak-sensitive-data-hashing]
	saltHash := md5.Sum(seed)
	saltHex := strings.ToUpper(fmt.Sprintf("%x", saltHash))
	keyBasis := append([]byte(g.password), saltHash[:]...)
	// codeql[go/weak-sensitive-data-hashing]
	key := md5.Sum(keyBasis)
	// codeql[go/weak-sensitive-data-hashing]
	keyHash := md5.Sum(key[:])
	info = fmt.Sprintf("GNTP/1.0 %s NONE MD5:%s.%s", messageType, strings.ToUpper(fmt.Sprintf("%x", keyHash)), saltHex)
	return info, nil
}

func growlPriorityFromRaw(raw string) int {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	for key, value := range growlPriorityMap {
		if strings.HasPrefix(normalized, key) {
			return value
		}
	}
	return 0
}
