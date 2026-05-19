package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/testutil"
	"gopkg.in/yaml.v3"
)

func TestLoadTaggedURLsSkipsMissingAndDirectoryPaths(t *testing.T) {
	dir := t.TempDir()
	configPath := writeConfig(t, "apprise.conf", "tag=json://localhost/one\n")

	tagged := loadTaggedURLs([]string{
		filepath.Join(dir, "missing.conf"),
		dir,
		configPath,
	})

	assertTaggedURLs(t, tagged, []taggedURL{{URL: "json://localhost/one", Tags: []string{"tag"}}})
}

func TestParseConfigFileTextFallbackForYAMLWithoutStructuredConfig(t *testing.T) {
	configPath := writeConfig(t, "apprise.yaml", "tag=json://localhost/one\n")

	tagged := parseConfigFile(configPath)

	assertTaggedURLs(t, tagged, []taggedURL{{URL: "json://localhost/one", Tags: []string{"tag"}}})
}

func TestParseConfigFileMissingFile(t *testing.T) {
	if tagged := parseConfigFile(filepath.Join(t.TempDir(), "missing.conf")); tagged != nil {
		t.Fatalf("expected nil for missing file, got %#v", tagged)
	}
}

func TestParseTextConfigCollectsURLsAndGroups(t *testing.T) {
	cfg := parseTextConfig(`
# ignored
groupA, groupB = tagA, tagB
tagA = json://localhost/one
json://localhost/two
`)

	assertTaggedURLs(t, cfg.URLs, []taggedURL{
		{URL: "json://localhost/one", Tags: []string{"taga"}},
		{URL: "json://localhost/two"},
	})
	assertStringSlices(t, cfg.Groups["groupa"], []string{"taga", "tagb"})
	assertStringSlices(t, cfg.Groups["groupb"], []string{"taga", "tagb"})
}

func TestParseTextConfigReturnsPartialConfigOnScannerError(t *testing.T) {
	cfg := parseTextConfig("tag=json://localhost/one\n" + strings.Repeat("x", 70*1024))

	assertTaggedURLs(t, cfg.URLs, []taggedURL{{URL: "json://localhost/one", Tags: []string{"tag"}}})
}

func TestParseYAMLConfigInvalidAndNonMappingInput(t *testing.T) {
	for _, raw := range []string{"[", "- json://localhost/one"} {
		cfg := parseYAMLConfig([]byte(raw))
		if len(cfg.URLs) != 0 || len(cfg.Groups) != 0 {
			t.Fatalf("expected empty config for %q, got %#v", raw, cfg)
		}
	}
}

func TestParseYAMLConfigSupportsURLShapesAndGlobalTags(t *testing.T) {
	cfg := parseYAMLConfig([]byte(`
tag: global, Team
groups:
  - groupA: tagA
  - groupB:
      - tagB
urls:
  - json://localhost/string
  - url:
      - json://localhost/list-one
      - json://localhost/list-two
    tag: tagA
  - urls: json://localhost/urls-key
    tags:
      - tagB
  - json://localhost/map-key:
      - tag: tagC
  - ignored: value
`))

	byURL := taggedByURL(cfg.URLs)
	assertStringSlices(t, byURL["json://localhost/string"].Tags, []string{"global", "team"})
	assertStringSlices(t, byURL["json://localhost/list-one"].Tags, []string{"global", "taga", "team"})
	assertStringSlices(t, byURL["json://localhost/list-two"].Tags, []string{"global", "taga", "team"})
	assertStringSlices(t, byURL["json://localhost/urls-key"].Tags, []string{"global", "tagb", "team"})
	assertStringSlices(t, byURL["json://localhost/map-key"].Tags, []string{"global", "tagc", "team"})
	if _, ok := byURL["ignored"]; ok {
		t.Fatalf("unexpected non-url YAML entry parsed: %#v", byURL)
	}
	assertStringSlices(t, cfg.Groups["groupa"], []string{"taga"})
	assertStringSlices(t, cfg.Groups["groupb"], []string{"tagb"})
}

