package notify

import (
	"net/http"
	"strings"
)

type RequestSpec struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

func (r RequestSpec) HTTPRequest() (*http.Request, error) {
	var bodyReader *strings.Reader
	if r.Body != "" {
		bodyReader = strings.NewReader(r.Body)
	} else {
		bodyReader = strings.NewReader("")
	}

	req, err := http.NewRequest(r.Method, r.URL, bodyReader)
	if err != nil {
		return nil, err
	}

	for key, value := range r.Headers {
		req.Header.Set(key, value)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "*/*")
	}

	return req, nil
}

func SendRequest(spec RequestSpec) error {
	req, err := spec.HTTPRequest()
	if err != nil {
		return err
	}

	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &HTTPStatusError{StatusCode: resp.StatusCode}
	}

	return nil
}

type HTTPStatusError struct {
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return "unexpected status: " + http.StatusText(e.StatusCode)
}
