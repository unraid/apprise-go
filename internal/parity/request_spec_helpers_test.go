package parity

import (
	"encoding/json"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
)

var headerDrop = map[string]struct{}{
	"x-apprise-id":              {},
	"x-apprise-recursion-count": {},
}

var headerKeep = map[string]struct{}{
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

func assertRequestSpecMatches(t *testing.T, pythonSpec, goSpec notify.RequestSpec) {
	t.Helper()

	assertRequestSpecMatchesExcept(t, pythonSpec, goSpec, nil)
}

// assertRequestSpecMatchesExcept compares two requests, treating the named
// headers as volatile: present and non-empty on both sides, but not equal.
func assertRequestSpecMatchesExcept(t *testing.T, pythonSpec, goSpec notify.RequestSpec, volatile []string) {
	t.Helper()

	if !strings.EqualFold(pythonSpec.Method, goSpec.Method) {
		t.Fatalf("method mismatch: python=%s go=%s", pythonSpec.Method, goSpec.Method)
	}

	pythonBody := normalizeBody(pythonSpec)
	goBody := normalizeBody(goSpec)

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
	pythonQuery := normalizeQueryValues(pythonURL.Query()).Encode()
	goQuery := normalizeQueryValues(goURL.Query()).Encode()
	if pythonQuery != goQuery {
		t.Fatalf("url query mismatch: python=%s go=%s", pythonQuery, goQuery)
	}

	// A multipart boundary is generated per request and never matches across
	// two runs, so both sides are rewritten to a fixed one. Everything the
	// boundary separates is still compared.
	pythonBody, pythonSpec.Headers = normalizeMultipart(pythonBody, pythonSpec.Headers)
	goBody, goSpec.Headers = normalizeMultipart(goBody, goSpec.Headers)

	pythonHeaders := normalizeHeaders(pythonSpec.Headers)
	goHeaders := normalizeHeaders(goSpec.Headers)
	for _, name := range volatile {
		name = strings.ToLower(strings.TrimSpace(name))
		for side, headers := range map[string]map[string]string{"python": pythonHeaders, "go": goHeaders} {
			if strings.TrimSpace(headers[name]) == "" {
				t.Fatalf("volatile header %s missing or empty on %s side", name, side)
			}
			delete(headers, name)
		}
	}
	if !reflect.DeepEqual(pythonHeaders, goHeaders) {
		t.Fatalf("header mismatch: python=%v go=%v", pythonHeaders, goHeaders)
	}

	if shouldCompareJSON(pythonHeaders) && strings.TrimSpace(pythonBody) != "" && strings.TrimSpace(goBody) != "" {
		assertJSONBodyEqual(t, pythonBody, goBody)
		return
	}
	if shouldCompareBodyAsJSON(pythonBody, goBody) {
		assertJSONBodyEqual(t, pythonBody, goBody)
		return
	}
	if shouldCompareForm(pythonHeaders, pythonBody) {
		assertQueryEqual(t, pythonBody, goBody)
		return
	}

	if isMultipartBody(pythonHeaders) {
		assertMultipartBodyEqual(t, pythonBody, goBody)

		return
	}

	if pythonBody != goBody {
		t.Fatalf("body mismatch: python=%s go=%s", pythonBody, goBody)
	}
}

func assertRequestSpecSequenceMatches(t *testing.T, pythonSpecs, goSpecs []notify.RequestSpec) {
	t.Helper()

	assertRequestSpecSequenceMatchesExcept(t, pythonSpecs, goSpecs, nil)
}

func assertRequestSpecSequenceMatchesExcept(t *testing.T, pythonSpecs, goSpecs []notify.RequestSpec, volatile []string) {
	t.Helper()

	if len(pythonSpecs) != len(goSpecs) {
		t.Fatalf("request count mismatch: python=%d go=%d", len(pythonSpecs), len(goSpecs))
	}

	for i := range pythonSpecs {
		assertRequestSpecMatchesExcept(t, pythonSpecs[i], goSpecs[i], volatile)
	}
}

func logProgress(t *testing.T, label string) {
	t.Helper()
	t.Logf("parity: %s", label)
}

func assertNotifySuccessMatches(t *testing.T, pythonSuccess *bool, err error) bool {
	t.Helper()

	if pythonSuccess == nil {
		return false
	}

	goSuccess := err == nil
	if *pythonSuccess != goSuccess {
		t.Fatalf("notify success mismatch: python=%v goErr=%v", *pythonSuccess, err)
	}

	return !goSuccess
}

func normalizeHeaders(headers map[string]string) map[string]string {
	normalized := map[string]string{}
	for key, value := range headers {
		lower := strings.ToLower(key)
		if _, drop := headerDrop[lower]; drop {
			continue
		}
		if _, keep := headerKeep[lower]; keep || strings.HasPrefix(lower, "x-") {
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

func normalizeBody(spec notify.RequestSpec) string {
	body := spec.Body
	if strings.TrimSpace(body) == "" {
		return ""
	}
	if strings.EqualFold(spec.Method, "GET") && strings.TrimSpace(body) == "null" {
		return ""
	}
	return body
}

func shouldCompareJSON(headers map[string]string) bool {
	contentType := strings.ToLower(headers["content-type"])
	return strings.Contains(contentType, "application/json")
}

func shouldCompareForm(headers map[string]string, body string) bool {
	contentType := strings.ToLower(headers["content-type"])
	if !strings.Contains(contentType, "application/x-www-form-urlencoded") {
		return false
	}
	return strings.Contains(body, "=")
}

func shouldCompareBodyAsJSON(pythonBody, goBody string) bool {
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

func assertJSONBodyEqual(t *testing.T, pythonBody, goBody string) {
	t.Helper()

	var pythonValue any
	var goValue any
	if err := json.Unmarshal([]byte(pythonBody), &pythonValue); err != nil {
		t.Fatalf("parse python json body: %v", err)
	}
	if err := json.Unmarshal([]byte(goBody), &goValue); err != nil {
		t.Fatalf("parse go json body: %v", err)
	}

	pythonValue = normalizeJSONValue(pythonValue)
	goValue = normalizeJSONValue(goValue)

	if !reflect.DeepEqual(pythonValue, goValue) {
		t.Fatalf("json body mismatch: python=%v go=%v", pythonValue, goValue)
	}
}

func assertQueryEqual(t *testing.T, pythonBody, goBody string) {
	t.Helper()

	pythonValues, err := url.ParseQuery(pythonBody)
	if err != nil {
		t.Fatalf("parse python query: %v", err)
	}
	goValues, err := url.ParseQuery(goBody)
	if err != nil {
		t.Fatalf("parse go query: %v", err)
	}

	pythonNormalized := normalizeQueryValues(pythonValues)
	goNormalized := normalizeQueryValues(goValues)

	if pythonNormalized.Encode() != goNormalized.Encode() {
		t.Fatalf("query mismatch: python=%s go=%s", pythonNormalized.Encode(), goNormalized.Encode())
	}
}

func normalizeQueryValues(values url.Values) url.Values {
	normalized := url.Values{}
	for key, list := range values {
		clean := make([]string, len(list))
		for i, value := range list {
			clean[i] = normalizeQueryValue(value)
		}
		normalized[key] = clean
	}
	return normalized
}

func normalizeQueryValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var parsed any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
			parsed = normalizeJSONValue(parsed)
			if normalized, err := json.Marshal(parsed); err == nil {
				return string(normalized)
			}
		}
	}
	return normalizeAppriseURL(value)
}

func normalizeJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, entry := range typed {
			normalized[key] = normalizeJSONValue(entry)
		}
		return normalized
	case []any:
		normalized := make([]any, len(typed))
		for i, entry := range typed {
			normalized[i] = normalizeJSONValue(entry)
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

// multipartBoundaryPattern finds the boundary in a content type header.
var multipartBoundaryPattern = regexp.MustCompile(`boundary=([^;]+)`)

const multipartFixedBoundary = "APPRISE-PARITY-BOUNDARY"

// normalizeMultipart replaces a generated multipart boundary with a fixed one
// in both the header and the body, so two runs of the same request compare
// equal. The parts themselves are untouched.
func normalizeMultipart(body string, headers map[string]string) (string, map[string]string) {
	contentType := ""
	contentTypeKey := ""
	for key, value := range headers {
		if strings.EqualFold(key, "content-type") {
			contentType, contentTypeKey = value, key
			break
		}
	}

	if !strings.Contains(strings.ToLower(contentType), "multipart/") {
		return body, headers
	}

	matches := multipartBoundaryPattern.FindStringSubmatch(contentType)
	if len(matches) < 2 {
		return body, headers
	}
	boundary := strings.Trim(matches[1], `"`)
	if boundary == "" {
		return body, headers
	}

	rewritten := map[string]string{}
	for key, value := range headers {
		rewritten[key] = value
	}
	rewritten[contentTypeKey] = multipartBoundaryPattern.ReplaceAllString(
		contentType, "boundary="+multipartFixedBoundary)

	return strings.ReplaceAll(body, boundary, multipartFixedBoundary), rewritten
}

func isMultipartBody(headers map[string]string) bool {
	return strings.Contains(strings.ToLower(headers["content-type"]), "multipart/")
}

// assertMultipartBodyEqual compares two multipart bodies part by part. A part
// carrying JSON is compared structurally, since key order and whitespace
// differ between the two encoders and neither is meaningful.
func assertMultipartBodyEqual(t *testing.T, pythonBody, goBody string) {
	t.Helper()

	pythonParts := splitMultipartParts(pythonBody)
	goParts := splitMultipartParts(goBody)

	if len(pythonParts) != len(goParts) {
		t.Fatalf("multipart part count mismatch: python=%d go=%d\npython=%s\ngo=%s",
			len(pythonParts), len(goParts), pythonBody, goBody)
	}

	for i := range pythonParts {
		pythonHeader, pythonContent := pythonParts[i].header, pythonParts[i].content
		goHeader, goContent := goParts[i].header, goParts[i].content

		if pythonHeader != goHeader {
			t.Fatalf("multipart part %d header mismatch:\npython=%s\ngo=%s", i, pythonHeader, goHeader)
		}

		if shouldCompareBodyAsJSON(pythonContent, goContent) {
			assertJSONBodyEqual(t, pythonContent, goContent)
			continue
		}
		if pythonContent != goContent {
			t.Fatalf("multipart part %d content mismatch:\npython=%s\ngo=%s", i, pythonContent, goContent)
		}
	}
}

type multipartPart struct {
	header  string
	content string
}

// splitMultipartParts breaks a body on the normalized boundary and separates
// each part's headers from its content.
func splitMultipartParts(body string) []multipartPart {
	parts := []multipartPart{}
	for _, chunk := range strings.Split(body, "--"+multipartFixedBoundary) {
		trimmed := strings.Trim(chunk, "-\r\n")
		if trimmed == "" {
			continue
		}

		header, content, found := strings.Cut(strings.TrimLeft(chunk, "\r\n"), "\r\n\r\n")
		if !found {
			header, content, found = strings.Cut(strings.TrimLeft(chunk, "\n"), "\n\n")
		}
		if !found {
			continue
		}

		parts = append(parts, multipartPart{
			header:  strings.TrimSpace(header),
			content: strings.Trim(content, "-\r\n"),
		})
	}

	return parts
}