func TestParseYAMLHelpersRejectUnsupportedURLShapes(t *testing.T) {
	if urls := parseYAMLURLs(42, nil); urls != nil {
		t.Fatalf("expected nil URLs for scalar url root, got %#v", urls)
	}
	if urls := parseYAMLURLEntry(42, nil); urls != nil {
		t.Fatalf("expected nil URLs for scalar url entry, got %#v", urls)
	}
	if urls := parseYAMLURLValue(42, nil); urls != nil {
		t.Fatalf("expected nil URLs for scalar url value, got %#v", urls)
	}
	if tags := parseYAMLTagsFromOptions(42); tags != nil {
		t.Fatalf("expected nil tags for scalar options, got %#v", tags)
	}
	assertStringSlices(t, parseYAMLTagsFromOptions([]any{map[string]any{"tag": "from-list"}, "invalid"}), []string{"from-list"})
	if tags := parseYAMLTagsFromOptions([]any{"invalid"}); len(tags) != 0 {
		t.Fatalf("expected empty tags for non-map options list, got %#v", tags)
	}
	assertTaggedURLs(t, parseYAMLURLMappedOptions("json://localhost/one", []any{"invalid"}, []string{"global"}), []taggedURL{
		{URL: "json://localhost/one", Tags: []string{"global"}},
	})
}

func TestParseYAMLConfigSupportsURLMap(t *testing.T) {
	cfg := parseYAMLConfig([]byte(`
urls:
  ignored: value
  json://localhost/one:
    tag: tagA
  json://localhost/two:
    tags:
      - tagB
`))

	byURL := taggedByURL(cfg.URLs)
	assertStringSlices(t, byURL["json://localhost/one"].Tags, []string{"taga"})
	assertStringSlices(t, byURL["json://localhost/two"].Tags, []string{"tagb"})
}

func TestParseYAMLConfigMatchesAppriseTagsAliasListExpansion(t *testing.T) {
	cfg := parseYAMLConfig([]byte(`
urls:
  - json://localhost/one:
    - tags: test1
    - tags: test2
  - json://localhost/two:
    - tags: test3
`))

	assertTaggedURLs(t, cfg.URLs, []taggedURL{
		{URL: "json://localhost/one", Tags: []string{"test1"}},
		{URL: "json://localhost/one", Tags: []string{"test2"}},
		{URL: "json://localhost/two", Tags: []string{"test3"}},
	})
}

func TestParseYAMLConfigMatchesAppriseTagPriorityOverTags(t *testing.T) {
	cfg := parseYAMLConfig([]byte(`
urls:
  - json://localhost/one:
      tag: primary
      tags: secondary
`))

	assertTaggedURLs(t, cfg.URLs, []taggedURL{
		{URL: "json://localhost/one", Tags: []string{"primary"}},
	})
}

func TestParseYAMLGroupTagsCoversScalarListAndMapForms(t *testing.T) {
	groups := parseYAMLGroups(mustYAMLValue(t, `
group1: tagA, tagB
group2:
  - tagC
  - tagD: comment
group3:
  tagE: comment
group4: 4
group5: true
group6:
`))

	assertStringSlices(t, groups["group1"], []string{"taga", "tagb"})
	assertStringSlices(t, groups["group2"], []string{"tagc", "tagd"})
	assertStringSlices(t, groups["group3"], []string{"tage"})
	assertStringSlices(t, groups["group4"], []string{"4"})
	assertStringSlices(t, groups["group5"], []string{"true"})
	if groups["group6"] != nil {
		t.Fatalf("expected nil group6 tags, got %#v", groups["group6"])
	}
	assertStringSlices(t, parseYAMLGroupTags(struct{ Name string }{"tagZ"}), []string{"{tagz}"})
}

func TestParseYAMLGroupsIgnoresNonMapListEntries(t *testing.T) {
	groups := parseYAMLGroups([]any{"invalid", map[string]any{"group": "tag"}})
	assertStringSlices(t, groups["group"], []string{"tag"})
}

func TestParseYAMLTagValueCoversScalarsAndLists(t *testing.T) {
	assertStringSlices(t, parseYAMLTagValue([]any{"tagA, tagB", 3, true, nil}), []string{"taga", "tagb", "3", "true"})
	assertStringSlices(t, parseYAMLTagValue(map[string]any{"unexpected": true}), []string{"map[unexpected:true]"})
}

