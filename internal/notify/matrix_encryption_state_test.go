package notify

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type matrixStateReply struct {
	status int
	body   string
	err    error
}

type matrixStateTraffic struct {
	lookups  map[string]int
	messages []string
	uploads  int
}

func matrixStateTarget(t *testing.T, options string, state func(string, int) matrixStateReply) (*MatrixTarget, *matrixStateTraffic) {
	t.Helper()
	traffic := &matrixStateTraffic{lookups: map[string]int{}}
	previousClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: reviewTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.example.net" {
			t.Fatalf("unexpected Matrix fixture host: %s", r.URL.Host)
		}
		status, body := http.StatusOK, `{}`
		switch {
		case strings.HasSuffix(r.URL.Path, "/account/whoami"):
			body = `{"user_id":"@bot:example.com","device_id":"DEVICE"}`
		case strings.HasSuffix(r.URL.Path, "/keys/upload"):
			body = `{"one_time_key_counts":{"signed_curve25519":10}}`
		case strings.Contains(r.URL.Path, "/join/"):
			room := strings.SplitN(r.URL.Path, "/join/", 2)[1]
			body = fmt.Sprintf(`{"room_id":%q}`, room)
		case strings.Contains(r.URL.Path, "/state/m.room.encryption"):
			room := strings.SplitN(r.URL.Path, "/rooms/", 2)[1]
			room = strings.TrimSuffix(room, "/state/m.room.encryption")
			traffic.lookups[room]++
			reply := state(room, traffic.lookups[room])
			if reply.err != nil {
				return nil, reply.err
			}
			status, body = reply.status, reply.body
		case strings.HasSuffix(r.URL.Path, "/joined_members"):
			body = `{"joined":{}}`
		case strings.HasSuffix(r.URL.Path, "/upload"):
			traffic.uploads++
			body = `{"content_uri":"mxc://example.com/attachment"}`
		case strings.Contains(r.URL.Path, "/send/"):
			payload, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			traffic.messages = append(traffic.messages, r.URL.Path+" "+string(payload))
			body = `{"event_id":"$message"}`
		default:
			t.Errorf("unexpected Matrix fixture request: %s", r.URL.Path)
			status = http.StatusNotImplemented
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	t.Cleanup(func() { http.DefaultClient = previousClient })
	parsed, err := ParseURL("matrixs://token@api.example.net/!room:example.com?discovery=no&store=no&" + options)
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewMatrixTarget(parsed)
	if err != nil {
		t.Fatal(err)
	}
	return target, traffic
}

func TestMatrixUnknownEncryptionStateFailsClosed(t *testing.T) {
	cases := []struct {
		name  string
		reply matrixStateReply
	}{
		{"unauthorized", matrixStateReply{status: 401, body: `{}`}},
		{"forbidden", matrixStateReply{status: 403, body: `{}`}},
		{"rate-limited", matrixStateReply{status: 429, body: `{}`}},
		{"server-error", matrixStateReply{status: 500, body: `{}`}},
		{"transport-error", matrixStateReply{err: io.ErrUnexpectedEOF}},
		{"empty-object", matrixStateReply{status: 200, body: `{}`}},
		{"null-response", matrixStateReply{status: 200, body: `null`}},
		{"invalid-json", matrixStateReply{status: 200, body: `{`}},
		{"no-content", matrixStateReply{status: 204}},
	}
	for _, tc := range cases {
		for _, options := range []string{"version=2&image=yes", "version=3&image=yes"} {
			t.Run(tc.name+"/"+options, func(t *testing.T) {
				target, traffic := matrixStateTarget(t, options, func(string, int) matrixStateReply { return tc.reply })
				attachment := Attachment{Name: "private.txt", MIMEType: "text/plain", Data: []byte("private attachment")}
				for attempt := range 2 {
					if err := target.SendWithAttachments("private message", "private title", NotifyInfo, []Attachment{attachment}); err == nil {
						t.Errorf("attempt %d reported success with unknown encryption state; outbound messages: %v", attempt+1, traffic.messages)
					}
				}
				if traffic.lookups["!room:example.com"] != 2 || len(traffic.messages) != 0 || traffic.uploads != 0 {
					t.Fatalf("unknown state was cached or content escaped: lookups=%v messages=%v uploads=%d", traffic.lookups, traffic.messages, traffic.uploads)
				}
			})
		}
	}
}

