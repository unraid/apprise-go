package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
)

func TestStorageListActive(t *testing.T) {
	tempDir := t.TempDir()
	url := "json://user:pass@example.com"

	parsed, err := notify.ParseURL(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	uid := notify.URLID(parsed, defaultStorageUIDLength, nil)
	if uid == "" {
		t.Fatalf("empty url id")
	}

	dataDir := filepath.Join(tempDir, uid, "var")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, storageCacheFile), []byte("x"), 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	t.Setenv(defaultEnvAppriseURLs, url)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"--storage-path", tempDir, "storage"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("storage list failed: code=%d stdout=%s stderr=%s", code, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}

	output := stdout.String()
	if !strings.Contains(output, uid) {
		t.Fatalf("expected uid in output: %s", output)
	}
	if !strings.Contains(output, "active") {
		t.Fatalf("expected active state in output: %s", output)
	}
}

func TestStorageListMatrixEffectiveCredentials(t *testing.T) {
	for _, credentials := range []string{"token@", ":token@", "named:old@"} {
		t.Run(credentials, func(t *testing.T) {
			storagePath := t.TempDir()
			notify.ConfigureStorage(storagePath, defaultStorageUIDLength, nil)
			t.Cleanup(func() { notify.ConfigureStorage("", defaultStorageUIDLength, nil) })
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/login"):
					_, _ = fmt.Fprint(w, `{"access_token":"token","user_id":"@bot:account.example.com","device_id":"DEVICE"}`)
				case strings.HasSuffix(r.URL.Path, "/account/whoami"):
					_, _ = fmt.Fprint(w, `{"user_id":"@bot:account.example.com","device_id":"DEVICE"}`)
				case strings.Contains(r.URL.Path, "/join/"):
					_, _ = fmt.Fprint(w, `{"room_id":"!room:account.example.com"}`)
				case strings.Contains(r.URL.Path, "/send/m.room.message/"):
					_, _ = fmt.Fprint(w, `{"event_id":"$message"}`)
				default:
					t.Errorf("unexpected Matrix request %s", r.URL.Path)
				}
			}))
			t.Cleanup(server.Close)
			previousClient := http.DefaultClient
			http.DefaultClient = server.Client()
			t.Cleanup(func() { http.DefaultClient = previousClient })
			url := "matrixs://" + credentials + strings.TrimPrefix(server.URL, "https://") + "/!room:account.example.com?discovery=no&e2ee=no"
			if credentials == "named:old@" {
				url += "&token=token"
			}
			parsed, err := notify.ParseURL(url)
			if err != nil {
				t.Fatal(err)
			}
			target, err := notify.NewMatrixTarget(parsed)
			if err != nil {
				t.Fatal(err)
			}
			if err := target.Send("body", "title", notify.NotifyInfo); err != nil {
				t.Fatal(err)
			}
			dirs, err := os.ReadDir(storagePath)
			if err != nil {
				t.Fatal(err)
			}
			if len(dirs) != 1 {
				t.Fatalf("expected one actual provider store: %v", dirs)
			}
			uid := dirs[0].Name()
			config := writeConfig(t, "apprise.conf", "matrix="+url+"\n")
			t.Setenv(defaultEnvAppriseStoragePath, storagePath)
			for _, tagged := range []bool{false, true} {
				args := []string{"--config", config, "storage"}
				if tagged {
					args = []string{"--config", config, "--tag", "matrix", "storage"}
				}
				var stdout, stderr bytes.Buffer
				if code := Run(args, &stdout, &stderr); code != 0 {
					t.Fatalf("storage list failed: %d %s", code, stderr.String())
				}
				output := stdout.String()
				if !strings.Contains(output, uid) || !strings.Contains(output, "active") || strings.Contains(output, "stale") || strings.Contains(output, "unused") {
					t.Fatalf("provider cache was not recognized for configured URL (tagged=%t): %s", tagged, output)
				}
			}
		})
	}
}
