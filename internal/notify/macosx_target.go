package notify

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var macosxNotifyPaths = []string{
	"/opt/homebrew/bin/terminal-notifier",
	"/usr/local/bin/terminal-notifier",
	"/usr/bin/terminal-notifier",
	"/bin/terminal-notifier",
	"/opt/local/bin/terminal-notifier",
}

type MacOSXTarget struct {
	includeImage bool
	clickURL     string
	sound        string
	notifyPath   string
	appleScript  string
}

func NewMacOSXTarget(target *ParsedURL) (*MacOSXTarget, error) {
	if target == nil {
		return nil, fmt.Errorf("missing target")
	}
	if strings.ToLower(strings.TrimSpace(target.Scheme)) != "macosx" {
		return nil, fmt.Errorf("invalid schema")
	}
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("macosx notifications not supported")
	}

	includeImage := parseBoolWithDefault(target.Query["image"], true)
	clickURL := strings.TrimSpace(target.Query["click"])
	sound := strings.TrimSpace(target.Query["sound"])

	notifyPath := ""
	for _, candidate := range macosxNotifyPaths {
		if candidate == "" {
			continue
		}
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Mode()&0111 != 0 {
			notifyPath = candidate
			break
		}
	}

	appleScript := ""
	if notifyPath == "" && parseBoolWithDefault(os.Getenv("APPRISE_GO_MACOSX_USE_OSASCRIPT"), false) {
		if path, err := exec.LookPath("osascript"); err == nil {
			appleScript = path
		}
	}

	return &MacOSXTarget{
		includeImage: includeImage,
		clickURL:     clickURL,
		sound:        sound,
		notifyPath:   notifyPath,
		appleScript:  appleScript,
	}, nil
}

func (m *MacOSXTarget) Send(body, title string, notifyType NotifyType) error {
	if m.notifyPath != "" {
		return m.sendTerminalNotifier(body, title, notifyType)
	}
	if m.appleScript != "" {
		return m.sendAppleScript(body, title)
	}
	return fmt.Errorf("macosx notifier not available")
}

func (m *MacOSXTarget) sendTerminalNotifier(body, title string, notifyType NotifyType) error {
	args := []string{
		"-message", body,
	}
	if strings.TrimSpace(title) != "" {
		args = append(args, "-title", title)
	}
	if strings.TrimSpace(m.clickURL) != "" {
		args = append(args, "-open", m.clickURL)
	}
	if strings.TrimSpace(m.sound) != "" {
		args = append(args, "-sound", m.sound)
	}
	if m.includeImage {
		if imageURL := appriseImageURL(notifyType, "128x128"); imageURL != "" {
			args = append(args, "-appIcon", imageURL)
		}
	}

	cmd := exec.Command(m.notifyPath, args...)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func (m *MacOSXTarget) sendAppleScript(body, title string) error {
	escapedBody := escapeAppleScriptString(body)
	escapedTitle := escapeAppleScriptString(title)

	if strings.TrimSpace(escapedTitle) == "" {
		escapedTitle = "Apprise"
	}

	script := fmt.Sprintf("display notification \"%s\" with title \"%s\"", escapedBody, escapedTitle)
	if strings.TrimSpace(m.sound) != "" {
		script = fmt.Sprintf("%s sound name \"%s\"", script, escapeAppleScriptString(m.sound))
	}

	cmd := exec.Command(m.appleScript, "-e", script)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func escapeAppleScriptString(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
