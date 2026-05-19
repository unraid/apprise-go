package apprise

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

func TestAppriseSendJSONTarget(t *testing.T) {
	testutil.RequirePythonApprise(t)

	targetURL := "json://example.com/notify"
	body := "hello"
	title := "Greeting"
	pythonRequests := testutil.CapturePythonRequestsWithFormat(t, targetURL, body, title, "text")

	goRequests := testutil.CaptureGoRequests(t, func() error {
		client := New()
		if err := client.Add(targetURL); err != nil {
			return err
		}
		return client.Send(body, WithTitle(title))
	})

	testutil.AssertRequestSpecSequenceMatches(t, pythonRequests, goRequests)
}

func TestAppriseSendConvertsInputFormat(t *testing.T) {
	testutil.RequirePythonApprise(t)

	requests := make(chan notify.RequestSpec, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- captureRequestSpec(t, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	targetURL := "json://" + server.Listener.Addr().String() + "/notify?format=html"
	body := "_hello_"

	pyStdout, pyStderr, err := testutil.RunApprise(t, "-i", "markdown", "-b", body, targetURL)
	if err != nil {
		t.Fatalf("python apprise failed: %v stdout=%s stderr=%s", err, pyStdout, pyStderr)
	}
	pythonRequests := readRequestSpecs(t, requests)

	if err := Send(
		[]string{targetURL},
		body,
		WithInputFormat("markdown"),
	); err != nil {
		t.Fatalf("send: %v", err)
	}
	goRequests := readRequestSpecs(t, requests)

	testutil.AssertRequestSpecSequenceMatches(t, pythonRequests, goRequests)
}

func TestAppriseSendNoTargets(t *testing.T) {
	err := New().Send("hello")
	if !errors.Is(err, ErrNoTargets) {
		t.Fatalf("error = %v, want ErrNoTargets", err)
	}
}

func TestAppriseAddRejectsUnsupportedSchema(t *testing.T) {
	err := New().Add("unknown://example.com")
	if err == nil || !strings.Contains(err.Error(), "unsupported URL scheme: unknown") {
		t.Fatalf("error = %v, want unsupported schema", err)
	}
}

func captureRequestSpec(t *testing.T, r *http.Request) notify.RequestSpec {
	t.Helper()
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	headers := map[string]string{}
	for key, values := range r.Header {
		headers[key] = strings.Join(values, ",")
	}

	return notify.RequestSpec{
		Method:  r.Method,
		URL:     "http://" + r.Host + r.URL.RequestURI(),
		Headers: headers,
		Body:    string(body),
	}
}

func readRequestSpecs(t *testing.T, requests <-chan notify.RequestSpec) []notify.RequestSpec {
	t.Helper()

	specs := []notify.RequestSpec{}
	select {
	case spec := <-requests:
		specs = append(specs, spec)
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for request")
		return nil
	}

	for {
		select {
		case spec := <-requests:
			specs = append(specs, spec)
		default:
			return specs
		}
	}
}
