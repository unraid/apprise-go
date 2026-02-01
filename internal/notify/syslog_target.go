//go:build !windows && !plan9 && !js && !wasip1

package notify

import (
	"fmt"
	"log/syslog"
	"os"
	"strings"
)

type SyslogTarget struct {
	facility  syslog.Priority
	logPID    bool
	logPerror bool
}

func NewSyslogTarget(target *ParsedURL) (*SyslogTarget, error) {
	if target == nil {
		return nil, fmt.Errorf("missing target")
	}
	if strings.ToLower(strings.TrimSpace(target.Scheme)) != "syslog" {
		return nil, fmt.Errorf("invalid schema")
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
	logPerror := parseBoolWithDefault(target.Query["logperror"], false)

	return &SyslogTarget{
		facility:  syslog.Priority(facilityValue),
		logPID:    logPID,
		logPerror: logPerror,
	}, nil
}

func (s *SyslogTarget) Send(body, title string, notifyType NotifyType) error {
	message := body
	if strings.TrimSpace(title) != "" {
		message = title + ": " + body
	}

	priority, ok := syslogPublishMap[notifyType]
	if !ok {
		return fmt.Errorf("invalid notify type")
	}

	writer, err := syslog.New(s.facility|syslog.Priority(priority), "apprise")
	if err != nil {
		return err
	}
	defer func() {
		_ = writer.Close()
	}()

	if _, err := writer.Write([]byte(message)); err != nil {
		return err
	}
	if s.logPerror {
		_, _ = fmt.Fprintln(os.Stderr, message)
	}

	return nil
}
