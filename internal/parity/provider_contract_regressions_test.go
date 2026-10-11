package parity

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

// TestProviderRegressionRequestParity compares complete Send sequences and
// delivery outcomes for the template, recipient, and optional-target fixes.
func TestProviderRegressionRequestParity(t *testing.T) {
	testutil.RequirePythonApprise(t)
	tests := []struct {
		name string
		url  string
	}{
		{"mailersend/copy-and-reply", "mailersend://apikey123:user@example.com?cc=bad,copy@example.com&bcc=bad,blind@example.com&reply=user@example.com"},
		{"mailersend/bracketed-copies", "mailersend://apikey123:user@example.com/one@example.com/two@example.com?cc=bad,%3Ccopy@example.com%3E,%3Cone@example.com%3E&bcc=bad,%3Cblind@example.com%3E&reply=Reply%20Name%20%3Cuser@example.com%3E"},
		{"postmark/bracketed-copies", "postmark://apikey123:sender@example.com/one@example.com/two@example.com?cc=bad,%3Cone@example.com%3E&bcc=bad,%3Cblind@example.com%3E"},
		{"postmark/copy-exclusions", "postmark://apikey123:sender@example.com/one@example.com/two@example.com?cc=bad,%3Ccopy@example.com%3E,%3Cone@example.com%3E,%3Cblind@example.com%3E&bcc=bad,%3Cblind@example.com%3E,%3Ctwo@example.com%3E"},
		{"postmark/blind-copy-exclusions", "postmark://apikey123:sender@example.com/one@example.com?cc=bad,%3Cblind@example.com%3E,%3Cone@example.com%3E&bcc=bad,%3Cone@example.com%3E"},
		{"mailersend/colon-and-duplicate-copies", "mailersend://apikey123:sender@example.com/one@example.com?cc=Ops%3Acopy@example.com,%3Ccopy@example.com%3E,Copy@example.com,One@example.com&bcc=Blind%3Ablind@example.com,%3Cblind@example.com%3E&reply=Reply%3Areply@example.com"},
		{"postmark/case-sensitive-copy-exclusions", "postmark://apikey123:sender@example.com/one@example.com?cc=copy@example.com,One@example.com&bcc=Copy@example.com"},
		{"postmark/no-valid-recipients", "postmark://apikey123:sender@example.com/invalid"},
		{"kook/empty-required", "kook://bottoken123?optional=no"},
		{"kook/empty-optional", "kook://bottoken123?optional=yes"},
		{"kook/invalid-required", "kook://bottoken123/invalid-channel?optional=no"},
		{"kook/invalid-optional", "kook://bottoken123/invalid-channel?optional=yes"},
	}
	for _, provider := range []struct {
		name  string
		url   string
		valid string
	}{
		{"slack", "slack://T111/F222/X333/Y444?mode=workflow&template=", `{"blocks":[{"type":"section","text":{"type":"mrkdwn","text":"{{app_title}}: {{app_body}}"}}]}`},
		{"discord", "discord://1234567890/abcdefghijklmnopqrstuvwxyz?template=", `{"content":"{{app_title}}: {{app_body}}","embeds":[]}`},
		{"guilded", "guilded://1234567890/abcdefghijklmnopqrstuvwxyz?template=", `{"content":"{{app_title}}: {{app_body}}","embeds":[]}`},
	} {
		payloads := []string{provider.valid, `{}`, `{"attachments":[{}]}`, `{"content":42,"embeds":["bad"]}`, `{"blocks":[{}]}`}
		for i, payload := range payloads {
			path := filepath.Join(t.TempDir(), "template.json")
			if err := os.WriteFile(path, []byte(payload), 0600); err != nil {
				t.Fatal(err)
			}
			label := "invalid"
			if i == 0 {
				label = "valid"
			}
			tests = append(tests, struct {
				name string
				url  string
			}{provider.name + "/" + label + "/" + payload, provider.url + url.QueryEscape(path)})
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pythonSpecs, pythonSuccess := testutil.CapturePythonRequestsWithTypeResult(t, tt.url, "body", "title", "", notify.NotifyInfo)
			parsed, err := notify.ParseURL(tt.url)
			if err != nil {
				t.Fatal(err)
			}
			goSpecs, err := testutil.CaptureGoRequestsResult(t, func() error {
				target, err := notify.NewTarget(parsed)
				if err != nil {
					return err
				}
				return notify.DispatchSendWithInput(target, parsed, "body", "title", "", notify.NotifyInfo, nil)
			})
			if pythonSuccess == nil {
				t.Fatal("Python capture omitted the delivery outcome")
			}
			assertNotifySuccessMatches(t, pythonSuccess, err)
			if strings.HasPrefix(tt.name, "mailersend/") {
				normalizeMailerSendCopyOrder(t, pythonSpecs)
				normalizeMailerSendCopyOrder(t, goSpecs)
			}
			if strings.HasPrefix(tt.name, "postmark/") {
				normalizePostmarkCopyOrder(t, pythonSpecs)
				normalizePostmarkCopyOrder(t, goSpecs)
			}
			assertRequestSpecSequenceMatches(t, pythonSpecs, goSpecs)
		})
	}
}

