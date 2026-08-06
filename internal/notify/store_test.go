package notify

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMemoryStoreRoundTrip(t *testing.T) {
	store := newMemoryStore()

	if _, ok := store.Get("missing"); ok {
		t.Fatal("absent key reported present")
	}

	if err := store.Set("device", map[string]string{"id": "ABC"}, 0); err != nil {
		t.Fatalf("set: %v", err)
	}

	var out map[string]string
	if !storeGetJSON(store, "device", &out) {
		t.Fatal("device missing after set")
	}
	if out["id"] != "ABC" {
		t.Fatalf("want ABC, got %s", out["id"])
	}

	if err := store.Clear("device"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, ok := store.Get("device"); ok {
		t.Fatal("cleared key still present")
	}
}

func TestStoreExpiry(t *testing.T) {
	now := time.Unix(1700000000, 0)
	original := storeNow
	t.Cleanup(func() { storeNow = original })
	storeNow = func() time.Time { return now }

	store := newMemoryStore()
	if err := store.Set("token", "value", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, ok := store.Get("token"); !ok {
		t.Fatal("token expired early")
	}

	now = now.Add(time.Minute)
	// The entry expires exactly on the boundary, not after it.
	if _, ok := store.Get("token"); ok {
		t.Fatal("token outlived its ttl")
	}
}

// TestDiskStorePersistsAcrossHandles is the property Matrix depends on: a
// second notifier built later must see what the first one wrote, or it would
// register a new device every send.
func TestDiskStorePersistsAcrossHandles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "uid", "cache.psdata")

	first := newDiskStore(path)
	if err := first.Set("device_id", "ABCDEF", 0); err != nil {
		t.Fatalf("set: %v", err)
	}

	second := newDiskStore(path)
	var deviceID string
	if !storeGetJSON(second, "device_id", &deviceID) {
		t.Fatal("device id did not survive a fresh handle")
	}
	if deviceID != "ABCDEF" {
		t.Fatalf("want ABCDEF, got %s", deviceID)
	}
}

func TestStoreForIsStableAndMemoryByDefault(t *testing.T) {
	t.Cleanup(func() { ConfigureStorage("", 8, nil) })
	ConfigureStorage("", 8, nil)

	parsed, err := ParseURL("matrix://user:pass@matrix.example.com/%23room:example.com")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	// Two notifiers built from one URL have to share, or neither sees what
	// the other stored.
	if StoreFor(parsed) != StoreFor(parsed) {
		t.Fatal("same url handed two different stores")
	}
}
