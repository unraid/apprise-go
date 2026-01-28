package cli

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	defaultEnvAppriseURLs        = "APPRISE_URLS"
	defaultEnvAppriseConfigPath  = "APPRISE_CONFIG_PATH"
	defaultEnvAppriseConfigAlias = "APPRISE_CONFIG"
	defaultEnvAppriseStoragePath = "APPRISE_STORAGE_PATH"
)

var (
	urlSchemeRe   = regexp.MustCompile(`(?i)[a-z0-9]{1,32}://`)
	splitDelimsRe = regexp.MustCompile(`[\[\];,\s]+`)
)

type taggedURL struct {
	URL  string
	Tags []string
}

func loadTaggedURLs(configPaths []string) []taggedURL {
	urls := []taggedURL{}
	for _, path := range configPaths {
		path = expandPath(path)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		fileURLs := parseConfigFile(path)
		if len(fileURLs) > 0 {
			urls = append(urls, fileURLs...)
		}
	}
	return urls
}

func parseConfigFile(path string) []taggedURL {
	handle, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer handle.Close()

	var urls []taggedURL
	scanner := bufio.NewScanner(handle)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if parsed := parseTaggedLine(line); len(parsed) > 0 {
			urls = append(urls, parsed...)
		}
	}
	if err := scanner.Err(); err != nil {
		return urls
	}
	return urls
}

func parseTaggedLine(line string) []taggedURL {
	var tags []string
	raw := line

	if strings.Contains(line, "=") {
		parts := strings.SplitN(line, "=", 2)
		tagPart := strings.TrimSpace(parts[0])
		raw = strings.TrimSpace(parts[1])
		if tagPart != "" && !strings.Contains(tagPart, "://") {
			tags = parseTags(tagPart)
		}
	}

	urls := detectURLs(raw)
	if len(urls) == 0 {
		return nil
	}

	result := make([]taggedURL, 0, len(urls))
	for _, url := range urls {
		result = append(result, taggedURL{
			URL:  url,
			Tags: tags,
		})
	}
	return result
}

func detectURLs(raw string) []string {
	matches := urlSchemeRe.FindAllStringIndex(raw, -1)
	if len(matches) > 0 {
		urls := make([]string, 0, len(matches))
		for idx, match := range matches {
			start := match[0]
			end := len(raw)
			if idx+1 < len(matches) {
				end = matches[idx+1][0]
			}
			chunk := strings.TrimSpace(raw[start:end])
			chunk = strings.TrimRight(chunk, ",")
			if chunk == "" {
				continue
			}
			urls = append(urls, chunk)
		}
		return urls
	}
	parts := splitDelimsRe.Split(raw, -1)
	urls := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		urls = append(urls, part)
	}
	return urls
}

func parseTags(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag == "" {
			continue
		}
		tags = append(tags, strings.ToLower(tag))
	}
	return tags
}

func parseTagFilters(values []string) [][]string {
	if len(values) == 0 {
		return nil
	}

	filters := make([][]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parts := parseTags(value)
		if len(parts) == 0 {
			continue
		}
		filters = append(filters, parts)
	}
	return filters
}

func filterTaggedURLs(urls []taggedURL, tagFilters [][]string) []taggedURL {
	if len(tagFilters) == 0 {
		return urls
	}

	filtered := make([]taggedURL, 0, len(urls))
	for _, entry := range urls {
		if matchesTagFilters(entry.Tags, tagFilters) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func matchesTagFilters(tags []string, filters [][]string) bool {
	if len(filters) == 0 {
		return true
	}
	if len(tags) == 0 {
		return false
	}

	tagSet := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tagSet[strings.ToLower(tag)] = struct{}{}
	}

	for _, filter := range filters {
		if len(filter) == 0 {
			continue
		}
		if len(filter) == 1 && strings.EqualFold(filter[0], "all") {
			return true
		}
		allMatch := true
		for _, tag := range filter {
			if _, ok := tagSet[strings.ToLower(tag)]; !ok {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
	}
	return false
}

func loadConfigPaths(explicit []string) []string {
	if len(explicit) > 0 {
		return expandPaths(explicit)
	}

	if raw := strings.TrimSpace(os.Getenv(defaultEnvAppriseConfigPath)); raw != "" {
		return expandPaths(splitPathList(raw))
	}

	if raw := strings.TrimSpace(os.Getenv(defaultEnvAppriseConfigAlias)); raw != "" {
		return expandPaths(splitPathList(raw))
	}

	return expandPaths(defaultConfigPaths())
}

func splitPathList(raw string) []string {
	return splitDelimsRe.Split(raw, -1)
}

func expandPaths(paths []string) []string {
	expanded := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		expanded = append(expanded, expandPath(path))
	}
	return expanded
}

func expandPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	path = os.ExpandEnv(path)
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			if path == "~" {
				return home
			}
			if strings.HasPrefix(path, "~/") {
				return filepath.Join(home, strings.TrimPrefix(path, "~/"))
			}
		}
	}
	return path
}

func defaultConfigPaths() []string {
	return []string{
		"~/.apprise",
		"~/.apprise.conf",
		"~/.apprise.yml",
		"~/.apprise.yaml",
		"~/.config/apprise",
		"~/.config/apprise.conf",
		"~/.config/apprise.yml",
		"~/.config/apprise.yaml",
		"~/.apprise/apprise",
		"~/.apprise/apprise.conf",
		"~/.apprise/apprise.yml",
		"~/.apprise/apprise.yaml",
		"~/.config/apprise/apprise",
		"~/.config/apprise/apprise.conf",
		"~/.config/apprise/apprise.yml",
		"~/.config/apprise/apprise.yaml",
		"/etc/apprise",
		"/etc/apprise.yml",
		"/etc/apprise.yaml",
		"/etc/apprise/apprise",
		"/etc/apprise/apprise.conf",
		"/etc/apprise/apprise.yml",
		"/etc/apprise/apprise.yaml",
	}
}
