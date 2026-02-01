//go:build windows || plan9 || js || wasip1

package notify

import "fmt"

type SyslogTarget struct{}

func NewSyslogTarget(target *ParsedURL) (*SyslogTarget, error) {
	return nil, fmt.Errorf("syslog is not supported on this platform")
}

func (s *SyslogTarget) Send(body, title string, notifyType NotifyType) error {
	return fmt.Errorf("syslog is not supported on this platform")
}
