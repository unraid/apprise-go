package testutil

import (
	"encoding/json"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
)

var requestHeaderDrop = map[string]struct{}{
	"x-apprise-id":              {},
	"x-apprise-recursion-count": {},
}

var requestHeaderKeep = map[string]struct{}{
	"user-agent":    {},
	"content-type":  {},
	"accept":        {},
	"accepts":       {},
	"authorization": {},
}

const (
	appriseUpstreamAssetPrefix = "https://github.com/caronc/apprise/raw/master/apprise/assets/themes/default/apprise-"
	appriseGoAssetPrefix       = "https://raw.githubusercontent.com/unraid/apprise-go/main/assets/themes/default/apprise-"
	appriseUpstreamRepoURL     = "https://github.com/caronc/apprise"
	appriseGoRepoURL           = "https://github.com/unraid/apprise-go"
)

func AssertRequestSpecSequenceMatches(t *testing.T, pythonSpecs, goSpecs []notify.RequestSpec) {
	t.Helper()

	if len(pythonSpecs) != len(goSpecs) {
		t.Fatalf("request count mismatch: python=%d go=%d", len(pythonSpecs), len(goSpecs))
	}

	for i := range pythonSpecs {
		assertRequestSpecMatches(t, pythonSpecs[i], goSpecs[i])
	}
}

func assertRequestSpecMatches(t *testing.T, pythonSpec, goSpec notify.RequestSpec) {
	t.Helper()

	if !strings.EqualFold(pythonSpec.Method, goSpec.Method) {
		t.Fatalf("method mismatch: python=%s go=%s", pythonSpec.Method, goSpec.Method)
	}

	pythonBody := normalizeRequestBody(pythonSpec)
	goBody := normalizeRequestBody(goSpec)

	pythonURL, err := url.Parse(pythonSpec.URL)
	if err != nil {
		t.Fatalf("parse python url: %v", err)
	}
	goURL, err := url.Parse(goSpec.URL)
	if err != nil {
		t.Fatalf("parse go url: %v", err)
	}
	if pythonURL.Scheme != goURL.Scheme || pythonURL.Host != goURL.Host || pythonURL.Path != goURL.Path || pythonURL.Fragment != goURL.Fragment {
		t.Fatalf("url mismatch: python=%s go=%s", pythonSpec.URL, goSpec.URL)
	}
	pythonQuery := normalizeRequestQueryValues(pythonURL.Query()).Encode()
	goQuery := normalizeRequestQueryValues(goURL.Query()).Encode()
	if pythonQuery != goQuery {
		t.Fatalf("url query mismatch: python=%s go=%s", pythonQuery, goQuery)
	}

	pythonHeaders := NormalizeRequestHeaders(pythonSpec.Headers)
	goHeaders := NormalizeRequestHeaders(goSpec.Headers)
	if !reflect.DeepEqual(pythonHeaders, goHeaders) {
		t.Fatalf("header mismatch: python=%v go=%v", pythonHeaders, goHeaders)
	}

	if shouldCompareRequestJSON(pythonHeaders) && strings.TrimSpace(pythonBody) != "" && strings.TrimSpace(goBody) != "" {
		assertRequestJSONBodyEqual(t, pythonBody, goBody)
		return
	}
	if shouldCompareRequestBodyAsJSON(pythonBody, goBody) {
		assertRequestJSONBodyEqual(t, pythonBody, goBody)
		return
	}
	if shouldCompareRequestForm(pythonHeaders, pythonBody) {
		assertRequestQueryEqual(t, pythonBody, goBody)
		return
	}

	if pythonBody != goBody {
		t.Fatalf("body mismatch: python=%s go=%s", pythonBody, goBody)
	}
}

func NormalizeRequestHeaders(headers map[string]string) map[string]string {
	normalized := map[string]string{}
	for key, value := range headers {
		lower := strings.ToLower(key)
		if _, drop := requestHeaderDrop[lower]; drop {
			continue
		}
		if _, keep := requestHeaderKeep[lower]; keep || strings.HasPrefix(lower, "x-") {
			normalized[lower] = normalizeAppriseURL(value)
		}
	}

	sorted := make([]string, 0, len(normalized))
	for key := range normalized {
		sorted = append(sorted, key)
	}
	sort.Strings(sorted)

	ordered := map[string]string{}
	for _, key := range sorted {
		ordered[key] = normalized[key]
	}

	return ordered
}

