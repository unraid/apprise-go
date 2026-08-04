package notify

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// Upstream's defaults, and the reason this file exists: every request in this
// port went out on http.DefaultClient, which has no timeout at all. A service
// that accepted a connection and then went quiet would hang the caller
// indefinitely, where upstream gives up after four seconds.
const (
	defaultConnectTimeout = 4 * time.Second
	defaultReadTimeout    = 4 * time.Second
)

// httpOptions are the per-send transport settings taken from the URL.
type httpOptions struct {
	connectTimeout time.Duration
	readTimeout    time.Duration
	followRedirect bool
}

func defaultHTTPOptions() httpOptions {
	return httpOptions{
		connectTimeout: defaultConnectTimeout,
		readTimeout:    defaultReadTimeout,
		followRedirect: true,
	}
}

// currentHTTPOptions is what the next request will use. Sends here are
// sequential — the CLI loops over targets one at a time and a split body is
// sent chunk by chunk — so scoping it around a send is safe. It is guarded
// anyway, because that constraint is not enforced by anything.
var (
	currentHTTPOptionsMu sync.RWMutex
	currentHTTPOptions   = defaultHTTPOptions()
)

// withHTTPOptions applies options for the duration of one send and returns the
// function that puts the previous ones back.
func withHTTPOptions(options httpOptions) func() {
	currentHTTPOptionsMu.Lock()
	previous := currentHTTPOptions
	currentHTTPOptions = options
	currentHTTPOptionsMu.Unlock()

	return func() {
		currentHTTPOptionsMu.Lock()
		currentHTTPOptions = previous
		currentHTTPOptionsMu.Unlock()
	}
}

// httpClient builds a client for the current options. Clients are cached by
// their settings, since a new transport per request would discard connection
// reuse and leak idle connections.
func httpClient() *http.Client {
	currentHTTPOptionsMu.RLock()
	options := currentHTTPOptions
	currentHTTPOptionsMu.RUnlock()

	// A test harness swaps the default transport for one that answers without
	// touching the network. Timeouts are meaningless against it, and cloning
	// assumes a type it does not have, so it is used unchanged.
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		client := &http.Client{Transport: http.DefaultTransport}
		if !options.followRedirect {
			client.CheckRedirect = func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			}
		}

		return client
	}

	// The cache is keyed by the transport as well as the options: a harness
	// may swap the default transport at any point, and a client cached
	// against the previous one would send to the network instead of to the
	// capture.
	key := clientKey{options: options, base: base}

	clientCacheMu.Lock()
	defer clientCacheMu.Unlock()

	if client, ok := clientCache[key]; ok {
		return client
	}

	transport := base.Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   options.connectTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	// The read timeout is how long to wait for the server to start replying,
	// which is what upstream's socket read timeout means.
	transport.ResponseHeaderTimeout = options.readTimeout

	client := &http.Client{Transport: transport}
	if !options.followRedirect {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	clientCache[key] = client

	return client
}

// clientKey identifies a cached client by what it was built from.
type clientKey struct {
	options httpOptions
	base    *http.Transport
}

var (
	clientCacheMu sync.Mutex
	clientCache   = map[clientKey]*http.Client{}
)
