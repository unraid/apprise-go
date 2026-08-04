package notify

import (
	"encoding/json"
	"net/http"
)

func doJSONRequest(spec RequestSpec, out any) error {
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

	if out == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