func TestMatrixEncryptionStateRecoversWithoutPlaintext(t *testing.T) {
	target, traffic := matrixStateTarget(t, "", func(_ string, call int) matrixStateReply {
		if call == 1 {
			return matrixStateReply{status: 500, body: `{}`}
		}
		return matrixStateReply{status: 200, body: `{"algorithm":"m.megolm.v1.aes-sha2"}`}
	})
	if err := target.Send("private message", "private title", NotifyInfo); err == nil {
		t.Error("failed encryption lookup reported success")
	}
	for range 2 {
		if err := target.Send("private message", "private title", NotifyInfo); err != nil {
			t.Fatal(err)
		}
	}
	if traffic.lookups["!room:example.com"] != 2 || len(traffic.messages) != 2 {
		t.Fatalf("state recovery/cache changed: lookups=%v messages=%v", traffic.lookups, traffic.messages)
	}
	for _, message := range traffic.messages {
		if !strings.Contains(message, "/send/m.room.encrypted/") || strings.Contains(message, "private message") {
			t.Fatalf("message escaped without encryption: %s", message)
		}
	}
}

func TestMatrixEncryptionStateIgnoresLegacyAndNullCache(t *testing.T) {
	for _, cache := range []struct {
		name, prefix string
		value        any
	}{
		{"legacy-false", "e2ee_room_enc_", false},
		{"legacy-true", "e2ee_room_enc_", true},
		{"verified-null", "e2ee_room_enc_v2_", nil},
	} {
		t.Run(cache.name, func(t *testing.T) {
			target, traffic := matrixStateTarget(t, "", func(string, int) matrixStateReply {
				return matrixStateReply{status: 200, body: `{"algorithm":"m.megolm.v1.aes-sha2"}`}
			})
			if err := target.store.Set(cache.prefix+"!room:example.com", cache.value, matrixE2EECacheTTL); err != nil {
				t.Fatal(err)
			}
			if err := target.Send("private message", "title", NotifyInfo); err != nil {
				t.Fatal(err)
			}
			if traffic.lookups["!room:example.com"] != 1 || len(traffic.messages) != 1 || !strings.Contains(traffic.messages[0], "/send/m.room.encrypted/") {
				t.Fatalf("unverified cache suppressed fresh encrypted state: lookups=%v messages=%v", traffic.lookups, traffic.messages)
			}
		})
	}
}

func TestMatrixConfirmedPlaintextStateCachesAndContinuesRooms(t *testing.T) {
	target, traffic := matrixStateTarget(t, "", func(room string, _ int) matrixStateReply {
		if room == "!unknown:example.com" {
			return matrixStateReply{status: 500, body: `{}`}
		}
		return matrixStateReply{status: 404, body: `{"errcode":"M_NOT_FOUND"}`}
	})
	target.rooms = []string{"!unknown:example.com", "!plain:example.com"}
	for range 2 {
		if err := target.Send("body", "title", NotifyInfo); err == nil {
			t.Error("failed room was omitted from delivery outcome")
		}
	}
	if traffic.lookups["!unknown:example.com"] != 2 || traffic.lookups["!plain:example.com"] != 1 || len(traffic.messages) != 2 {
		t.Fatalf("room outcome/cache changed: lookups=%v messages=%v", traffic.lookups, traffic.messages)
	}
	for _, message := range traffic.messages {
		if !strings.Contains(message, "/rooms/!plain:example.com/send/m.room.message/") {
			t.Fatalf("sent content to an unconfirmed room: %s", message)
		}
	}
}