func TestResolveNotifyURLsFiltersTextGroupTags(t *testing.T) {
	configPath := writeConfig(t, "apprise.conf", `
mytagA,mytagB=json://localhost/a
mygrouptag=mytagA
`)

	opts := &cliOptions{
		configPaths: []string{configPath},
		tags:        []string{"mygrouptag"},
	}

	tagged := resolveNotifyURLs(opts, nil, nil)
	if len(tagged) != 1 {
		t.Fatalf("expected one tagged URL, got %d: %#v", len(tagged), tagged)
	}
	if tagged[0].URL != "json://localhost/a" {
		t.Fatalf("expected grouped URL, got %q", tagged[0].URL)
	}
}

func TestResolveNotifyURLsMatchesIssue47TextTagAndGroupRepro(t *testing.T) {
	configPath := writeConfig(t, "apprise.conf", `
mytagA,mytagB=tgram://123456:abcdef/7890/
mygrouptag=mytagA
`)

	for _, tc := range []struct {
		name string
		tag  string
	}{
		{name: "single tag works", tag: "mytagB"},
		{name: "group tag works", tag: "mygrouptag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := &cliOptions{
				configPaths: []string{configPath},
				tags:        []string{tc.tag},
			}

			tagged := resolveNotifyURLs(opts, nil, nil)

			assertTaggedURLs(t, tagged, []taggedURL{
				{URL: "tgram://123456:abcdef/7890/", Tags: []string{"mygrouptag", "mytaga", "mytagb"}},
			})
		})
	}
}

func TestResolveNotifyURLsDoesNotTreatColonTextAsGroupSyntax(t *testing.T) {
	configPath := writeConfig(t, "apprise.conf", `
# Groups
now: now_pers
now_pers: my_now_pers

# Telegram: Personal
my_now_pers=tgram://123456:abcdef/7890/?format=markdown&mdv=v1
`)

	for _, tag := range []string{"now_pers", "now"} {
		opts := &cliOptions{
			configPaths: []string{configPath},
			tags:        []string{tag},
		}
		if tagged := resolveNotifyURLs(opts, nil, nil); len(tagged) != 0 {
			t.Fatalf("expected unsupported colon group %q not to match, got %#v", tag, tagged)
		}
	}

	opts := &cliOptions{
		configPaths: []string{configPath},
		tags:        []string{"my_now_pers"},
	}
	assertTaggedURLs(t, resolveNotifyURLs(opts, nil, nil), []taggedURL{
		{URL: "tgram://123456:abcdef/7890/?format=markdown&mdv=v1", Tags: []string{"my_now_pers"}},
	})
}

func TestResolveNotifyURLsFiltersYAMLGroupTags(t *testing.T) {
	configPath := writeConfig(t, "apprise.yaml", `
version: 1
groups:
  grouptag: mytag
urls:
  - json://localhost/one:
      tag:
        - mytag
  - json://localhost/two:
      tag: other
`)

	opts := &cliOptions{
		configPaths: []string{configPath},
		tags:        []string{"grouptag"},
	}

	tagged := resolveNotifyURLs(opts, nil, nil)
	if len(tagged) != 1 {
		t.Fatalf("expected one tagged URL, got %d: %#v", len(tagged), tagged)
	}
	if tagged[0].URL != "json://localhost/one" {
		t.Fatalf("expected grouped YAML URL, got %q", tagged[0].URL)
	}
}

func TestResolveNotifyURLsMatchesIssue47YAMLTagAndGroupRepro(t *testing.T) {
	configPath := writeConfig(t, "apprise.yaml", `
version: 1
groups:
  grouptag: mytag
urls:
  - tgram://123456:abcdef/7890/:
      tag:
        - mytag
`)

	for _, tc := range []struct {
		name string
		tag  string
	}{
		{name: "yaml tag works", tag: "mytag"},
		{name: "yaml group tag works", tag: "grouptag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := &cliOptions{
				configPaths: []string{configPath},
				tags:        []string{tc.tag},
			}

			tagged := resolveNotifyURLs(opts, nil, nil)

			assertTaggedURLs(t, tagged, []taggedURL{
				{URL: "tgram://123456:abcdef/7890/", Tags: []string{"grouptag", "mytag"}},
			})
		})
	}
}

