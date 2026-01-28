//go:build e2e

package parity

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
	"github.com/unraid/apprise-go/internal/testutil"
)

type schemaExerciseCase struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	URL    string `json:"url"`
}

const (
	defaultBody  = "apprise parity body"
	defaultTitle = "apprise parity title"
)

func bodyForExerciseCase(name string) string {
	if strings.HasPrefix(name, "choice-overflow-") {
		return strings.Repeat("apprise overflow ", 400)
	}
	return defaultBody
}

func loadSchemaExerciseCases(t *testing.T, schemas []string) []schemaExerciseCase {
	t.Helper()

	appriseRoot := testutil.AppriseSourceRoot(t)
	t.Setenv("PYTHONPATH", appriseRoot)

	script := filepath.Join(testutil.RepoRoot(t), "internal", "testutil", "scripts", "schema_exercise_cases.py")
	stdout, stderr, err := testutil.RunPythonScript(t, script, schemas...)
	if err != nil {
		t.Fatalf("schema exercise cases failed: %v (stderr: %s)", err, strings.TrimSpace(stderr))
	}

	var cases []schemaExerciseCase
	if err := json.Unmarshal([]byte(stdout), &cases); err != nil {
		t.Fatalf("decode schema exercise cases: %v (stdout: %s)", err, strings.TrimSpace(stdout))
	}

	return cases
}

func TestE2ERequestParitySchemaExercise(t *testing.T) {
	schemas := notify.SupportedSchemas()
	if len(schemas) == 0 {
		t.Fatalf("no supported schemas discovered")
	}

	cases := loadSchemaExerciseCases(t, schemas)
	if len(cases) == 0 {
		t.Fatalf("no schema exercise cases generated")
	}

	defs := loadProviderDefinitions(t)
	providerBySchema := schemaProviderMap(defs)

	seen := map[string]int{}
	for _, c := range cases {
		schema := strings.ToLower(strings.TrimSpace(c.Schema))
		if !notify.SupportsSchema(schema) {
			continue
		}
		seen[schema]++

		name := c.Name
		if strings.TrimSpace(name) == "" {
			name = "case"
		}

		t.Run(schema+"/"+name, func(t *testing.T) {
			maybeParallel(t)
			provider, ok := providerBySchema[schema]
			if !ok {
				t.Fatalf("no provider manifest for schema %s", schema)
			}
			builder, ok := providerBuilders[provider]
			if !ok {
				t.Fatalf("missing provider builder for %s (schema %s)", provider, schema)
			}

			logProgress(t, "python-vs-go "+schema+"/"+name)
			body := bodyForExerciseCase(c.Name)
			pythonSpecs, pythonSuccess := testutil.CapturePythonRequestsWithTypeResult(
				t,
				c.URL,
				body,
				defaultTitle,
				notify.NotifyInfo,
			)

			parsedURL, err := notify.ParseURL(c.URL)
			if err != nil {
				if pythonSuccess != nil && !*pythonSuccess {
					return
				}
				t.Fatalf("parse url: %v", err)
			}
			target, err := builder(parsedURL)
			if err != nil {
				if pythonSuccess != nil && !*pythonSuccess {
					return
				}
				if len(pythonSpecs) == 0 {
					return
				}
				t.Fatalf("build target: %v", err)
			}
			goSpecs, err := testutil.CaptureGoRequestsResult(t, func() error {
				return target.Send(body, defaultTitle, notify.NotifyInfo)
			})
			if shouldSkip := assertNotifySuccessMatches(t, pythonSuccess, err); shouldSkip {
				return
			}
			if err != nil {
				t.Fatalf("send request failed: %v", err)
			}

			assertRequestSpecSequenceMatches(t, pythonSpecs, goSpecs)
		})
	}

	for _, schema := range schemas {
		if seen[strings.ToLower(schema)] == 0 {
			t.Fatalf("missing exercise cases for schema %s", schema)
		}
	}
}
