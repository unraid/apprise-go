package notify

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/unraid/apprise-go/internal/version"
)

const (
	aprsDefaultPort   = 10152
	aprsDeviceID      = "APPRIS"
	aprsBodyMaxLength = 67
	aprsTimeout       = 4 * time.Second
)

var aprsLocales = map[string]string{
	"NOAM": "noam.aprs2.net",
	"SOAM": "soam.aprs2.net",
	"EURO": "euro.aprs2.net",
	"ASIA": "asia.aprs2.net",
	"AUNZ": "aunz.aprs2.net",
	"ROTA": "rotate.aprs2.net",
}

var aprsBadCharRegex = regexp.MustCompile(`[{}|~]+`)

var aprsBadCharReplacer = strings.NewReplacer(
	"Ä", "Ae",
	"Ö", "Oe",
	"Ü", "Ue",
	"ä", "ae",
	"ö", "oe",
	"ü", "ue",
	"ß", "ss",
)

var aprsCallSignRegex = regexp.MustCompile(`(?i)^[0-9a-z]{1,2}[0-9][a-z0-9]{1,3}(-[0-9]{1,2})?$`)

type AprsTarget struct {
	user     string
	password string
	targets  []string
	locale   string
	delay    float64
}

func NewAprsTarget(target *ParsedURL) (*AprsTarget, error) {
	if target == nil {
		return nil, fmt.Errorf("missing target")
	}

	if strings.ToLower(strings.TrimSpace(target.Scheme)) != "aprs" {
		return nil, fmt.Errorf("invalid schema")
	}

	user := strings.TrimSpace(target.User)
	password := strings.TrimSpace(target.Password)
	if user == "" || password == "" {
		return nil, fmt.Errorf("missing aprs user/pass")
	}
	if password == "-1" {
		return nil, fmt.Errorf("aprs read-only passwords are not supported")
	}
	if !isNumericString(password) {
		return nil, fmt.Errorf("invalid aprs password")
	}

	locale := strings.TrimSpace(target.Query["locale"])
	if locale == "" {
		locale = "EURO"
	}
	locale = strings.ToUpper(locale)
	if _, ok := aprsLocales[locale]; !ok {
		return nil, fmt.Errorf("unsupported aprs locale")
	}

	delay := 0.0
	if raw := strings.TrimSpace(target.Query["delay"]); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil || parsed < 0 || parsed >= 5 {
			return nil, fmt.Errorf("unsupported aprs delay")
		}
		delay = parsed
	}

	rawTargets := []string{}
	if strings.TrimSpace(target.Host) != "" {
		rawTargets = append(rawTargets, target.Host)
	}
	if entries := splitPath(target.Path); len(entries) > 0 {
		rawTargets = append(rawTargets, entries...)
	}
	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		rawTargets = append(rawTargets, toValue)
	}

	targets := []string{}
	for _, candidate := range parseAprsCallSigns(rawTargets) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if isAprsCallSign(candidate) {
			targets = append(targets, strings.ToUpper(candidate))
		}
	}

	return &AprsTarget{
		user:     strings.ToUpper(user),
		password: password,
		targets:  targets,
		locale:   locale,
		delay:    delay,
	}, nil
}

func (a *AprsTarget) Send(body, title string, _ NotifyType) error {
	if len(a.targets) == 0 {
		return fmt.Errorf("no aprs targets provided")
	}

	payload := prepareAprsPayload(body, title)

	conn, err := a.openConnection()
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()

	if err := a.login(conn); err != nil {
		return err
	}

	for _, target := range a.targets {
		buffer := fmt.Sprintf("%s>%s::%-9s:%s", a.user, aprsDeviceID, target, payload)
		if _, err := conn.Write([]byte(buffer)); err != nil {
			return err
		}
	}

	return nil
}

func (a *AprsTarget) openConnection() (net.Conn, error) {
	host, port, err := aprsServerForLocale(a.locale)
	if err != nil {
		return nil, err
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := net.Dialer{Timeout: aprsTimeout}
	return dialer.Dial("tcp", addr)
}

func (a *AprsTarget) login(conn net.Conn) error {
	login := fmt.Sprintf("user %s pass %s vers apprise %s\r\n", a.user, a.password, version.UpstreamVersion)
	if _, err := conn.Write([]byte(login)); err != nil {
		return err
	}

	if err := conn.SetReadDeadline(time.Now().Add(aprsTimeout)); err != nil {
		return err
	}
	reader := bufio.NewReader(conn)
	first, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("aprs login response failed: %w", err)
	}
	second, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("aprs login response failed: %w", err)
	}

	lines := []string{strings.TrimRight(first, "\r\n"), strings.TrimRight(second, "\r\n")}
	if len(lines) < 2 {
		return fmt.Errorf("aprs login response too short")
	}

	parts := strings.SplitN(lines[1], " ", 5)
	if len(parts) < 5 {
		return fmt.Errorf("aprs login response invalid")
	}

	callsign := strings.TrimSpace(parts[2])
	status := strings.TrimSpace(parts[3])
	if strings.ToUpper(callsign) != a.user {
		return fmt.Errorf("aprs login callsign mismatch")
	}
	if strings.HasPrefix(strings.ToLower(status), "unverified") {
		return fmt.Errorf("aprs login rejected")
	}

	return nil
}

func prepareAprsPayload(body, title string) string {
	title = strings.TrimSpace(title)
	body = strings.TrimRight(body, "\r\n")
	payload := mergeTitleBody(title, body)
	payload = aprsBadCharRegex.ReplaceAllString(payload, "")
	payload = aprsBadCharReplacer.Replace(payload)

	runes := []rune(payload)
	if len(runes) > aprsBodyMaxLength {
		payload = string(runes[:aprsBodyMaxLength])
	}

	payload = strings.TrimRight(payload, "\r\n") + "\r\n"
	return payload
}

func parseAprsCallSigns(values []string) []string {
	out := []string{}
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		out = append(out, parseDelimitedList(raw)...)
	}
	return out
}

func isAprsCallSign(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	return aprsCallSignRegex.MatchString(raw)
}

func isNumericString(raw string) bool {
	if raw == "" {
		return false
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func aprsServerForLocale(locale string) (string, int, error) {
	host, ok := aprsLocales[locale]
	if !ok {
		return "", 0, fmt.Errorf("unsupported aprs locale")
	}

	if override := strings.TrimSpace(os.Getenv("APPRISE_APRS_TEST_HOST")); override != "" {
		host = override
	}

	port := aprsDefaultPort
	if override := strings.TrimSpace(os.Getenv("APPRISE_APRS_TEST_PORT")); override != "" {
		if parsed, err := strconv.Atoi(override); err == nil && parsed > 0 {
			port = parsed
		}
	}

	return host, port, nil
}
