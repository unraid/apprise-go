package notify

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const rsyslogDefaultPort = 514

var syslogFacilityOrder = []string{
	"kern",
	"user",
	"mail",
	"daemon",
	"auth",
	"syslog",
	"lpr",
	"news",
	"uucp",
	"cron",
	"local0",
	"local1",
	"local2",
	"local3",
	"local4",
	"local5",
	"local6",
	"local7",
}

var syslogFacilityMap = map[string]int{
	"kern":   0,
	"user":   8,
	"mail":   16,
	"daemon": 24,
	"auth":   32,
	"syslog": 40,
	"lpr":    48,
	"news":   56,
	"uucp":   64,
	"cron":   72,
	"local0": 128,
	"local1": 136,
	"local2": 144,
	"local3": 152,
	"local4": 160,
	"local5": 168,
	"local6": 176,
	"local7": 184,
}

var syslogPublishMap = map[NotifyType]int{
	NotifyInfo:    6,
	NotifySuccess: 5,
	NotifyFailure: 2,
	NotifyWarning: 4,
}

type RSyslogTarget struct {
	host     string
	port     int
	facility int
	logPID   bool
}

func NewRSyslogTarget(target *ParsedURL) (*RSyslogTarget, error) {
	if target == nil {
		return nil, fmt.Errorf("missing target")
	}

	host := strings.TrimSpace(target.Host)
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}

	port := target.Port
	if !target.HasPort {
		port = rsyslogDefaultPort
	}

	facility := ""
	if raw := strings.TrimSpace(target.Query["facility"]); raw != "" {
		facility = strings.ToLower(raw)
	} else if entries := splitPath(target.Path); len(entries) > 0 {
		facility = strings.ToLower(strings.TrimSpace(entries[len(entries)-1]))
	}

	if facility == "" {
		facility = "user"
	}

	facilityValue, ok := syslogFacilityMap[facility]
	if !ok {
		for _, key := range syslogFacilityOrder {
			if strings.HasPrefix(key, facility) {
				facilityValue = syslogFacilityMap[key]
				ok = true
				break
			}
		}
	}
	if !ok {
		return nil, fmt.Errorf("invalid facility")
	}

	logPID := parseBoolWithDefault(target.Query["logpid"], true)

	return &RSyslogTarget{
		host:     host,
		port:     port,
		facility: facilityValue,
		logPID:   logPID,
	}, nil
}

func (r *RSyslogTarget) Send(body, title string, notifyType NotifyType) error {
	message := body
	if strings.TrimSpace(title) != "" {
		message = title + ": " + body
	}

	priority, ok := syslogPublishMap[notifyType]
	if !ok {
		return fmt.Errorf("invalid notify type")
	}
	priority += r.facility * 8

	payload := ""
	if r.logPID {
		payload = fmt.Sprintf("<%d>- %d %s", priority, os.Getpid(), message)
	} else {
		payload = fmt.Sprintf("<%d>- %s", priority, message)
	}

	addr := net.JoinHostPort(r.host, strconv.Itoa(r.port))
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()

	n, err := conn.Write([]byte(payload))
	if err != nil {
		return err
	}
	if n < len(payload) {
		return fmt.Errorf("rsyslog sent %d byte(s) but intended %d", n, len(payload))
	}
	return nil
}
