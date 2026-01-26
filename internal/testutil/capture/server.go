package capture

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type Request struct {
	Method   string
	Path     string
	RawQuery string
	Header   http.Header
	Body     []byte
}

type Server struct {
	t  *testing.T
	mu sync.Mutex

	srv      *httptest.Server
	requests []Request
}

func NewServer(t *testing.T) *Server {
	t.Helper()

	server := &Server{t: t}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("unable to bind test server listener: %v", err)
		return server
	}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(server.handle))
	srv.Listener = listener
	srv.Start()
	server.srv = srv
	return server
}

func (s *Server) URL() string {
	return s.srv.URL
}

func (s *Server) Close() {
	s.srv.Close()
}

func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()

	copyHeaders := func(h http.Header) http.Header {
		dup := make(http.Header, len(h))
		for k, v := range h {
			values := make([]string, len(v))
			copy(values, v)
			dup[k] = values
		}
		return dup
	}

	requests := make([]Request, len(s.requests))
	for i, req := range s.requests {
		requests[i] = Request{
			Method:   req.Method,
			Path:     req.Path,
			RawQuery: req.RawQuery,
			Header:   copyHeaders(req.Header),
			Body:     append([]byte(nil), req.Body...),
		}
	}

	return requests
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.t.Errorf("read body: %v", err)
	}
	_ = r.Body.Close()

	s.mu.Lock()
	s.requests = append(s.requests, Request{
		Method:   r.Method,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
		Header:   r.Header.Clone(),
		Body:     body,
	})
	s.mu.Unlock()

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
