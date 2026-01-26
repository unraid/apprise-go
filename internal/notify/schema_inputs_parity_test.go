package notify

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/testutil"
)

type pythonSchemaInputs struct {
	Values  map[string]any               `json:"values"`
	Kwargs  map[string]map[string]string `json:"kwargs"`
	Aliases map[string]string            `json:"aliases"`
}

func loadPythonSchemaInputs(t *testing.T, schema, url string) pythonSchemaInputs {
	t.Helper()

	appriseRoot := testutil.AppriseSourceRoot(t)
	t.Setenv("PYTHONPATH", appriseRoot)

	script := filepath.Join(testutil.RepoRoot(t), "internal", "testutil", "scripts", "schema_inputs.py")
	stdout, stderr, err := testutil.RunPythonScript(t, script, schema, url)
	if err != nil {
		t.Fatalf("python schema inputs failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	var result pythonSchemaInputs
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode python schema inputs: %v (stdout: %s)", err, strings.TrimSpace(stdout))
	}

	return result
}

func pythonString(t *testing.T, values map[string]any, key string) string {
	t.Helper()

	raw, ok := values[key]
	if !ok {
		t.Fatalf("missing python value: %s", key)
	}
	value, ok := raw.(string)
	if !ok {
		t.Fatalf("python value %s type mismatch: %T", key, raw)
	}
	return value
}

func pythonBool(t *testing.T, values map[string]any, key string) bool {
	t.Helper()

	raw, ok := values[key]
	if !ok {
		t.Fatalf("missing python value: %s", key)
	}
	value, ok := raw.(bool)
	if !ok {
		t.Fatalf("python value %s type mismatch: %T", key, raw)
	}
	return value
}

func pythonStringList(t *testing.T, values map[string]any, key string) []string {
	t.Helper()

	raw, ok := values[key]
	if !ok {
		return nil
	}
	switch typed := raw.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			value, ok := item.(string)
			if !ok {
				t.Fatalf("python value %s list entry type mismatch: %T", key, item)
			}
			out = append(out, value)
		}
		return out
	case []string:
		return append([]string(nil), typed...)
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	default:
		t.Fatalf("python value %s type mismatch: %T", key, raw)
		return nil
	}
}