func TestResolveNotifyURLsMatchesIssue47NestedYAMLGroupRepro(t *testing.T) {
	configPath := writeConfig(t, "apprise.yaml", `
version: 1
groups:
  now: now_pers
  now_pers: my_now_pers
urls:
  - tgram://123456:abcdef/7890/?format=markdown&mdv=v1:
      tag: my_now_pers
`)

	for _, tag := range []string{"my_now_pers", "now_pers", "now"} {
		t.Run(tag, func(t *testing.T) {
			opts := &cliOptions{
				configPaths: []string{configPath},
				tags:        []string{tag},
			}

			tagged := resolveNotifyURLs(opts, nil, nil)

			assertTaggedURLs(t, tagged, []taggedURL{
				{
					URL:  "tgram://123456:abcdef/7890/?format=markdown&mdv=v1",
					Tags: []string{"my_now_pers", "now", "now_pers"},
				},
			})
		})
	}
}

func TestResolveNotifyURLsMatchesPythonNestedGroupParity(t *testing.T) {
	testutil.RequirePythonApprise(t)

	for _, tc := range []struct {
		name     string
		filename string
		content  string
	}{
		{
			name:     "text equals nested groups",
			filename: "apprise.conf",
			content: `
now=now_pers
now_pers=my_now_pers
my_now_pers=json://localhost/one
`,
		},
		{
			name:     "yaml nested groups",
			filename: "apprise.yaml",
			content: `
version: 1
groups:
  now: now_pers
  now_pers: my_now_pers
urls:
  - json://localhost/one:
      tag: my_now_pers
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configPath := writeConfig(t, tc.filename, tc.content)
			tags := []string{"my_now_pers", "now_pers", "now"}
			pythonTags := pythonAppriseResolvedTags(t, configPath, tags)

			for _, tag := range tags {
				opts := &cliOptions{
					configPaths: []string{configPath},
					tags:        []string{tag},
				}
				tagged := resolveNotifyURLs(opts, nil, nil)
				if len(tagged) != len(pythonTags[tag]) {
					t.Fatalf("tag %q URL count mismatch: go=%d python=%d", tag, len(tagged), len(pythonTags[tag]))
				}
				for idx := range tagged {
					assertStringSlices(t, tagged[idx].Tags, pythonTags[tag][idx])
				}
			}
		})
	}
}

func TestResolveNotifyURLsFiltersYAMLTagsAlias(t *testing.T) {
	configPath := writeConfig(t, "apprise.yml", `
version: 1
urls:
  - url: json://localhost/one
    tags: mytag
  - url: json://localhost/two
    tag: other
`)

	opts := &cliOptions{
		configPaths: []string{configPath},
		tags:        []string{"mytag"},
	}

	tagged := resolveNotifyURLs(opts, nil, nil)
	if len(tagged) != 1 {
		t.Fatalf("expected one tagged URL, got %d: %#v", len(tagged), tagged)
	}
	if tagged[0].URL != "json://localhost/one" {
		t.Fatalf("expected YAML tags alias URL, got %q", tagged[0].URL)
	}
}

func TestResolveNotifyURLsUsesEnvURLsAndDefaultConfigPaths(t *testing.T) {
	t.Setenv(defaultEnvAppriseURLs, "tag=json://localhost/env")
	opts := &cliOptions{tags: []string{"tag"}}

	tagged := resolveNotifyURLs(opts, nil, nil)

	assertTaggedURLs(t, tagged, []taggedURL{{URL: "json://localhost/env", Tags: []string{"tag"}}})
}

func TestResolveNotifyURLsIgnoresTagsAndConfigWhenURLsProvided(t *testing.T) {
	var stderr strings.Builder
	opts := &cliOptions{
		configPaths: []string{"ignored.conf"},
		tags:        []string{"tag"},
	}

	tagged := resolveNotifyURLs(opts, []string{"json://localhost/one json://localhost/two"}, &stderr)

	assertTaggedURLs(t, tagged, []taggedURL{
		{URL: "json://localhost/one"},
		{URL: "json://localhost/two"},
	})
	output := stderr.String()
	if !strings.Contains(output, "--tag (-g) entries are ignored") {
		t.Fatalf("expected tag warning, got %q", output)
	}
	if !strings.Contains(output, "Only the URLs will be referenced") {
		t.Fatalf("expected config warning, got %q", output)
	}
}

func TestResolveNotifyURLsUsesEnvURLTokens(t *testing.T) {
	t.Setenv(defaultEnvAppriseURLs, "tag=bare-token")

	tagged := resolveNotifyURLs(&cliOptions{tags: []string{"tag"}}, nil, nil)

	assertTaggedURLs(t, tagged, []taggedURL{{URL: "bare-token", Tags: []string{"tag"}}})
}

func TestParseTaggedLinePreservesUntaggedURLQueryValues(t *testing.T) {
	tagged := parseTaggedLine("json://localhost/path?format=full")
	if len(tagged) != 1 {
		t.Fatalf("expected one URL, got %d: %#v", len(tagged), tagged)
	}
	if tagged[0].URL != "json://localhost/path?format=full" {
		t.Fatalf("expected full URL, got %q", tagged[0].URL)
	}
	if len(tagged[0].Tags) != 0 {
		t.Fatalf("expected no tags, got %#v", tagged[0].Tags)
	}
}

func TestParseTaggedLineHandlesMultipleURLsAndNoURL(t *testing.T) {
	assertTaggedURLs(t, parseTaggedLine("tagA,tagB=json://localhost/one, xml://localhost/two"), []taggedURL{
		{URL: "json://localhost/one", Tags: []string{"taga", "tagb"}},
		{URL: "xml://localhost/two", Tags: []string{"taga", "tagb"}},
	})
	if tagged := parseTaggedLine("tagA="); tagged != nil {
		t.Fatalf("expected nil when no URL is present, got %#v", tagged)
	}
}

func TestParseTextConfigLineSeparatesConfigGrammar(t *testing.T) {
	for _, line := range []string{"", " # comment", "; comment"} {
		if got := parseTextConfigLine(line); len(got.URLs) != 0 || len(got.Groups) != 0 || len(got.GroupTags) != 0 {
			t.Fatalf("expected empty parsed line for %q, got %#v", line, got)
		}
	}

	assertTaggedURLs(t, parseTextConfigLine("tagA=json://localhost/one").URLs, []taggedURL{
		{URL: "json://localhost/one", Tags: []string{"taga"}},
	})

	grouped := parseTextConfigLine("groupA = tagA")
	assertStringSlices(t, grouped.Groups, []string{"groupa"})
	assertStringSlices(t, grouped.GroupTags, []string{"taga"})
	if len(grouped.URLs) != 0 {
		t.Fatalf("expected no URLs for group assignment, got %#v", grouped.URLs)
	}

	if got := parseTextConfigLine("groupA: tagA"); len(got.URLs) != 0 || len(got.Groups) != 0 || len(got.GroupTags) != 0 {
		t.Fatalf("expected unsupported colon assignment to be ignored, got %#v", got)
	}

	assertTaggedURLs(t, parseTextConfigLine("json://localhost/path?format=full").URLs, []taggedURL{
		{URL: "json://localhost/path?format=full"},
	})
}

func TestParseGroupAssignmentValidation(t *testing.T) {
	groups, tags, ok := parseGroupAssignment("groupA, groupB = tagA tagB")
	if !ok {
		t.Fatalf("expected valid group assignment")
	}
	assertStringSlices(t, groups, []string{"groupa", "groupb"})
	assertStringSlices(t, tags, []string{"taga", "tagb"})

	for _, line := range []string{"no equals", "groupC: tagC", "=tag", "group=", "json://localhost/a=tag", "group=json://localhost/a"} {
		if _, _, ok := parseGroupAssignment(line); ok {
			t.Fatalf("expected invalid assignment for %q", line)
		}
	}
	for _, line := range []string{" , = tag", "group = , ,"} {
		if _, _, ok := parseGroupAssignment(line); ok {
			t.Fatalf("expected empty parsed tags to be invalid for %q", line)
		}
	}
}

func TestDetectURLsCoversSchemeAndDelimitedForms(t *testing.T) {
	assertStringSlices(t, detectURLs("json://localhost/one, xml://localhost/two,"), []string{"json://localhost/one", "xml://localhost/two"})
	assertStringSlices(t, detectURLs("one two;three,[four]"), []string{"one", "two", "three", "four"})
	assertStringSlices(t, detectURLs(" \t "), nil)
}

func TestParseTagsAndFilters(t *testing.T) {
	assertStringSlices(t, parseTags(" TagA,tagB\t tagC ,, "), []string{"taga", "tagb", "tagc"})
	assertStringSlices(t, parseTags(" , \t "), nil)
	assertStringSlices(t, parseTags("\r"), nil)

	filters := parseTagFilters([]string{" tagA, tagB ", "", "tagC"})
	if !reflect.DeepEqual(filters, [][]string{{"taga", "tagb"}, {"tagc"}}) {
		t.Fatalf("unexpected filters: %#v", filters)
	}
	if filters := parseTagFilters([]string{","}); len(filters) != 0 {
		t.Fatalf("expected empty filters for delimiter-only value, got %#v", filters)
	}
	if filters := parseTagFilters(nil); filters != nil {
		t.Fatalf("expected nil filters, got %#v", filters)
	}
}

func TestFilterTaggedURLsAndMatchesTagFilters(t *testing.T) {
	urls := []taggedURL{
		{URL: "json://localhost/one", Tags: []string{"tagA", "tagB"}},
		{URL: "json://localhost/two", Tags: []string{"tagC"}},
		{URL: "json://localhost/untagged"},
	}

	assertTaggedURLs(t, filterTaggedURLs(urls, [][]string{{"taga", "tagb"}, {"missing"}}), []taggedURL{
		{URL: "json://localhost/one", Tags: []string{"tagA", "tagB"}},
	})
	if got := filterTaggedURLs(urls, nil); !reflect.DeepEqual(got, urls) {
		t.Fatalf("expected original URLs without filters, got %#v", got)
	}
	if !matchesTagFilters(nil, nil) {
		t.Fatalf("expected empty filters to match")
	}
	if matchesTagFilters(nil, [][]string{{"tag"}}) {
		t.Fatalf("expected tagged filter not to match untagged URL")
	}
	if !matchesTagFilters([]string{"anything"}, [][]string{{}, {"all"}}) {
		t.Fatalf("expected all filter to match")
	}
	if matchesTagFilters([]string{"tagA"}, [][]string{{"tagA", "missing"}}) {
		t.Fatalf("expected strict multi-tag filter not to match")
	}
}

func TestApplyTagGroupsHandlesNestedSelfAndRecursiveGroups(t *testing.T) {
	urls := []taggedURL{
		{URL: "json://localhost/a", Tags: []string{"tagA"}},
		{URL: "json://localhost/b", Tags: []string{"tagB"}},
		{URL: "json://localhost/c", Tags: []string{"tagC"}},
	}
	groups := map[string][]string{
		"groupa": {"tagA"},
		"groupb": {"groupa", "tagB"},
		"groupc": {"groupb"},
		"empty":  nil,
		"self":   {"self"},
		"loopa":  {"loopb"},
		"loopb":  {"loopa"},
	}

	tagged := applyTagGroups(urls, groups)

	assertStringSlices(t, tagged[0].Tags, []string{"groupa", "groupb", "groupc", "taga"})
	assertStringSlices(t, tagged[1].Tags, []string{"groupb", "groupc", "tagb"})
	assertStringSlices(t, tagged[2].Tags, []string{"tagc"})
}

func TestApplyTagGroupsNoopsWithoutURLsOrGroups(t *testing.T) {
	urls := []taggedURL{{URL: "json://localhost/one", Tags: []string{"tag"}}}
	if got := applyTagGroups(nil, map[string][]string{"group": {"tag"}}); got != nil {
		t.Fatalf("expected nil URLs, got %#v", got)
	}
	if got := applyTagGroups(urls, nil); !reflect.DeepEqual(got, urls) {
		t.Fatalf("expected URLs unchanged, got %#v", got)
	}
	if got := sortedTagSet(nil); got != nil {
		t.Fatalf("expected nil sorted tags, got %#v", got)
	}
	assertTaggedURLs(t, applyTagGroups([]taggedURL{{URL: "json://localhost/blank", Tags: []string{"", "tag"}}}, nil), []taggedURL{
		{URL: "json://localhost/blank", Tags: []string{"", "tag"}},
	})
	assertTaggedURLs(t, applyTagGroups([]taggedURL{{URL: "json://localhost/blank", Tags: []string{"", "tag"}}}, map[string][]string{"group": {"tag"}}), []taggedURL{
		{URL: "json://localhost/blank", Tags: []string{"group", "tag"}},
	})
	assertStringSlices(t, mergeTags([]string{"", "tag"}), []string{"tag"})
}

func TestYAMLPathAndPathResolutionHelpers(t *testing.T) {
	for _, path := range []string{"apprise.yml", "apprise.yaml", "APPRISE.YAML"} {
		if !isYAMLConfigPath(path) {
			t.Fatalf("expected YAML path: %s", path)
		}
	}
	if isYAMLConfigPath("apprise.conf") {
		t.Fatalf("did not expect text config to be YAML")
	}

	t.Setenv("APPRISE_GO_TEST_DIR", "expanded")
	assertStringSlices(t, splitPathList("a:b c;d,[e"), []string{"a:b", "c", "d", "e"})
	assertStringSlices(t, expandPaths([]string{"", "$APPRISE_GO_TEST_DIR"}), []string{"expanded"})
	if got := expandPath(""); got != "" {
		t.Fatalf("expected empty path to stay empty, got %q", got)
	}
	if got := expandPath("~"); got == "~" || got == "" {
		t.Fatalf("expected home expansion, got %q", got)
	}
	if got := expandPath("~/apprise-go-test"); !strings.HasSuffix(got, string(filepath.Separator)+"apprise-go-test") {
		t.Fatalf("expected home-relative expansion, got %q", got)
	}
}

func TestLoadConfigPathsPrecedence(t *testing.T) {
	explicit := loadConfigPaths([]string{" explicit.conf "})
	assertStringSlices(t, explicit, []string{"explicit.conf"})

	t.Setenv(defaultEnvAppriseConfigPath, "one.conf two.conf")
	assertStringSlices(t, loadConfigPaths(nil), []string{"one.conf", "two.conf"})

	t.Setenv(defaultEnvAppriseConfigPath, "")
	t.Setenv(defaultEnvAppriseConfigAlias, "alias.conf")
	assertStringSlices(t, loadConfigPaths(nil), []string{"alias.conf"})

	t.Setenv(defaultEnvAppriseConfigAlias, "")
	defaults := loadConfigPaths(nil)
	if len(defaults) == 0 {
		t.Fatalf("expected default config paths")
	}
}

func writeConfig(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func pythonAppriseResolvedTags(t *testing.T, configPath string, tags []string) map[string][][]string {
	t.Helper()

	script := `
import apprise
import json
import sys

config_path = sys.argv[1]
tags = sys.argv[2:]
config = apprise.AppriseConfig(paths=config_path)
app = apprise.Apprise()
app.add(config)
result = {}
for tag in tags:
    result[tag] = [sorted(server.tags) for server in app.find(tag)]
print(json.dumps(result, sort_keys=True))
`
	args := append([]string{"-c", script, configPath}, tags...)
	stdout, stderr, err := testutil.RunCommand(t, testutil.PythonPath(t), args...)
	if err != nil {
		t.Fatalf("python apprise config resolution failed: %v stdout=%s stderr=%s", err, strings.TrimSpace(stdout), strings.TrimSpace(stderr))
	}

	var resolved map[string][][]string
	if err := json.Unmarshal([]byte(stdout), &resolved); err != nil {
		t.Fatalf("decode python apprise config resolution: %v stdout=%s", err, stdout)
	}
	return resolved
}

func mustYAMLValue(t *testing.T, raw string) any {
	t.Helper()
	var value any
	if err := yaml.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatalf("unmarshal YAML: %v", err)
	}
	return value
}

func taggedByURL(urls []taggedURL) map[string]taggedURL {
	byURL := make(map[string]taggedURL, len(urls))
	for _, entry := range urls {
		byURL[entry.URL] = entry
	}
	return byURL
}

func assertTaggedURLs(t *testing.T, got []taggedURL, want []taggedURL) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tagged URLs mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func assertStringSlices(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(want) == 0 {
		if len(got) != 0 {
			t.Fatalf("string slice mismatch\ngot:  %#v\nwant: %#v", got, want)
		}
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("string slice mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}
