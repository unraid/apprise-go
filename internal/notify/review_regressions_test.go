package notify

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

type reviewTransport func(*http.Request) (*http.Response, error)

func (f reviewTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMatrixWhoamiResolvesAccountIdentity(t *testing.T) {
	for _, login := range []bool{false, true} {
		t.Run(fmt.Sprintf("login=%t", login), func(t *testing.T) {
			var joins []string
			previousClient := http.DefaultClient
			http.DefaultClient = &http.Client{Transport: reviewTransport(func(r *http.Request) (*http.Response, error) {
				body := `{}`
				switch {
				case strings.HasSuffix(r.URL.Path, "/login"):
					body = `{"access_token":"token","device_id":"KNOWNDEVICE"}`
				case strings.HasSuffix(r.URL.Path, "/account/whoami"):
					body = `{"user_id":"@bot:account.example.com"}`
					if !login {
						body = `{"user_id":"@bot:account.example.com","device_id":"KNOWNDEVICE"}`
					}
				case strings.Contains(r.URL.Path, "/join/"):
					joins = append(joins, r.URL.Path)
					body = `{"room_id":"!room:account.example.com"}`
				case strings.Contains(r.URL.Path, "/send/m.room.message/"):
					body = `{"event_id":"$message"}`
				default:
					t.Errorf("unexpected Matrix request %s", r.URL.Path)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}
			t.Cleanup(func() { http.DefaultClient = previousClient })
			credentials := "token"
			if login {
				credentials = "bot:password"
			}
			parsed, err := ParseURL("matrixs://" + credentials + "@api.example.net/alerts?discovery=no&e2ee=no&store=no")
			if err != nil {
				t.Fatal(err)
			}
			target, err := NewMatrixTarget(parsed)
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := target.Send("body", "title", NotifyInfo); err != nil {
					t.Fatal(err)
				}
			}
			if target.userID != "@bot:account.example.com" || target.deviceID != "KNOWNDEVICE" || target.homeServer != "account.example.com" {
				t.Fatalf("resolved identity user=%q device=%q homeserver=%q", target.userID, target.deviceID, target.homeServer)
			}
			if !reflect.DeepEqual(joins, []string{"/_matrix/client/v3/join/#alerts:account.example.com"}) {
				t.Fatalf("wrong room resolution or cache reuse: %v", joins)
			}
		})
	}
}

func TestProviderNormalizedCopyRecipients(t *testing.T) {
	for _, scheme := range []string{"mailersend", "postmark"} {
		t.Run(scheme, func(t *testing.T) {
			parsed, err := ParseURL(scheme + "://token:sender@example.com/one@example.com?cc=Ops:copy@example.com,<copy@example.com>,Copy@example.com,One@example.com&bcc=Blind:blind@example.com,<blind@example.com>&reply=Reply:reply@example.com")
			if err != nil {
				t.Fatal(err)
			}
			var spec RequestSpec
			if scheme == "mailersend" {
				target, targetErr := NewMailerSendTarget(parsed)
				if targetErr != nil {
					t.Fatal(targetErr)
				}
				spec, err = target.BuildRequest("body", "title", NotifyInfo)
			} else {
				target, targetErr := NewPostmarkTarget(parsed)
				if targetErr != nil {
					t.Fatal(targetErr)
				}
				spec, err = target.BuildRequest("body", "title", NotifyInfo)
			}
			if err != nil {
				t.Fatal(err)
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(spec.Body), &payload); err != nil {
				t.Fatal(err)
			}
			if scheme == "mailersend" {
				if payload["reply_to"].(map[string]any)["email"] != "reply@example.com" {
					t.Fatalf("reply not normalized: %v", payload["reply_to"])
				}
				cc := payload["cc"].([]any)
				bcc := payload["bcc"].([]any)
				if len(cc) != 3 || len(bcc) != 1 || cc[0].(map[string]any)["email"] != "copy@example.com" || bcc[0].(map[string]any)["email"] != "blind@example.com" {
					t.Fatalf("copy normalization/deduplication changed recipients: %s", spec.Body)
				}
			} else if payload["Cc"] != "copy@example.com,Copy@example.com,One@example.com" || payload["Bcc"] != "blind@example.com" {
				t.Fatalf("copy normalization/deduplication changed case-distinct recipients: %s", spec.Body)
			}
		})
	}
}

func TestPostmarkMissingRecipientsFailsDelivery(t *testing.T) {
	parsed, err := ParseURL("postmark://token:sender@example.com/invalid")
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewPostmarkTarget(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Send("body", "title", NotifyInfo); err == nil {
		t.Fatal("empty recipient list reported delivery success")
	}
	if _, err := target.BuildRequest("body", "title", NotifyInfo); err == nil {
		t.Fatal("empty recipient list produced a request")
	}
}

func TestTelegramOptionalIconDoesNotBlockDelivery(t *testing.T) {
	for _, missing := range []bool{true, false} {
		t.Run(fmt.Sprintf("missing=%t", missing), func(t *testing.T) {
			t.Setenv("APPRISE_ASSET_ROOT", "")
			if missing {
				t.Setenv("APPRISE_ASSET_ROOT", t.TempDir())
			}
			var paths []string
			previousTransport := http.DefaultTransport
			http.DefaultTransport = reviewTransport(func(r *http.Request) (*http.Response, error) {
				paths = append(paths, r.URL.Path)
				code := 200
				if strings.HasSuffix(r.URL.Path, "/SendPhoto") {
					code = 500
				}
				return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Request: r}, nil
			})
			t.Cleanup(func() { http.DefaultTransport = previousTransport })
			parsed, err := ParseURL("tgram://123456:abcdef/7890/7891?image=yes&detect=no")
			if err != nil {
				t.Fatal(err)
			}
			target, err := NewTelegramTarget(parsed)
			if err != nil {
				t.Fatal(err)
			}
			if err := target.Send("body", "title", NotifyInfo); err != nil {
				t.Fatalf("optional icon failure blocked delivery: %v", err)
			}
			messages := 0
			for _, path := range paths {
				if strings.HasSuffix(path, "/sendMessage") {
					messages++
				}
			}
			if messages != 2 {
				t.Fatalf("expected both messages despite optional icon failure: %v", paths)
			}
		})
	}
}

func TestTelegramExplicitAttachmentFailureIsReported(t *testing.T) {
	previousTransport := http.DefaultTransport
	http.DefaultTransport = reviewTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":false}`)), Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })
	parsed, err := ParseURL("tgram://123456:abcdef/7890?image=no&detect=no")
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewTelegramTarget(parsed)
	if err != nil {
		t.Fatal(err)
	}
	attachment := Attachment{Name: "report.txt", MIMEType: "text/plain", Data: []byte("report")}
	if err := target.SendWithAttachments("body", "title", NotifyInfo, []Attachment{attachment}); err == nil {
		t.Fatal("explicit attachment failure reported success")
	}
}
