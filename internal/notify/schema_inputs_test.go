package notify

import "testing"

func TestSchemaInputsFromURLJSON(t *testing.T) {
	url := "json://user:pass@host:123/path?method=PUT&+X-Test=1&-q=2&:extra=3"
	inputs, err := SchemaInputsFromURL("json", url)
	if err != nil {
		t.Fatalf("schema inputs: %v", err)
	}

	values := inputs.ValuesMap()
	if values["host"] != "host" {
		t.Fatalf("host mismatch: %v", values["host"])
	}
	if values["user"] != "user" {
		t.Fatalf("user mismatch: %v", values["user"])
	}
	if values["password"] != "pass" {
		t.Fatalf("password mismatch: %v", values["password"])
	}
	if values["port"] != 123 {
		t.Fatalf("port mismatch: %v", values["port"])
	}
	if values["method"] != "PUT" {
		t.Fatalf("method mismatch: %v", values["method"])
	}

	headers := inputs.Kwargs["headers"]
	if headers["X-Test"] != "1" {
		t.Fatalf("headers mismatch: %v", headers)
	}
	params := inputs.Kwargs["params"]
	if params["q"] != "2" {
		t.Fatalf("params mismatch: %v", params)
	}
	payload := inputs.Kwargs["payload"]
	if payload["extra"] != "3" {
		t.Fatalf("payload mismatch: %v", payload)
	}
}

func TestSchemaInputsFromURLAppriseToken(t *testing.T) {
	url := "apprise://example.com/token?method=json"
	inputs, err := SchemaInputsFromURL("apprise", url)
	if err != nil {
		t.Fatalf("schema inputs: %v", err)
	}

	values := inputs.ValuesMap()
	if values["token"] != "token" {
		t.Fatalf("token mismatch: %v", values["token"])
	}
}
