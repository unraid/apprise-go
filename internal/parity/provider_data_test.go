package parity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/testutil"
)

const providerRoot = "internal/parity/providers"

type providerManifest struct {
	Name    string   `json:"name"`
	Schemas []string `json:"schemas"`

	// VolatileHeaders names headers whose value cannot be reproduced across
	// two runs — SOGS signs a random nonce and the current time, so its
	// X-SOGS-* headers differ every request. They are asserted present and
	// non-empty rather than equal. Nothing else re-checks their contents, so
	// a provider listing one owes a pinned vector test proving how it is
	// built; see sogs_vectors_test.go.
	VolatileHeaders []string `json:"volatile_headers"`
}

type providerCase struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Body  string `json:"body"`
	Title string `json:"title"`
	Type  string `json:"type"`

	// SendsNothing marks a case where upstream deliberately issues no request
	// at all — an Opsgenie note against an alert that was never created, for
	// instance. Sending nothing is a real behavior worth pinning, but an
	// empty golden otherwise means the capture silently failed, so the case
	// has to say which one it is.
	SendsNothing bool `json:"sends_nothing"`

	// Attachments name files to send with the notification. They are paths
	// relative to the repository, written with a {repo} prefix so the fixture
	// stays portable.
	Attachments []string `json:"attachments"`

	// VolatileHeaders names headers this case alone cannot reproduce, added to
	// whatever the manifest already lists. SES signs the request body, and a
	// body carrying attachments carries a randomly generated MIME boundary, so
	// the two sides sign different bytes by construction. Everything else about
	// the request is still compared, and ses_signature_test.go pins how the
	// signature is built.
	VolatileHeaders []string `json:"volatile_headers"`

	// AttachmentDropped marks a case where the service deliberately refuses
	// the attachment — an image-only service handed a PDF, or an upload that
	// needs credentials the URL does not carry. The request is still made,
	// just without the file.
	AttachmentDropped bool `json:"attachment_dropped"`
}

type providerDefinition struct {
	Name            string
	Schemas         []string
	Cases           []providerCase
	Dir             string
	VolatileHeaders []string
}

func loadProviderDefinitions(t *testing.T) map[string]providerDefinition {
	t.Helper()

	root := filepath.Join(testutil.RepoRoot(t), providerRoot)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read provider root: %v", err)
	}

	defs := make(map[string]providerDefinition)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		providerDir := filepath.Join(root, name)
		manifest := loadProviderManifest(t, providerDir, name)
		cases := loadProviderCases(t, providerDir, name)

		def := providerDefinition{
			Name:            name,
			Schemas:         manifest.Schemas,
			Cases:           cases,
			Dir:             providerDir,
			VolatileHeaders: manifest.VolatileHeaders,
		}

		if _, exists := defs[name]; exists {
			t.Fatalf("duplicate provider definition for %s", name)
		}
		defs[name] = def
	}

	if len(defs) == 0 {
		t.Fatalf("no providers found under %s", root)
	}

	return defs
}

func loadProviderManifest(t *testing.T, providerDir, providerName string) providerManifest {
	t.Helper()

	manifestPath := filepath.Join(providerDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest %s: %v", manifestPath, err)
	}

	var manifest providerManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse manifest %s: %v", manifestPath, err)
	}

	if manifest.Name != "" && manifest.Name != providerName {
		t.Fatalf("manifest name %s does not match folder %s", manifest.Name, providerName)
	}
	manifest.Name = providerName

	if len(manifest.Schemas) == 0 {
		t.Fatalf("manifest %s missing schemas", manifestPath)
	}

	for i, schema := range manifest.Schemas {
		trimmed := strings.TrimSpace(schema)
		if trimmed == "" {
			t.Fatalf("manifest %s contains empty schema", manifestPath)
		}
		manifest.Schemas[i] = strings.ToLower(trimmed)
	}

	sort.Strings(manifest.Schemas)

	return manifest
}

func loadProviderCases(t *testing.T, providerDir, providerName string) []providerCase {
	t.Helper()

	casesPath := filepath.Join(providerDir, "cases.json")
	data, err := os.ReadFile(casesPath)
	if err != nil {
		t.Fatalf("read cases %s: %v", casesPath, err)
	}

	var cases []providerCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("parse cases %s: %v", casesPath, err)
	}

	if len(cases) == 0 {
		t.Fatalf("cases %s empty", casesPath)
	}

	// A case that points at a checked-in file — a payload template, say —
	// needs an absolute path, because apprise resolves a relative one against
	// the home directory rather than the working directory. {repo} keeps the
	// fixture portable and is expanded before either side sees the URL.
	repoRoot := testutil.RepoRoot(t)
	expand := func(value string) string {
		value = strings.ReplaceAll(value, "%7Brepo%7D", repoRoot)

		return strings.ReplaceAll(value, "{repo}", repoRoot)
	}
	for i := range cases {
		cases[i].URL = expand(cases[i].URL)
		for j := range cases[i].Attachments {
			cases[i].Attachments[j] = expand(cases[i].Attachments[j])
		}
	}

	seen := map[string]struct{}{}
	for _, c := range cases {
		if strings.TrimSpace(c.Name) == "" {
			t.Fatalf("cases %s contains empty name", casesPath)
		}
		if _, exists := seen[c.Name]; exists {
			t.Fatalf("cases %s has duplicate name %s", casesPath, c.Name)
		}
		seen[c.Name] = struct{}{}
		if strings.TrimSpace(c.URL) == "" {
			t.Fatalf("cases %s missing url for %s", casesPath, c.Name)
		}
	}

	return cases
}

func sortedProviderNames(defs map[string]providerDefinition) []string {
	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedKeys(input map[string]string) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func formatList(values []string) string {
	if len(values) == 0 {
		return ""
	}
	sort.Strings(values)
	return fmt.Sprintf("[%s]", strings.Join(values, ", "))
}

// caseVolatileHeaders merges the headers a provider always fails to reproduce
// with the ones only this case does.
func caseVolatileHeaders(def providerDefinition, c providerCase) []string {
	if len(c.VolatileHeaders) == 0 {
		return def.VolatileHeaders
	}

	merged := make([]string, 0, len(def.VolatileHeaders)+len(c.VolatileHeaders))
	merged = append(merged, def.VolatileHeaders...)

	return append(merged, c.VolatileHeaders...)
}
