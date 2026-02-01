package notify

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type WindowsTarget struct {
	includeImage bool
	durationSec  int
}

func NewWindowsTarget(target *ParsedURL) (*WindowsTarget, error) {
	if target == nil {
		return nil, fmt.Errorf("missing target")
	}
	if strings.ToLower(strings.TrimSpace(target.Scheme)) != "windows" {
		return nil, fmt.Errorf("invalid schema")
	}
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("windows notifications not supported")
	}

	includeImage := parseBoolWithDefault(target.Query["image"], true)
	duration := 12
	if raw := strings.TrimSpace(target.Query["duration"]); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			duration = parsed
		}
	}

	return &WindowsTarget{
		includeImage: includeImage,
		durationSec:  duration,
	}, nil
}

func (w *WindowsTarget) Send(body, title string, notifyType NotifyType) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("windows notifications not supported")
	}

	if strings.TrimSpace(title) == "" {
		title = "Apprise"
	}

	duration := "short"
	if w.durationSec >= 8 {
		duration = "long"
	}

	escapedTitle := escapeWindowsXML(title)
	escapedBody := escapeWindowsXML(body)

	script := fmt.Sprintf(`
$ErrorActionPreference = "Stop"
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] > $null
$toastXml = @"
<toast duration="%s">
  <visual>
    <binding template="ToastGeneric">
      <text>%s</text>
      <text>%s</text>
    </binding>
  </visual>
</toast>
"@
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($toastXml)
$toast = New-Object Windows.UI.Notifications.ToastNotification $xml
$notifier = [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("Apprise")
$notifier.Show($toast)
`, duration, escapedTitle, escapedBody)

	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func escapeWindowsXML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}
