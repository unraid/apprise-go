package notify

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func matrixRoomCacheTarget(t *testing.T, handler http.HandlerFunc) *MatrixTarget {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	previousClient := http.DefaultClient
	http.DefaultClient = server.Client()
	t.Cleanup(func() {
		http.DefaultClient = previousClient
		server.Close()
	})
	parsed, err := ParseURL("matrixs://token@" + strings.TrimPrefix(server.URL, "https://") + "/!room?discovery=no&e2ee=no")
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewMatrixTarget(parsed)
	if err != nil {
		t.Fatal(err)
	}
	target.store = newMemoryStore()
	target.homeServer = "example.com"
	return target
}

func TestMatrixRoomJoinCachesSuccess(t *testing.T) {
	for _, tt := range []struct {
		name     string
		room     string
		hsreq    bool
		cacheKey string
		joinRoom string
	}{
		{"explicit-id", "!room:example.com", true, "!room:example.com", "!room:example.com"},
		{"bare-id", "!room", true, "!room:example.com", "!room:example.com"},
		{"bare-id-without-hsreq", "!room", false, "!room:example.com", "!room"},
		{"explicit-alias", "#room:example.com", true, "#room:example.com", "#room:example.com"},
		{"bare-alias", "#room", true, "#room:example.com", "#room:example.com"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var joins atomic.Int32
			target := matrixRoomCacheTarget(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/_matrix/client/v3/join/"+tt.joinRoom {
					t.Errorf("unexpected join path %s", r.URL.Path)
				}
				joins.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"room_id":"!joined:example.com"}`)
			})
			target.hsreq = tt.hsreq
			for range 2 {
				if got := target.roomJoin(tt.room); got != "!joined:example.com" {
					t.Fatalf("joined room = %q", got)
				}
			}
			if joins.Load() != 1 {
				t.Fatalf("successful room rejoined: requests=%d", joins.Load())
			}
			var mapping struct {
				ID         string `json:"id"`
				HomeServer string `json:"home_server"`
			}
			if !storeGetJSON(target.store, tt.cacheKey, &mapping) || mapping.ID != "!joined:example.com" || mapping.HomeServer != "example.com" {
				t.Fatalf("successful join mapping missing or incorrect: %+v", mapping)
			}
		})
	}
}

func TestMatrixRoomJoinRetriesFailure(t *testing.T) {
	var joins atomic.Int32
	target := matrixRoomCacheTarget(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if joins.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, `{"error":"join unavailable"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"room_id":"!joined:example.com"}`)
	})
	const room = "!room:example.com"
	if got := target.roomJoin(room); got != "" {
		t.Fatalf("failed join returned a room: %q", got)
	}
	if _, cached := target.store.Get(room); cached {
		t.Fatal("failed join was cached")
	}
	for range 2 {
		if got := target.roomJoin(room); got != "!joined:example.com" {
			t.Fatalf("join did not recover: %q", got)
		}
	}
	if joins.Load() != 2 {
		t.Fatalf("expected one failed join and one successful retry: requests=%d", joins.Load())
	}
}

func TestMatrixRoomJoinCacheSeparatesHomeservers(t *testing.T) {
	var joins atomic.Int32
	target := matrixRoomCacheTarget(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"room_id":"!joined%d:example.com"}`, joins.Add(1))
	})
	target.hsreq = false
	for i, homeServer := range []string{"one.example.com", "two.example.com"} {
		target.homeServer = homeServer
		want := fmt.Sprintf("!joined%d:example.com", i+1)
		for range 2 {
			if got := target.roomJoin("!room"); got != want {
				t.Fatalf("room mapping for %s = %q; want %q", homeServer, got, want)
			}
		}
	}
	if joins.Load() != 2 {
		t.Fatalf("expected one join per homeserver: requests=%d", joins.Load())
	}
}

func TestMatrixRoomCacheSeparatesTokenCredentials(t *testing.T) {
	var mu sync.Mutex
	joins := map[string]int{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/_matrix/client/versions":
			_, _ = fmt.Fprint(w, `{"versions":["r0.6.0"]}`)
		case strings.HasSuffix(r.URL.Path, "/account/whoami"):
			user := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			_, _ = fmt.Fprintf(w, `{"user_id":"@%s:example.com","device_id":"DEVICE"}`, user)
		case strings.Contains(r.URL.Path, "/join/"):
			mu.Lock()
			joins[r.Header.Get("Authorization")]++
			mu.Unlock()
			_, _ = fmt.Fprint(w, `{"room_id":"!joined:example.com"}`)
		case strings.Contains(r.URL.Path, "/send/m.room.message/"):
			_, _ = fmt.Fprint(w, `{"event_id":"$message"}`)
		default:
			t.Errorf("unexpected Matrix request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	previousClient := http.DefaultClient
	http.DefaultClient = server.Client()
	t.Cleanup(func() {
		http.DefaultClient = previousClient
		server.Close()
	})
	authority := strings.TrimPrefix(server.URL, "https://")
	path := "/!room:example.com?discovery=no&e2ee=no"
	build := func(authority, token string) *MatrixTarget {
		t.Helper()
		parsed, err := ParseURL("matrixs://" + authority + path + token)
		if err != nil {
			t.Fatal(err)
		}
		target, err := NewMatrixTarget(parsed)
		if err != nil {
			t.Fatal(err)
		}
		return target
	}
	first := build(authority, "&token=first")
	second := build(authority, "&token=second")
	for _, target := range []*MatrixTarget{first, second} {
		for range 2 {
			if err := target.Send("body", "title", NotifyInfo); err != nil {
				t.Fatal(err)
			}
		}
	}
	mu.Lock()
	firstJoins, secondJoins := joins["Bearer first"], joins["Bearer second"]
	mu.Unlock()
	if firstJoins != 1 || secondJoins != 1 {
		t.Fatalf("each token must join independently once: first=%d second=%d", firstJoins, secondJoins)
	}
	if first.store == second.store {
		t.Fatal("different token credentials share a provider store")
	}
	if first.store != build(authority, "&token=first").store {
		t.Fatal("the same effective token lost its stable provider store")
	}
	for _, equivalent := range []*MatrixTarget{
		build("first@"+authority, ""),
		build(":first@"+authority, ""),
	} {
		if first.store != equivalent.store {
			t.Fatal("equivalent token/password credentials use different stores")
		}
	}
	overridden := build("named:old@"+authority, "&token=first")
	if overridden.store != build("named:first@"+authority, "").store {
		t.Fatal("query token override differs from the effective password store")
	}
	if first.store == build("other.example.com", "&token=first").store {
		t.Fatal("the same token on different servers shares a provider store")
	}
}