func TestSchemaInputsParityApprise(t *testing.T) {
	url := "apprise://example.com/token?method=json&+X-Test=1"
	python := loadPythonSchemaInputs(t, "apprise", url)

	parsed, err := ParseURL(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	target, err := NewAppriseTarget(parsed)
	if err != nil {
		t.Fatalf("new apprise target: %v", err)
	}

	if pythonString(t, python.Values, "token") != target.token {
		t.Fatalf("token mismatch: python=%s go=%s", python.Values["token"], target.token)
	}
	if strings.ToLower(pythonString(t, python.Values, "method")) != target.method {
		t.Fatalf("method mismatch: python=%s go=%s", python.Values["method"], target.method)
	}
	if !reflect.DeepEqual(python.Kwargs["headers"], target.headers) {
		t.Fatalf("headers mismatch: python=%v go=%v", python.Kwargs["headers"], target.headers)
	}
}

func TestSchemaInputsParityDiscord(t *testing.T) {
	url := "discord://bot@123/abc?tts=yes&avatar=no&avatar_url=https://example.com/avatar.png"
	python := loadPythonSchemaInputs(t, "discord", url)

	parsed, err := ParseURL(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	target, err := NewDiscordTarget(parsed)
	if err != nil {
		t.Fatalf("new discord target: %v", err)
	}

	if pythonString(t, python.Values, "webhook_id") != target.webhookID {
		t.Fatalf("webhook id mismatch: python=%s go=%s", python.Values["webhook_id"], target.webhookID)
	}
	if pythonString(t, python.Values, "webhook_token") != target.webhookToken {
		t.Fatalf("webhook token mismatch: python=%s go=%s", python.Values["webhook_token"], target.webhookToken)
	}
	if pythonString(t, python.Values, "user") != target.username {
		t.Fatalf("username mismatch: python=%s go=%s", python.Values["user"], target.username)
	}
	if pythonBool(t, python.Values, "tts") != target.tts {
		t.Fatalf("tts mismatch: python=%v go=%v", python.Values["tts"], target.tts)
	}
	if pythonBool(t, python.Values, "avatar") != target.avatar {
		t.Fatalf("avatar mismatch: python=%v go=%v", python.Values["avatar"], target.avatar)
	}
	if pythonString(t, python.Values, "avatar_url") != target.avatarURL {
		t.Fatalf("avatar url mismatch: python=%s go=%s", python.Values["avatar_url"], target.avatarURL)
	}
}

func TestSchemaInputsParitySlack(t *testing.T) {
	url := "slack://tokenA/tokenB/tokenC/chan1?to=chan2&token=overrideA/overrideB/overrideC&mode=hook&image=no&footer=yes&timestamp=no&blocks=yes"
	python := loadPythonSchemaInputs(t, "slack", url)

	parsed, err := ParseURL(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	target, err := NewSlackTarget(parsed)
	if err != nil {
		t.Fatalf("new slack target: %v", err)
	}

	if pythonString(t, python.Values, "token_a") != target.tokenA {
		t.Fatalf("token_a mismatch: python=%s go=%s", python.Values["token_a"], target.tokenA)
	}
	if pythonString(t, python.Values, "token_b") != target.tokenB {
		t.Fatalf("token_b mismatch: python=%s go=%s", python.Values["token_b"], target.tokenB)
	}
	if pythonString(t, python.Values, "token_c") != target.tokenC {
		t.Fatalf("token_c mismatch: python=%s go=%s", python.Values["token_c"], target.tokenC)
	}
	if strings.ToLower(pythonString(t, python.Values, "mode")) != target.mode {
		t.Fatalf("mode mismatch: python=%s go=%s", python.Values["mode"], target.mode)
	}
	if pythonString(t, python.Values, "user") != target.username {
		t.Fatalf("user mismatch: python=%s go=%s", python.Values["user"], target.username)
	}
	if pythonBool(t, python.Values, "include_image") != target.includeImage {
		t.Fatalf("include image mismatch: python=%v go=%v", python.Values["include_image"], target.includeImage)
	}
	if pythonBool(t, python.Values, "include_footer") != target.includeFooter {
		t.Fatalf("include footer mismatch: python=%v go=%v", python.Values["include_footer"], target.includeFooter)
	}
	if pythonBool(t, python.Values, "include_timestamp") != target.includeTimestamp {
		t.Fatalf("include timestamp mismatch: python=%v go=%v", python.Values["include_timestamp"], target.includeTimestamp)
	}
	if pythonBool(t, python.Values, "use_blocks") != target.useBlocks {
		t.Fatalf("use blocks mismatch: python=%v go=%v", python.Values["use_blocks"], target.useBlocks)
	}
	if !reflect.DeepEqual(pythonStringList(t, python.Values, "targets"), target.targets) {
		t.Fatalf("targets mismatch: python=%v go=%v", python.Values["targets"], target.targets)
	}
}

func TestSchemaInputsParityNtfy(t *testing.T) {
	url := "ntfy://user:pass@ntfy.example.com/topic?mode=private&image=no"
	python := loadPythonSchemaInputs(t, "ntfy", url)

	parsed, err := ParseURL(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	target, err := NewNtfyTarget(parsed)
	if err != nil {
		t.Fatalf("new ntfy target: %v", err)
	}

	if strings.ToLower(pythonString(t, python.Values, "mode")) != string(target.mode) {
		t.Fatalf("mode mismatch: python=%s go=%s", python.Values["mode"], target.mode)
	}
	if pythonBool(t, python.Values, "include_image") != target.includeImage {
		t.Fatalf("include image mismatch: python=%v go=%v", python.Values["include_image"], target.includeImage)
	}
	if !reflect.DeepEqual(pythonStringList(t, python.Values, "targets"), target.topics) {
		t.Fatalf("targets mismatch: python=%v go=%v", python.Values["targets"], target.topics)
	}
}

func TestSchemaInputsParityJSON(t *testing.T) {
	url := "json://user:pass@host:123/path?method=PUT&+X-Test=1&-q=2&:extra=3"
	python := loadPythonSchemaInputs(t, "json", url)

	parsed, err := ParseURL(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	target, err := NewJSONTarget(parsed)
	if err != nil {
		t.Fatalf("new json target: %v", err)
	}

	if strings.ToUpper(pythonString(t, python.Values, "method")) != target.method {
		t.Fatalf("method mismatch: python=%s go=%s", python.Values["method"], target.method)
	}
	if !reflect.DeepEqual(python.Kwargs["headers"], target.headers) {
		t.Fatalf("headers mismatch: python=%v go=%v", python.Kwargs["headers"], target.headers)
	}
	if !reflect.DeepEqual(python.Kwargs["params"], target.params) {
		t.Fatalf("params mismatch: python=%v go=%v", python.Kwargs["params"], target.params)
	}
	if !reflect.DeepEqual(python.Kwargs["payload"], target.payloadExtras) {
		t.Fatalf("payload mismatch: python=%v go=%v", python.Kwargs["payload"], target.payloadExtras)
	}
}

func TestSchemaInputsParityXML(t *testing.T) {
	url := "xml://user:pass@host:123/path?method=PUT&+X-Test=1&-q=2&:extra=3"
	python := loadPythonSchemaInputs(t, "xml", url)

	parsed, err := ParseURL(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	target, err := NewXMLTarget(parsed)
	if err != nil {
		t.Fatalf("new xml target: %v", err)
	}

	if strings.ToUpper(pythonString(t, python.Values, "method")) != target.method {
		t.Fatalf("method mismatch: python=%s go=%s", python.Values["method"], target.method)
	}
	if !reflect.DeepEqual(python.Kwargs["headers"], target.headers) {
		t.Fatalf("headers mismatch: python=%v go=%v", python.Kwargs["headers"], target.headers)
	}
	if !reflect.DeepEqual(python.Kwargs["params"], target.params) {
		t.Fatalf("params mismatch: python=%v go=%v", python.Kwargs["params"], target.params)
	}
	if !reflect.DeepEqual(python.Kwargs["payload"], target.payloadExtras) {
		t.Fatalf("payload mismatch: python=%v go=%v", python.Kwargs["payload"], target.payloadExtras)
	}
}
