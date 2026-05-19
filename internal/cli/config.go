package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
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

type parsedConfig struct {
	URLs   []taggedURL
	Groups map[string][]string
}

type textConfigLine struct {
	URLs      []taggedURL
	Groups    []string
	GroupTags []string
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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if isYAMLConfigPath(path) {
		parsed, validYAML := parseYAMLConfigWithStatus(data)
		if !validYAML {
			return nil
		}
		if len(parsed.URLs) > 0 || len(parsed.Groups) > 0 {
			return applyTagGroups(parsed.URLs, parsed.Groups)
		}
	}
	parsed := parseTextConfig(string(data))
	return applyTagGroups(parsed.URLs, parsed.Groups)
}

func parseTextConfig(raw string) parsedConfig {
	cfg := parsedConfig{
		URLs:   []taggedURL{},
		Groups: map[string][]string{},
	}

	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		parsed := parseTextConfigLine(scanner.Text())
		for _, group := range parsed.Groups {
			cfg.Groups[group] = append(cfg.Groups[group], parsed.GroupTags...)
		}
		cfg.URLs = append(cfg.URLs, parsed.URLs...)
	}
	if err := scanner.Err(); err != nil {
		return cfg
	}
	return cfg
}

func parseYAMLConfig(data []byte) parsedConfig {
	parsed, _ := parseYAMLConfigWithStatus(data)
	return parsed
}

func parseYAMLConfigWithStatus(data []byte) (parsedConfig, bool) {
	var root any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return parsedConfig{URLs: []taggedURL{}, Groups: map[string][]string{}}, false
	}

	rootMap, ok := asStringMap(root)
	if !ok {
		return parsedConfig{URLs: []taggedURL{}, Groups: map[string][]string{}}, true
	}

	globalTags := parseYAMLTags(rootMap)
	cfg := parsedConfig{
		URLs:   parseYAMLURLs(rootMap["urls"], globalTags),
		Groups: parseYAMLGroups(rootMap["groups"]),
	}
	return cfg, true
}

func parseTextConfigLine(line string) textConfigLine {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
		return textConfigLine{}
	}

	if groups, tags, ok := parseGroupAssignment(line); ok {
		return textConfigLine{Groups: groups, GroupTags: tags}
	}
	if strings.Contains(line, ":") && !urlSchemeRe.MatchString(line) {
		return textConfigLine{}
	}

	return textConfigLine{URLs: parseTaggedLine(line)}
}

