package notify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Persistent storage exists so a notifier can remember something between
// sends. Matrix needs it most: without somewhere to keep its Olm account and
// device id, every notification would register a fresh device, recipients
// would see a new unverified one each time, and nothing sent under a previous
// identity would stay readable.
//
// The on-disk layout matches what the storage CLI already walks —
// <path>/<url-id>/cache.psdata — so `apprise storage list|prune|clear` sees
// what providers write here.

// Store is the small slice of upstream's persistent store that providers use.
type Store interface {
	// Get returns the raw JSON for a key, or false when it is absent or has
	// expired.
	Get(key string) (json.RawMessage, bool)
	// Set records a value. A zero ttl means it never expires.
	Set(key string, value any, ttl time.Duration) error
	// Clear removes the named keys, or every key when none are named.
	Clear(keys ...string) error
}

type storeEntry struct {
	Value   json.RawMessage `json:"value"`
	Expires *time.Time      `json:"expires,omitempty"`
}

// memoryStore keeps entries for the life of the process. It is what a
// provider gets when no storage path is configured, which keeps every caller
// free of nil checks.
type memoryStore struct {
	mu      sync.Mutex
	entries map[string]storeEntry
}

func newMemoryStore() *memoryStore {
	return &memoryStore{entries: map[string]storeEntry{}}
}

func (m *memoryStore) Get(key string) (json.RawMessage, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.entries[key]
	if !ok {
		return nil, false
	}
	if entry.Expires != nil && !storeNow().Before(*entry.Expires) {
		delete(m.entries, key)
		return nil, false
	}

	return entry.Value, true
}

func (m *memoryStore) Set(key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	entry := storeEntry{Value: data}
	if ttl > 0 {
		expires := storeNow().Add(ttl)
		entry.Expires = &expires
	}
	m.entries[key] = entry

	return nil
}

func (m *memoryStore) Clear(keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(keys) == 0 {
		m.entries = map[string]storeEntry{}
		return nil
	}
	for _, key := range keys {
		delete(m.entries, key)
	}

	return nil
}

// diskStore persists to a single JSON file per namespace, read on first use
// and rewritten whenever it changes. Notifications are infrequent enough that
// the simplicity is worth more than incremental writes.
type diskStore struct {
	mu      sync.Mutex
	path    string
	loaded  bool
	entries map[string]storeEntry
}

func newDiskStore(path string) *diskStore {
	return &diskStore{path: path, entries: map[string]storeEntry{}}
}

func (d *diskStore) load() {
	if d.loaded {
		return
	}
	d.loaded = true

	data, err := os.ReadFile(d.path)
	if err != nil {
		// A missing or unreadable file is an empty store, not an error; the
		// caller's job is to notify, not to manage a cache.
		return
	}

	entries := map[string]storeEntry{}
	if err := json.Unmarshal(data, &entries); err != nil {
		return
	}
	d.entries = entries
}

func (d *diskStore) flush() error {
	if err := os.MkdirAll(filepath.Dir(d.path), 0o700); err != nil {
		return err
	}

	data, err := json.Marshal(d.entries)
	if err != nil {
		return err
	}

	// Write through a temporary file so a crash cannot leave a half-written
	// cache behind.
	temp := d.path + ".tmp"
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return err
	}

	return os.Rename(temp, d.path)
}

func (d *diskStore) Get(key string) (json.RawMessage, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.load()

	entry, ok := d.entries[key]
	if !ok {
		return nil, false
	}
	if entry.Expires != nil && !storeNow().Before(*entry.Expires) {
		delete(d.entries, key)
		_ = d.flush()

		return nil, false
	}

	return entry.Value, true
}

func (d *diskStore) Set(key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.load()

	entry := storeEntry{Value: data}
	if ttl > 0 {
		expires := storeNow().Add(ttl)
		entry.Expires = &expires
	}
	d.entries[key] = entry

	return d.flush()
}

func (d *diskStore) Clear(keys ...string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.load()

	if len(keys) == 0 {
		d.entries = map[string]storeEntry{}
	} else {
		for _, key := range keys {
			delete(d.entries, key)
		}
	}

	return d.flush()
}

// storeNow is indirected so tests can pin expiry.
var storeNow = time.Now

var (
	storeMu        sync.Mutex
	storeRoot      string
	storeUIDLength = 8
	storeSalt      []byte
	storeCache     = map[string]Store{}
)

// ConfigureStorage points persistent storage at a directory. An empty path
// keeps everything in memory, which is the default.
func ConfigureStorage(root string, uidLength int, salt []byte) {
	storeMu.Lock()
	defer storeMu.Unlock()

	storeRoot = strings.TrimSpace(root)
	if uidLength >= 2 {
		storeUIDLength = uidLength
	}
	storeSalt = salt
	// Namespaces are derived from the settings, so cached handles from the
	// previous configuration no longer apply.
	storeCache = map[string]Store{}
}

// StoreFor returns the store for a URL. The same URL always gets the same
// store, so two notifiers built from one URL share what they remember.
func StoreFor(target *ParsedURL) Store {
	storeMu.Lock()
	defer storeMu.Unlock()

	uid := URLID(target, storeUIDLength, storeSalt)
	if uid == "" {
		return newMemoryStore()
	}

	if existing, ok := storeCache[uid]; ok {
		return existing
	}

	var store Store
	if storeRoot == "" {
		store = newMemoryStore()
	} else {
		store = newDiskStore(filepath.Join(expandStorePath(storeRoot), uid, "cache.psdata"))
	}
	storeCache[uid] = store

	return store
}

// expandStorePath resolves a leading ~ so the CLI's default path works
// without the shell having expanded it.
func expandStorePath(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
		}
	}

	return path
}

// storeGetJSON reads a key into out, reporting whether it was present and
// well-formed.
func storeGetJSON(store Store, key string, out any) bool {
	raw, ok := store.Get(key)
	if !ok {
		return false
	}

	return json.Unmarshal(raw, out) == nil
}
