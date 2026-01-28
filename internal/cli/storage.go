package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/unraid/apprise-go/internal/notify"
)

const (
	defaultStoragePath      = "~/.local/share/apprise/cache"
	defaultStoragePruneDays = 30
	defaultStorageUIDLength = 8
	defaultStorageMode      = "auto"
)

const (
	storageActionList  = "list"
	storageActionPrune = "prune"
	storageActionClear = "clear"
)

var (
	storageKeyRe     = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
	storageCacheFile = "cache.psdata"
	storageBackup    = "cache._psbak"
)

type storageEntry struct {
	urls  []taggedURL
	state string
	size  int64
}

func RunStorage(opts *cliOptions, args []string, stdout, stderr io.Writer) int {
	action := storageActionList
	filterUIDs := []string{}
	if len(args) > 1 {
		filterUIDs = args[1:]
	}
	if len(filterUIDs) > 0 {
		if resolved, ok := resolveStorageAction(filterUIDs[0]); ok {
			action = resolved
			filterUIDs = filterUIDs[1:]
		}
	}

	if opts.storageUIDLength < 2 {
		fmt.Fprintln(stderr, "The --storage-uid-length (-SUL) value can not be lower than two (2).")
		return 2
	}
	if opts.storagePruneDays < 0 {
		fmt.Fprintln(stderr, "The --storage-prune-days (-SPD) value can not be lower than zero (0).")
		return 2
	}

	storagePath := resolveStoragePath(opts.storagePath)
	taggedURLs := loadStorageURLs(opts)
	tagFilters := parseTagFilters(opts.tags)
	if len(tagFilters) > 0 {
		taggedURLs = filterTaggedURLs(taggedURLs, tagFilters)
	}

	uids := make(map[string]*storageEntry)
	for _, entry := range taggedURLs {
		parsed, err := notify.ParseURL(entry.URL)
		if err != nil {
			continue
		}
		uid := notify.URLID(parsed, opts.storageUIDLength, nil)
		if uid == "" {
			continue
		}
		if _, ok := uids[uid]; !ok {
			uids[uid] = &storageEntry{
				state: "unused",
				urls:  []taggedURL{},
			}
		}
		uids[uid].urls = append(uids[uid].urls, entry)
	}

	if action == storageActionList {
		detected := diskScan(storagePath, filterUIDs)
		for _, uid := range detected {
			size := dirSize(filepath.Join(storagePath, uid))
			if entry, ok := uids[uid]; ok {
				entry.state = "active"
				entry.size = size
			} else if len(tagFilters) == 0 {
				uids[uid] = &storageEntry{
					state: "stale",
					size:  size,
				}
			}
		}
		printStorageList(stdout, storagePath, uids)
		return 0
	}

	pruneDays := opts.storagePruneDays
	if action == storageActionClear {
		pruneDays = 0
	}
	expiry := time.Duration(pruneDays) * 24 * time.Hour
	if err := diskPrune(storagePath, filterUIDs, expiry, !opts.dryRun); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func resolveStorageAction(raw string) (string, bool) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return "", false
	}
	for _, candidate := range []string{storageActionList, storageActionPrune, storageActionClear} {
		if strings.HasPrefix(candidate, raw) {
			return candidate, true
		}
	}
	return "", false
}

func resolveStoragePath(explicit string) string {
	if envPath := strings.TrimSpace(os.Getenv(defaultEnvAppriseStoragePath)); envPath != "" {
		return expandPath(envPath)
	}
	path := strings.TrimSpace(explicit)
	if path != "" {
		return expandPath(path)
	}
	return expandPath(defaultStoragePath)
}

func loadStorageURLs(opts *cliOptions) []taggedURL {
	if len(opts.configPaths) > 0 {
		return loadTaggedURLs(loadConfigPaths(opts.configPaths))
	}

	if raw := strings.TrimSpace(os.Getenv(defaultEnvAppriseURLs)); raw != "" {
		parsed := parseTaggedLine(raw)
		if len(parsed) > 0 {
			return parsed
		}
	}

	return loadTaggedURLs(loadConfigPaths(nil))
}

func diskScan(path string, namespaces []string) []string {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}

	var filtered []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !storageKeyRe.MatchString(name) {
			continue
		}
		if len(namespaces) == 0 || matchesNamespacePrefix(name, namespaces) {
			filtered = append(filtered, name)
		}
	}
	sort.Strings(filtered)
	return filtered
}

func matchesNamespacePrefix(value string, namespaces []string) bool {
	for _, prefix := range namespaces {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func diskPrune(path string, namespaces []string, expiry time.Duration, doDelete bool) error {
	now := time.Now()
	ids := diskScan(path, namespaces)
	for _, namespace := range ids {
		baseDir := filepath.Join(path, namespace)
		dataDir := filepath.Join(baseDir, "var")
		tempDir := filepath.Join(baseDir, "tmp")

		candidates := []string{
			filepath.Join(baseDir, storageCacheFile),
			filepath.Join(baseDir, storageBackup),
		}
		candidates = append(candidates, listFiles(dataDir)...)
		candidates = append(candidates, listFiles(tempDir)...)

		for _, file := range candidates {
			info, err := os.Stat(file)
			if err != nil {
				continue
			}
			if expiry > 0 && now.Sub(info.ModTime()) < expiry {
				continue
			}
			if doDelete {
				_ = os.Remove(file)
			}
		}
		if doDelete {
			_ = os.Remove(tempDir)
			_ = os.Remove(dataDir)
			_ = os.Remove(baseDir)
		}
	}
	return nil
}

func listFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	files := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	return files
}

func dirSize(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		size += info.Size()
		return nil
	})
	return size
}

func printStorageList(w io.Writer, storagePath string, uids map[string]*storageEntry) {
	type entry struct {
		id   string
		data *storageEntry
	}
	entries := make([]entry, 0, len(uids))
	for id, data := range uids {
		entries = append(entries, entry{id: id, data: data})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].id < entries[j].id
	})

	for idx, item := range entries {
		if idx > 0 {
			fmt.Fprintln(w)
		}
		size := bytesToString(item.data.size)
		line := fmt.Sprintf("%-52s %-8s %s", item.id, size, item.data.state)
		fmt.Fprintf(w, "%4d. %s\n", idx+1, line)

		for _, entry := range item.data.urls {
			url := entry.URL
			url = truncate(url, 80-8)
			fmt.Fprintf(w, "%7s %s\n", "-", url)
			if len(entry.Tags) > 0 {
				fmt.Fprintf(w, "%10s: %s\n", "tags", strings.Join(entry.Tags, ", "))
			}
		}
	}
}

func truncate(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func bytesToString(value int64) string {
	if value < 0 {
		return "0.00B"
	}
	unit := "B"
	size := float64(value)
	if size >= 1024.0 {
		size /= 1024.0
		unit = "KB"
		if size >= 1024.0 {
			size /= 1024.0
			unit = "MB"
			if size >= 1024.0 {
				size /= 1024.0
				unit = "GB"
				if size >= 1024.0 {
					size /= 1024.0
					unit = "TB"
				}
			}
		}
	}
	return fmt.Sprintf("%.2f%s", size, unit)
}