func parseTaggedLine(line string) []taggedURL {
	var tags []string
	raw := line

	if strings.Contains(line, "=") {
		parts := strings.SplitN(line, "=", 2)
		tagPart := strings.TrimSpace(parts[0])
		if tagPart != "" && !urlSchemeRe.MatchString(tagPart) {
			raw = strings.TrimSpace(parts[1])
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

func parseGroupAssignment(line string) ([]string, []string, bool) {
	if !strings.Contains(line, "=") {
		return nil, nil, false
	}
	parts := strings.SplitN(line, "=", 2)
	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])
	if left == "" || right == "" || urlSchemeRe.MatchString(left) || urlSchemeRe.MatchString(right) {
		return nil, nil, false
	}
	groups := parseTags(left)
	tags := parseTags(right)
	if len(groups) == 0 || len(tags) == 0 {
		return nil, nil, false
	}
	return groups, tags, true
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

func applyTagGroups(urls []taggedURL, groups map[string][]string) []taggedURL {
	if len(urls) == 0 || len(groups) == 0 {
		return urls
	}

	expandedGroups := normalizeTagGroups(groups)
	for idx := range urls {
		tagSet := make(map[string]struct{}, len(urls[idx].Tags)+len(expandedGroups))
		for _, tag := range urls[idx].Tags {
			if tag == "" {
				continue
			}
			tagSet[strings.ToLower(tag)] = struct{}{}
		}
		for group, tags := range expandedGroups {
			if len(tags) == 0 {
				continue
			}
			for tag := range tagSet {
				if _, ok := tags[tag]; ok {
					tagSet[group] = struct{}{}
					break
				}
			}
		}
		urls[idx].Tags = sortedTagSet(tagSet)
	}
	return urls
}

func normalizeTagGroups(groups map[string][]string) map[string]map[string]struct{} {
	groupNames := make(map[string]struct{}, len(groups))
	for group := range groups {
		groupNames[group] = struct{}{}
	}

	expanded := make(map[string]map[string]struct{}, len(groups))
	var expand func(group string, seen map[string]struct{}) map[string]struct{}
	expand = func(group string, seen map[string]struct{}) map[string]struct{} {
		if tags, ok := expanded[group]; ok {
			return tags
		}
		if _, ok := seen[group]; ok {
			return map[string]struct{}{}
		}
		seen[group] = struct{}{}
		tags := map[string]struct{}{}
		for _, rawTag := range groups[group] {
			tag := strings.ToLower(strings.TrimSpace(rawTag))
			if tag == "" || tag == group {
				continue
			}
			if _, isGroup := groupNames[tag]; isGroup {
				for expandedTag := range expand(tag, cloneTagSet(seen)) {
					tags[expandedTag] = struct{}{}
				}
				continue
			}
			tags[tag] = struct{}{}
		}
		expanded[group] = tags
		return tags
	}

	for group := range groups {
		expand(group, map[string]struct{}{})
	}
	return expanded
}

func cloneTagSet(tags map[string]struct{}) map[string]struct{} {
	cloned := make(map[string]struct{}, len(tags))
	for tag := range tags {
		cloned[tag] = struct{}{}
	}
	return cloned
}

func sortedTagSet(tagSet map[string]struct{}) []string {
	if len(tagSet) == 0 {
		return nil
	}
	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

func isYAMLConfigPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func parseYAMLURLs(value any, globalTags []string) []taggedURL {
	switch urls := value.(type) {
	case []any:
		parsed := []taggedURL{}
		for _, entry := range urls {
			parsed = append(parsed, parseYAMLURLEntry(entry, globalTags)...)
		}
		return parsed
	case map[string]any:
		parsed := []taggedURL{}
		for key, options := range urls {
			if !urlSchemeRe.MatchString(key) {
				continue
			}
			parsed = append(parsed, parseYAMLURLMappedOptions(key, options, globalTags)...)
		}
		return parsed
	default:
		return nil
	}
}

func parseYAMLURLEntry(entry any, globalTags []string) []taggedURL {
	switch value := entry.(type) {
	case string:
		urls := detectURLs(value)
		parsed := make([]taggedURL, 0, len(urls))
		for _, url := range urls {
			parsed = append(parsed, taggedURL{URL: url, Tags: mergeTags(globalTags, nil)})
		}
		return parsed
	case map[string]any:
		if rawURL, ok := value["url"]; ok {
			return parseYAMLURLValue(rawURL, mergeTags(globalTags, parseYAMLTags(value)))
		}
		if rawURL, ok := value["urls"]; ok {
			return parseYAMLURLValue(rawURL, mergeTags(globalTags, parseYAMLTags(value)))
		}
		parsed := []taggedURL{}
		for key, options := range value {
			if !urlSchemeRe.MatchString(key) {
				continue
			}
			parsed = append(parsed, parseYAMLURLMappedOptions(key, options, globalTags)...)
		}
		return parsed
	default:
		return nil
	}
}

func parseYAMLURLMappedOptions(rawURL string, options any, globalTags []string) []taggedURL {
	url := strings.TrimSpace(rawURL)
	if entries, ok := options.([]any); ok {
		parsed := []taggedURL{}
		for _, entry := range entries {
			entryMap, ok := asStringMap(entry)
			if !ok {
				continue
			}
			parsed = append(parsed, taggedURL{
				URL:  url,
				Tags: mergeTags(globalTags, parseYAMLTags(entryMap)),
			})
		}
		if len(parsed) > 0 {
			return parsed
		}
	}

	return []taggedURL{{
		URL:  url,
		Tags: mergeTags(globalTags, parseYAMLTagsFromOptions(options)),
	}}
}

func parseYAMLURLValue(value any, tags []string) []taggedURL {
	switch urls := value.(type) {
	case string:
		detected := detectURLs(urls)
		parsed := make([]taggedURL, 0, len(detected))
		for _, url := range detected {
			parsed = append(parsed, taggedURL{URL: url, Tags: mergeTags(tags, nil)})
		}
		return parsed
	case []any:
		parsed := []taggedURL{}
		for _, entry := range urls {
			parsed = append(parsed, parseYAMLURLValue(entry, tags)...)
		}
		return parsed
	default:
		return nil
	}
}

func parseYAMLGroups(value any) map[string][]string {
	groups := map[string][]string{}
	switch entries := value.(type) {
	case map[string]any:
		for group, rawTags := range entries {
			for _, parsedGroup := range parseTags(group) {
				groups[parsedGroup] = append(groups[parsedGroup], parseYAMLGroupTags(rawTags)...)
			}
		}
	case []any:
		for _, entry := range entries {
			entryMap, ok := asStringMap(entry)
			if !ok {
				continue
			}
			for group, rawTags := range entryMap {
				for _, parsedGroup := range parseTags(group) {
					groups[parsedGroup] = append(groups[parsedGroup], parseYAMLGroupTags(rawTags)...)
				}
			}
		}
	}
	return groups
}

func parseYAMLGroupTags(value any) []string {
	switch tags := value.(type) {
	case nil:
		return nil
	case string:
		return parseTags(tags)
	case int, int64, float64, bool:
		return parseTags(fmt.Sprint(tags))
	case []any:
		parsed := []string{}
		for _, entry := range tags {
			entryMap, ok := asStringMap(entry)
			if ok {
				for tag := range entryMap {
					parsed = append(parsed, parseTags(tag)...)
				}
				continue
			}
			parsed = append(parsed, parseYAMLGroupTags(entry)...)
		}
		return parsed
	case map[string]any:
		parsed := []string{}
		for tag := range tags {
			parsed = append(parsed, parseTags(tag)...)
		}
		return parsed
	default:
		return parseTags(fmt.Sprint(tags))
	}
}

func parseYAMLTags(value map[string]any) []string {
	if rawTags, ok := value["tag"]; ok {
		return parseYAMLTagValue(rawTags)
	}
	if rawTags, ok := value["tags"]; ok {
		return parseYAMLTagValue(rawTags)
	}
	return nil
}

func parseYAMLTagsFromOptions(options any) []string {
	switch value := options.(type) {
	case map[string]any:
		return parseYAMLTags(value)
	case []any:
		tags := []string{}
		for _, entry := range value {
			if entryMap, ok := asStringMap(entry); ok {
				tags = append(tags, parseYAMLTags(entryMap)...)
			}
		}
		return tags
	default:
		return nil
	}
}

func parseYAMLTagValue(value any) []string {
	switch tags := value.(type) {
	case nil:
		return nil
	case string:
		return parseTags(tags)
	case int, int64, float64, bool:
		return parseTags(fmt.Sprint(tags))
	case []any:
		parsed := []string{}
		for _, entry := range tags {
			parsed = append(parsed, parseYAMLTagValue(entry)...)
		}
		return parsed
	default:
		return parseTags(fmt.Sprint(tags))
	}
}

func asStringMap(value any) (map[string]any, bool) {
	if typed, ok := value.(map[string]any); ok {
		return typed, true
	}
	return nil, false
}

func mergeTags(groups ...[]string) []string {
	tagSet := map[string]struct{}{}
	for _, group := range groups {
		for _, tag := range group {
			if tag == "" {
				continue
			}
			tagSet[strings.ToLower(tag)] = struct{}{}
		}
	}
	return sortedTagSet(tagSet)
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