// Python Postmark serializes Cc/Bcc sets in arbitrary order. Sort only those
// captured lists; retain every address and duplicate, and compare all other data.
func normalizePostmarkCopyOrder(t *testing.T, specs []notify.RequestSpec) {
	t.Helper()
	for i := range specs {
		var payload map[string]json.RawMessage
		if err := json.Unmarshal([]byte(specs[i].Body), &payload); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"Cc", "Bcc"} {
			raw, exists := payload[field]
			if !exists {
				continue
			}
			var list string
			if err := json.Unmarshal(raw, &list); err != nil {
				t.Fatal(err)
			}
			addresses := strings.Split(list, ",")
			slices.Sort(addresses)
			data, err := json.Marshal(strings.Join(addresses, ","))
			if err != nil {
				t.Fatal(err)
			}
			payload[field] = data
		}
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		specs[i].Body = string(data)
	}
}

// Python MailerSend serializes Cc/Bcc sets in arbitrary order. Sort only those
// captured arrays by the complete recipient object, retaining all fields and
// duplicate entries; primary recipients, attachments and request order stay exact.
func normalizeMailerSendCopyOrder(t *testing.T, specs []notify.RequestSpec) {
	t.Helper()
	for i := range specs {
		var payload map[string]json.RawMessage
		if err := json.Unmarshal([]byte(specs[i].Body), &payload); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"cc", "bcc"} {
			raw, exists := payload[field]
			if !exists {
				continue
			}
			var recipients []map[string]json.RawMessage
			if err := json.Unmarshal(raw, &recipients); err != nil {
				t.Fatal(err)
			}
			if recipients == nil {
				continue
			}
			objects := make([]json.RawMessage, len(recipients))
			for j, recipient := range recipients {
				data, err := json.Marshal(recipient)
				if err != nil {
					t.Fatal(err)
				}
				objects[j] = data
			}
			slices.SortFunc(objects, func(a, b json.RawMessage) int {
				return strings.Compare(string(a), string(b))
			})
			data, err := json.Marshal(objects)
			if err != nil {
				t.Fatal(err)
			}
			payload[field] = data
		}
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		specs[i].Body = string(data)
	}
}

func TestNormalizeMailerSendCopyOrderPreservesPayload(t *testing.T) {
	specs := []notify.RequestSpec{
		{
			Method: "POST", URL: "https://api.mailersend.com/v1/email",
			Headers: map[string]string{"Authorization": "Bearer exact-token", "X-Fixture": "First"},
			Body:    `{"cc":[{"email":"copy@example.com","name":"Lower","meta":{"id":9007199254740993}},{"name":"Upper","email":"Copy@example.com"},{"email":"copy@example.com","name":"Lower","meta":{"id":9007199254740993}}],"bcc":[{"email":"hidden@example.com","name":"Second"},{"name":"First","email":"hidden@example.com"}],"to":[{"email":"two@example.com"},{"email":"one@example.com"}],"attachments":[{"filename":"second.txt"},{"filename":"first.txt"}],"subject":"Exact subject","text":"Exact body"}`,
		},
		{
			Method: "POST", URL: "https://api.mailersend.com/v1/email",
			Headers: map[string]string{"Authorization": "Bearer exact-token", "X-Fixture": "Second"},
			Body:    `{"to":[{"email":"last@example.com"}],"subject":"Second request","cc":[],"bcc":null}`,
		},
	}
	expected := []notify.RequestSpec{
		{
			Method: "POST", URL: "https://api.mailersend.com/v1/email",
			Headers: map[string]string{"Authorization": "Bearer exact-token", "X-Fixture": "First"},
			Body:    `{"cc":[{"email":"Copy@example.com","name":"Upper"},{"email":"copy@example.com","name":"Lower","meta":{"id":9007199254740993}},{"email":"copy@example.com","name":"Lower","meta":{"id":9007199254740993}}],"bcc":[{"email":"hidden@example.com","name":"First"},{"email":"hidden@example.com","name":"Second"}],"to":[{"email":"two@example.com"},{"email":"one@example.com"}],"attachments":[{"filename":"second.txt"},{"filename":"first.txt"}],"subject":"Exact subject","text":"Exact body"}`,
		},
		{
			Method: "POST", URL: "https://api.mailersend.com/v1/email",
			Headers: map[string]string{"Authorization": "Bearer exact-token", "X-Fixture": "Second"},
			Body:    `{"to":[{"email":"last@example.com"}],"subject":"Second request","cc":[],"bcc":null}`,
		},
	}
	normalizeMailerSendCopyOrder(t, specs)
	assertRequestSpecSequenceMatches(t, expected, specs)
	for i := range specs {
		if specs[i].Method != expected[i].Method || specs[i].URL != expected[i].URL || !reflect.DeepEqual(specs[i].Headers, expected[i].Headers) {
			t.Fatalf("request %d transport fields changed: %#v", i, specs[i])
		}
	}
	if !strings.Contains(specs[0].Body, "9007199254740993") {
		t.Fatal("recipient metadata lost integer precision")
	}
}
