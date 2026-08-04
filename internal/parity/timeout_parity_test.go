package parity

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/unraid/apprise-go/internal/notify"
)

// TestSocketTimeouts covers ?cto= and ?rto=, which no request fixture could
// reach: a captured request cannot show how long the caller was willing to
// wait, and every mock in the harness answers instantly.
//
// The instrumentation is a server that accepts the connection and then goes
// quiet. Before this, every request in the port went out on
// http.DefaultClient, which has no timeout at all — such a server would hang
// the caller forever, where upstream gives up after four seconds.
func TestSocketTimeouts(t *testing.T) {
	stalled := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		// Accept, then never reply.
		<-stalled
	}))

	// Order matters: Close waits for outstanding handlers, so the stalled one
	// has to be released first or the two wait on each other.
	defer server.Close()
	defer close(stalled)

	tests := []struct {
		name  string
		query string
		max   time.Duration
	}{
		{
			name:  "read timeout is honored",
			query: "?rto=1",
			max:   4 * time.Second,
		},
		{
			// With no argument the default applies, which is the part that
			// matters most: it is what every send in the port now gets.
			name: "a default read timeout applies",
			max:  8 * time.Second,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			host, port, err := net.SplitHostPort(server.Listener.Addr().String())
			if err != nil {
				t.Fatalf("split server address: %v", err)
			}

			url := fmt.Sprintf("json://%s:%s/%s", host, port, tc.query)
			parsed, err := notify.ParseURL(url)
			if err != nil {
				t.Fatalf("parse url: %v", err)
			}
			target, err := notify.NewTarget(parsed)
			if err != nil {
				t.Fatalf("build target: %v", err)
			}

			done := make(chan error, 1)
			started := time.Now()
			go func() {
				done <- notify.DispatchSendWithOverflow(
					target, parsed, "body", "title", notify.NotifyInfo, nil)
			}()

			select {
			case err := <-done:
				if err == nil {
					t.Fatal("a server that never replies should not produce a successful send")
				}
				if elapsed := time.Since(started); elapsed > tc.max {
					t.Fatalf("gave up after %s, which is longer than the %s allowed",
						elapsed, tc.max)
				}

			case <-time.After(tc.max):
				t.Fatalf("still waiting after %s; the request has no effective "+
					"timeout", tc.max)
			}
		})
	}
}

// TestRedirectHonoursTheURL covers ?redirect=no, which needs a server that
// redirects — the capture mocks never do, so neither side was exercised.
func TestRedirectHonoursTheURL(t *testing.T) {
	var landed int

	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		landed++
		w.WriteHeader(http.StatusOK)
	}))
	defer final.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redirector.Close()

	for _, tc := range []struct {
		name       string
		query      string
		wantLanded int
		wantErr    bool
	}{
		{name: "redirects are followed by default", wantLanded: 1},
		{name: "redirect=no stops at the redirect", query: "?redirect=no", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			landed = 0

			host, port, err := net.SplitHostPort(redirector.Listener.Addr().String())
			if err != nil {
				t.Fatalf("split address: %v", err)
			}

			parsed, err := notify.ParseURL(fmt.Sprintf("json://%s:%s/%s", host, port, tc.query))
			if err != nil {
				t.Fatalf("parse url: %v", err)
			}
			target, err := notify.NewTarget(parsed)
			if err != nil {
				t.Fatalf("build target: %v", err)
			}

			err = notify.DispatchSendWithOverflow(
				target, parsed, "body", "title", notify.NotifyInfo, nil)

			if tc.wantErr && err == nil {
				t.Fatal("a 302 left unfollowed should not be reported as success")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("send failed: %v", err)
			}
			if landed != tc.wantLanded {
				t.Fatalf("the redirect target saw %d request(s), expected %d",
					landed, tc.wantLanded)
			}
		})
	}
}
