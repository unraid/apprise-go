package notify

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

var notifySendOnce sync.Once
var notifySendPath string
var notifySendErr error

type LocalNotifyTarget struct {
	schema       string
	urgency      int
	includeImage bool
	xAxis        *int
	yAxis        *int
}

func NewLocalNotifyTarget(target *ParsedURL) (*LocalNotifyTarget, error) {
	if target == nil {
		return nil, fmt.Errorf("missing target")
	}

	schema := strings.ToLower(strings.TrimSpace(target.Scheme))
	switch schema {
	case "dbus", "kde", "qt", "glib", "gio", "gnome":
	default:
		return nil, fmt.Errorf("invalid schema")
	}

	if !localNotifySupported() {
		return nil, fmt.Errorf("local notifications are not supported")
	}

	urgency := 1
	if raw := strings.TrimSpace(target.Query["urgency"]); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			urgency = parsed
		}
	}
	if raw := strings.TrimSpace(target.Query["priority"]); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			urgency = parsed
		}
	}

	includeImage := parseBoolWithDefault(target.Query["image"], true)

	var xAxis *int
	if raw := strings.TrimSpace(target.Query["x"]); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			xAxis = &parsed
		}
	}
	var yAxis *int
	if raw := strings.TrimSpace(target.Query["y"]); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			yAxis = &parsed
		}
	}

	return &LocalNotifyTarget{
		schema:       schema,
		urgency:      urgency,
		includeImage: includeImage,
		xAxis:        xAxis,
		yAxis:        yAxis,
	}, nil
}

func (l *LocalNotifyTarget) Send(body, title string, notifyType NotifyType) error {
	path, err := lookupNotifySend()
	if err != nil {
		return err
	}

	notifyTitle, notifyBody := l.prepareMessage(title, body)
	if notifyTitle == "" && notifyBody == "" {
		return fmt.Errorf("empty notification")
	}

	args := []string{}
	if urgency := notifySendUrgency(l.urgency); urgency != "" {
		args = append(args, "-u", urgency)
	}

	if l.includeImage {
		if imageURL := appriseImageURL(notifyType, "128x128"); imageURL != "" {
			args = append(args, "-i", imageURL)
		}
	}

	if notifyTitle == "" {
		args = append(args, notifyBody)
	} else {
		args = append(args, notifyTitle)
		if notifyBody != "" {
			args = append(args, notifyBody)
		}
	}

	cmd := exec.Command(path, args...)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func (l *LocalNotifyTarget) prepareMessage(title, body string) (string, string) {
	switch l.schema {
	case "gnome":
		if strings.TrimSpace(title) != "" {
			return "", mergeTitleBody(title, body)
		}
	case "dbus", "qt", "kde", "glib", "gio":
		if strings.TrimSpace(title) == "" && strings.TrimSpace(body) != "" {
			return body, ""
		}
	}
	return title, body
}

func notifySendUrgency(value int) string {
	switch value {
	case 0:
		return "low"
	case 2:
		return "critical"
	default:
		return "normal"
	}
}

func localNotifySupported() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if _, err := lookupNotifySend(); err != nil {
		return false
	}
	return true
}

func lookupNotifySend() (string, error) {
	notifySendOnce.Do(func() {
		path, err := exec.LookPath("notify-send")
		if err != nil {
			notifySendErr = err
			return
		}
		notifySendPath = path
	})
	if notifySendPath == "" {
		if notifySendErr == nil {
			notifySendErr = errors.New("notify-send not found")
		}
		return "", notifySendErr
	}
	return notifySendPath, nil
}