func normalizeRequestBody(spec notify.RequestSpec) string {
	body := spec.Body
	if strings.TrimSpace(body) == "" {
		return ""
	}
	if strings.EqualFold(spec.Method, "GET") && strings.TrimSpace(body) == "null" {
		return ""
	}
	return body
}

func shouldCompareRequestJSON(headers map[string]string) bool {
	contentType := strings.ToLower(headers["content-type"])
	return strings.Contains(contentType, "application/json")
}

func shouldCompareRequestForm(headers map[string]string, body string) bool {
	contentType := strings.ToLower(headers["content-type"])
	if !strings.Contains(contentType, "application/x-www-form-urlencoded") {
		return false
	}
	return strings.Contains(body, "=")
}

func shouldCompareRequestBodyAsJSON(pythonBody, goBody string) bool {
	if strings.TrimSpace(pythonBody) == "" || strings.TrimSpace(goBody) == "" {
		return false
	}
	var pythonValue any
	if err := json.Unmarshal([]byte(pythonBody), &pythonValue); err != nil {
		return false
	}
	var goValue any
	if err := json.Unmarshal([]byte(goBody), &goValue); err != nil {
		return false
	}
	return true
}

func assertRequestJSONBodyEqual(t *testing.T, pythonBody, goBody string) {
	t.Helper()

	var pythonValue any
	var goValue any
	if err := json.Unmarshal([]byte(pythonBody), &pythonValue); err != nil {
		t.Fatalf("parse python json body: %v", err)
	}
	if err := json.Unmarshal([]byte(goBody), &goValue); err != nil {
		t.Fatalf("parse go json body: %v", err)
	}

	pythonValue = normalizeRequestJSONValue(pythonValue)
	goValue = normalizeRequestJSONValue(goValue)

	if !reflect.DeepEqual(pythonValue, goValue) {
		t.Fatalf("json body mismatch: python=%v go=%v", pythonValue, goValue)
	}
}

func assertRequestQueryEqual(t *testing.T, pythonBody, goBody string) {
	t.Helper()

	pythonValues, err := url.ParseQuery(pythonBody)
	if err != nil {
		t.Fatalf("parse python query: %v", err)
	}
	goValues, err := url.ParseQuery(goBody)
	if err != nil {
		t.Fatalf("parse go query: %v", err)
	}

	pythonNormalized := normalizeRequestQueryValues(pythonValues)
	goNormalized := normalizeRequestQueryValues(goValues)

	if pythonNormalized.Encode() != goNormalized.Encode() {
		t.Fatalf("query mismatch: python=%s go=%s", pythonNormalized.Encode(), goNormalized.Encode())
	}
}

func normalizeRequestQueryValues(values url.Values) url.Values {
	normalized := url.Values{}
	for key, list := range values {
		clean := make([]string, len(list))
		for i, value := range list {
			clean[i] = normalizeRequestQueryValue(value)
		}
		normalized[key] = clean
	}
	return normalized
}

func normalizeRequestQueryValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var parsed any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
			parsed = normalizeRequestJSONValue(parsed)
			if normalized, err := json.Marshal(parsed); err == nil {
				return string(normalized)
			}
		}
	}
	return normalizeAppriseURL(value)
}

func normalizeRequestJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, entry := range typed {
			normalized[key] = normalizeRequestJSONValue(entry)
		}
		return normalized
	case []any:
		normalized := make([]any, len(typed))
		for i, entry := range typed {
			normalized[i] = normalizeRequestJSONValue(entry)
		}
		return normalized
	case string:
		return normalizeAppriseURL(typed)
	default:
		return typed
	}
}

func normalizeAppriseURL(value string) string {
	if strings.HasPrefix(value, appriseUpstreamAssetPrefix) {
		return appriseGoAssetPrefix + strings.TrimPrefix(value, appriseUpstreamAssetPrefix)
	}
	if strings.HasPrefix(value, appriseGoAssetPrefix) {
		return value
	}
	if value == appriseUpstreamRepoURL {
		return appriseGoRepoURL
	}
	return value
}
