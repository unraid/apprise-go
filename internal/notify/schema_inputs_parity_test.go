package notify_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
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

func loadGoSchemaInputs(t *testing.T, schema, url string) notify.SchemaInputs {
	t.Helper()

	parsed, err := notify.ParseURL(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	inputs, err := notify.SchemaInputsForParsed(schema, parsed)
	if err != nil {
		t.Fatalf("schema inputs: %v", err)
	}

	return inputs
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

func goString(t *testing.T, values map[string]any, key string) string {
	t.Helper()

	raw, ok := values[key]
	if !ok {
		t.Fatalf("missing go value: %s", key)
	}
	value, ok := raw.(string)
	if !ok {
		t.Fatalf("go value %s type mismatch: %T", key, raw)
	}
	return value
}

func goBool(t *testing.T, values map[string]any, key string) bool {
	t.Helper()

	raw, ok := values[key]
	if !ok {
		t.Fatalf("missing go value: %s", key)
	}
	value, ok := raw.(bool)
	if !ok {
		t.Fatalf("go value %s type mismatch: %T", key, raw)
	}
	return value
}

func goStringList(t *testing.T, values map[string]any, key string) []string {
	t.Helper()

	raw, ok := values[key]
	if !ok {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			value, ok := item.(string)
			if !ok {
				t.Fatalf("go value %s list entry type mismatch: %T", key, item)
			}
			out = append(out, value)
		}
		return out
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	default:
		t.Fatalf("go value %s type mismatch: %T", key, raw)
		return nil
	}
}

func TestSchemaInputsParityApprise(t *testing.T) {
	url := "apprise://example.com/token?method=json&+X-Test=1"
	python := loadPythonSchemaInputs(t, "apprise", url)
	inputs := loadGoSchemaInputs(t, "apprise", url)
	goValues := inputs.ValuesMap()

	if pythonString(t, python.Values, "token") != goString(t, goValues, "token") {
		t.Fatalf("token mismatch: python=%s go=%s", python.Values["token"], goValues["token"])
	}
	if strings.ToLower(pythonString(t, python.Values, "method")) != strings.ToLower(goString(t, goValues, "method")) {
		t.Fatalf("method mismatch: python=%s go=%s", python.Values["method"], goValues["method"])
	}
	if !reflect.DeepEqual(python.Kwargs["headers"], inputs.Kwargs["headers"]) {
		t.Fatalf("headers mismatch: python=%v go=%v", python.Kwargs["headers"], inputs.Kwargs["headers"])
	}
}

func TestSchemaInputsParityDiscord(t *testing.T) {
	url := "discord://bot@123/abc?tts=yes&avatar=no&avatar_url=https://example.com/avatar.png"
	python := loadPythonSchemaInputs(t, "discord", url)
	inputs := loadGoSchemaInputs(t, "discord", url)
	goValues := inputs.ValuesMap()

	if pythonString(t, python.Values, "webhook_id") != goString(t, goValues, "webhook_id") {
		t.Fatalf("webhook id mismatch: python=%s go=%s", python.Values["webhook_id"], goValues["webhook_id"])
	}
	if pythonString(t, python.Values, "webhook_token") != goString(t, goValues, "webhook_token") {
		t.Fatalf("webhook token mismatch: python=%s go=%s", python.Values["webhook_token"], goValues["webhook_token"])
	}
	if pythonString(t, python.Values, "user") != goString(t, goValues, "user") {
		t.Fatalf("username mismatch: python=%s go=%s", python.Values["user"], goValues["user"])
	}
	if pythonBool(t, python.Values, "tts") != goBool(t, goValues, "tts") {
		t.Fatalf("tts mismatch: python=%v go=%v", python.Values["tts"], goValues["tts"])
	}
	if pythonBool(t, python.Values, "avatar") != goBool(t, goValues, "avatar") {
		t.Fatalf("avatar mismatch: python=%v go=%v", python.Values["avatar"], goValues["avatar"])
	}
	if pythonString(t, python.Values, "avatar_url") != goString(t, goValues, "avatar_url") {
		t.Fatalf("avatar url mismatch: python=%s go=%s", python.Values["avatar_url"], goValues["avatar_url"])
	}
}

func TestSchemaInputsParitySlack(t *testing.T) {
	url := "slack://tokenA/tokenB/tokenC/chan1?to=chan2&token=overrideA/overrideB/overrideC&mode=hook&image=no&footer=yes&timestamp=no&blocks=yes"
	python := loadPythonSchemaInputs(t, "slack", url)
	inputs := loadGoSchemaInputs(t, "slack", url)
	goValues := inputs.ValuesMap()

	if pythonString(t, python.Values, "token_a") != goString(t, goValues, "token_a") {
		t.Fatalf("token_a mismatch: python=%s go=%s", python.Values["token_a"], goValues["token_a"])
	}
	if pythonString(t, python.Values, "token_b") != goString(t, goValues, "token_b") {
		t.Fatalf("token_b mismatch: python=%s go=%s", python.Values["token_b"], goValues["token_b"])
	}
	if pythonString(t, python.Values, "token_c") != goString(t, goValues, "token_c") {
		t.Fatalf("token_c mismatch: python=%s go=%s", python.Values["token_c"], goValues["token_c"])
	}
	if strings.ToLower(pythonString(t, python.Values, "mode")) != strings.ToLower(goString(t, goValues, "mode")) {
		t.Fatalf("mode mismatch: python=%s go=%s", python.Values["mode"], goValues["mode"])
	}
	if python.Values["user"] != nil {
		if pythonString(t, python.Values, "user") != goString(t, goValues, "user") {
			t.Fatalf("user mismatch: python=%s go=%s", python.Values["user"], goValues["user"])
		}
	} else if value, ok := goValues["user"]; ok && value != nil && value != "" {
		t.Fatalf("user mismatch: python=nil go=%v", value)
	}
	if pythonBool(t, python.Values, "include_image") != goBool(t, goValues, "include_image") {
		t.Fatalf("include image mismatch: python=%v go=%v", python.Values["include_image"], goValues["include_image"])
	}
	if pythonBool(t, python.Values, "include_footer") != goBool(t, goValues, "include_footer") {
		t.Fatalf("include footer mismatch: python=%v go=%v", python.Values["include_footer"], goValues["include_footer"])
	}
	if pythonBool(t, python.Values, "include_timestamp") != goBool(t, goValues, "include_timestamp") {
		t.Fatalf("include timestamp mismatch: python=%v go=%v", python.Values["include_timestamp"], goValues["include_timestamp"])
	}
	if pythonBool(t, python.Values, "use_blocks") != goBool(t, goValues, "use_blocks") {
		t.Fatalf("use blocks mismatch: python=%v go=%v", python.Values["use_blocks"], goValues["use_blocks"])
	}
	if !reflect.DeepEqual(pythonStringList(t, python.Values, "targets"), goStringList(t, goValues, "targets")) {
		t.Fatalf("targets mismatch: python=%v go=%v", python.Values["targets"], goValues["targets"])
	}
}

func TestSchemaInputsParityNtfy(t *testing.T) {
	url := "ntfy://user:pass@ntfy.example.com/topic?mode=private&image=no"
	python := loadPythonSchemaInputs(t, "ntfy", url)
	inputs := loadGoSchemaInputs(t, "ntfy", url)
	goValues := inputs.ValuesMap()

	if strings.ToLower(pythonString(t, python.Values, "mode")) != strings.ToLower(goString(t, goValues, "mode")) {
		t.Fatalf("mode mismatch: python=%s go=%s", python.Values["mode"], goValues["mode"])
	}
	if pythonBool(t, python.Values, "include_image") != goBool(t, goValues, "include_image") {
		t.Fatalf("include image mismatch: python=%v go=%v", python.Values["include_image"], goValues["include_image"])
	}
	if !reflect.DeepEqual(pythonStringList(t, python.Values, "targets"), goStringList(t, goValues, "targets")) {
		t.Fatalf("targets mismatch: python=%v go=%v", python.Values["targets"], goValues["targets"])
	}
}

func TestSchemaInputsParityJSON(t *testing.T) {
	url := "json://user:pass@host:123/path?method=PUT&+X-Test=1&-q=2&:extra=3"
	python := loadPythonSchemaInputs(t, "json", url)
	inputs := loadGoSchemaInputs(t, "json", url)
	goValues := inputs.ValuesMap()

	if strings.ToUpper(pythonString(t, python.Values, "method")) != strings.ToUpper(goString(t, goValues, "method")) {
		t.Fatalf("method mismatch: python=%s go=%s", python.Values["method"], goValues["method"])
	}
	if !reflect.DeepEqual(python.Kwargs["headers"], inputs.Kwargs["headers"]) {
		t.Fatalf("headers mismatch: python=%v go=%v", python.Kwargs["headers"], inputs.Kwargs["headers"])
	}
	if !reflect.DeepEqual(python.Kwargs["params"], inputs.Kwargs["params"]) {
		t.Fatalf("params mismatch: python=%v go=%v", python.Kwargs["params"], inputs.Kwargs["params"])
	}
	if !reflect.DeepEqual(python.Kwargs["payload"], inputs.Kwargs["payload"]) {
		t.Fatalf("payload mismatch: python=%v go=%v", python.Kwargs["payload"], inputs.Kwargs["payload"])
	}
}

func TestSchemaInputsParityXML(t *testing.T) {
	url := "xml://user:pass@host:123/path?method=PUT&+X-Test=1&-q=2&:extra=3"
	python := loadPythonSchemaInputs(t, "xml", url)
	inputs := loadGoSchemaInputs(t, "xml", url)
	goValues := inputs.ValuesMap()

	if strings.ToUpper(pythonString(t, python.Values, "method")) != strings.ToUpper(goString(t, goValues, "method")) {
		t.Fatalf("method mismatch: python=%s go=%s", python.Values["method"], goValues["method"])
	}
	if !reflect.DeepEqual(python.Kwargs["headers"], inputs.Kwargs["headers"]) {
		t.Fatalf("headers mismatch: python=%v go=%v", python.Kwargs["headers"], inputs.Kwargs["headers"])
	}
	if !reflect.DeepEqual(python.Kwargs["params"], inputs.Kwargs["params"]) {
		t.Fatalf("params mismatch: python=%v go=%v", python.Kwargs["params"], inputs.Kwargs["params"])
	}
	if !reflect.DeepEqual(python.Kwargs["payload"], inputs.Kwargs["payload"]) {
		t.Fatalf("payload mismatch: python=%v go=%v", python.Kwargs["payload"], inputs.Kwargs["payload"])
	}
}
