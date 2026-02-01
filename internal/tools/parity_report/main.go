package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/unraid/apprise-go/internal/notify"
)

type testEvent struct {
	Time    string  `json:"Time"`
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

type testResult struct {
	Status  string
	Elapsed float64
}

type report struct {
	GeneratedAt    time.Time
	Package        string
	PackageStatus  string
	Tests          map[string]testResult
	TopLevel       map[string]testResult
	PassedCount    int
	FailedCount    int
	SkippedCount   int
	AppriseRoot    string
	PythonSchemas  []string
	GoSchemas      []string
	MissingSchemas []string
	ExtraSchemas   []string
	SchemaDiffErr  string
	HTTPSchemas    []string
	NonHTTPSchemas []string
	SchemaCoverage string
	GoldenCheck    string
	ProviderParity string
	GoldenParity   string
	ProviderCount  int
}

func main() {
	var (
		outPath  string
		jsonPath string
		pkg      string
	)
	flag.StringVar(&outPath, "out", "reports/parity_report.md", "path to write markdown report")
	flag.StringVar(&jsonPath, "json", "reports/parity_report.json", "path to write raw go test -json output")
	flag.StringVar(&pkg, "pkg", "./internal/parity", "go test package to run")
	flag.Parse()

	rep, raw, err := runParityTests(pkg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	if jsonPath != "" {
		if err := os.MkdirAll(filepath.Dir(jsonPath), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "create json output dir: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(jsonPath, raw, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write json output: %v\n", err)
			os.Exit(1)
		}
	}

	if outPath != "" {
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "create report dir: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(outPath, []byte(renderMarkdown(rep, jsonPath)), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write report: %v\n", err)
			os.Exit(1)
		}
	}

	if err := schemaDiffError(rep); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runParityTests(pkg string) (report, []byte, error) {
	cmd := exec.Command("go", "test", pkg, "-count=1", "-json", "-v")
	cmd.Env = os.Environ()
	if os.Getenv("GOCACHE") == "" {
		if dir, err := os.MkdirTemp("", "gocache"); err == nil {
			defer os.RemoveAll(dir)
			cmd.Env = append(cmd.Env, "GOCACHE="+dir)
		}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return report{}, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return report{}, nil, fmt.Errorf("start go test: %w", err)
	}

	raw := &bytes.Buffer{}
	rep := report{
		GeneratedAt: time.Now(),
		Package:     pkg,
		Tests:       map[string]testResult{},
		TopLevel:    map[string]testResult{},
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Bytes()
		raw.Write(line)
		raw.WriteByte('\n')

		var event testEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}

		if event.Action == "output" && event.Output != "" {
			fmt.Fprint(os.Stdout, event.Output)
		}

		if event.Test == "" {
			if event.Action == "pass" || event.Action == "fail" {
				rep.PackageStatus = event.Action
			}
			continue
		}

		if event.Action == "pass" || event.Action == "fail" || event.Action == "skip" {
			rep.Tests[event.Test] = testResult{Status: event.Action, Elapsed: event.Elapsed}
			top := topLevelTest(event.Test)
			if top == event.Test {
				rep.TopLevel[top] = testResult{Status: event.Action, Elapsed: event.Elapsed}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return report{}, nil, fmt.Errorf("read go test output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return rep, raw.Bytes(), fmt.Errorf("go test failed: %w", err)
	}

	repoRoot := findRepoRoot()
	if repoRoot != "" {
		pythonSchemas, appriseRoot, diffErr := loadPythonSchemas(repoRoot)
		rep.AppriseRoot = appriseRoot
		if diffErr != nil {
			rep.SchemaDiffErr = diffErr.Error()
		} else {
			rep.PythonSchemas = pythonSchemas
			rep.GoSchemas = notify.SupportedSchemas()
			rep.MissingSchemas, rep.ExtraSchemas = diffSchemas(rep.PythonSchemas, rep.GoSchemas)
		}
	}

	rep.NonHTTPSchemas = loadNonHTTPSchemas()
	rep.HTTPSchemas = loadHTTPSchemas(rep.NonHTTPSchemas)
	rep.ProviderCount = countProviders()
	rep.SchemaCoverage = testStatus(rep.TopLevel, "TestSchemaCoverage")
	rep.GoldenCheck = testStatus(rep.TopLevel, "TestGoldenUpdateCheck")
	rep.ProviderParity = testStatus(rep.TopLevel, "TestProviderRequestParity")
	rep.GoldenParity = testStatus(rep.TopLevel, "TestProviderGoldenRequests")

	for _, result := range rep.TopLevel {
		switch result.Status {
		case "pass":
			rep.PassedCount++
		case "fail":
			rep.FailedCount++
		case "skip":
			rep.SkippedCount++
		}
	}

	return rep, raw.Bytes(), nil
}

func topLevelTest(name string) string {
	if idx := strings.Index(name, "/"); idx != -1 {
		return name[:idx]
	}
	return name
}

func testStatus(results map[string]testResult, name string) string {
	if result, ok := results[name]; ok {
		return strings.ToUpper(result.Status)
	}
	return "UNKNOWN"
}

func schemaDiffError(rep report) error {
	if rep.SchemaDiffErr != "" {
		return fmt.Errorf("schema diff failed: %s", rep.SchemaDiffErr)
	}
	if len(rep.MissingSchemas) == 0 && len(rep.ExtraSchemas) == 0 {
		return nil
	}
	return fmt.Errorf(
		"schema diff failed: missing in go=%d extra in go=%d (see parity report for details)",
		len(rep.MissingSchemas),
		len(rep.ExtraSchemas),
	)
}

func findRepoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	current := wd
	for i := 0; i < 10; i++ {
		if fileExists(filepath.Join(current, "go.mod")) &&
			fileExists(filepath.Join(current, "internal", "parity")) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func appriseSourceRoot(repoRoot string) string {
	if envRoot := strings.TrimSpace(os.Getenv("APPRISE_SOURCE_ROOT")); envRoot != "" {
		if fileExists(filepath.Join(envRoot, "apprise")) {
			return envRoot
		}
	}
	candidate := filepath.Clean(filepath.Join(repoRoot, "..", "apprise"))
	if fileExists(filepath.Join(candidate, "apprise")) {
		return candidate
	}
	return ""
}

func pythonExecutable(repoRoot string) string {
	venvPython := filepath.Join(repoRoot, ".venv", "bin", "python")
	if fileExists(venvPython) {
		return venvPython
	}
	return "python"
}

func loadPythonSchemas(repoRoot string) ([]string, string, error) {
	appriseRoot := appriseSourceRoot(repoRoot)
	if appriseRoot == "" {
		return nil, "", fmt.Errorf("apprise source repo not found; set APPRISE_SOURCE_ROOT or place ../apprise")
	}

	script := filepath.Join(repoRoot, "internal", "testutil", "scripts", "list_schemas.py")
	python := pythonExecutable(repoRoot)
	cmd := exec.Command(python, script, appriseRoot)
	cmd.Env = os.Environ()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, appriseRoot, fmt.Errorf("list schemas failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	var schemas []string
	if err := json.Unmarshal(stdout.Bytes(), &schemas); err != nil {
		return nil, appriseRoot, fmt.Errorf("parse schemas: %w (output: %s)", err, strings.TrimSpace(stdout.String()))
	}
	return schemas, appriseRoot, nil
}

func diffSchemas(pythonSchemas, goSchemas []string) ([]string, []string) {
	pythonSet := map[string]struct{}{}
	for _, schema := range pythonSchemas {
		normalized := strings.ToLower(strings.TrimSpace(schema))
		if normalized == "" {
			continue
		}
		pythonSet[normalized] = struct{}{}
	}

	goSet := map[string]struct{}{}
	for _, schema := range goSchemas {
		normalized := strings.ToLower(strings.TrimSpace(schema))
		if normalized == "" {
			continue
		}
		goSet[normalized] = struct{}{}
	}

	missing := []string{}
	for schema := range pythonSet {
		if _, ok := goSet[schema]; !ok {
			missing = append(missing, schema)
		}
	}
	extra := []string{}
	for schema := range goSet {
		if _, ok := pythonSet[schema]; !ok {
			extra = append(extra, schema)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}

func countProviders() int {
	entries, err := os.ReadDir(filepath.Join("internal", "parity", "providers"))
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			count++
		}
	}
	return count
}

func loadNonHTTPSchemas() []string {
	path := filepath.Join("internal", "parity", "non_http_schemas.go")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	re := regexp.MustCompile(`"([a-z0-9]+)"`)
	matches := re.FindAllStringSubmatch(string(data), -1)
	seen := map[string]struct{}{}
	out := []string{}
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		value := match[1]
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

type manifest struct {
	Schemas []string `json:"schemas"`
}

func loadHTTPSchemas(nonHTTPSchemas []string) []string {
	entries, err := os.ReadDir(filepath.Join("internal", "parity", "providers"))
	if err != nil {
		return nil
	}

	nonHTTPSet := map[string]struct{}{}
	for _, schema := range nonHTTPSchemas {
		nonHTTPSet[schema] = struct{}{}
	}

	seen := map[string]struct{}{}
	out := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join("internal", "parity", "providers", entry.Name(), "manifest.json")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var parsed manifest
		if err := json.Unmarshal(data, &parsed); err != nil {
			continue
		}
		for _, schema := range parsed.Schemas {
			normalized := strings.ToLower(strings.TrimSpace(schema))
			if normalized == "" {
				continue
			}
			if _, ok := nonHTTPSet[normalized]; ok {
				continue
			}
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			out = append(out, normalized)
		}
	}
	sort.Strings(out)
	return out
}

func renderMarkdown(rep report, jsonPath string) string {
	var b strings.Builder
	b.WriteString("# Parity Report\n\n")
	b.WriteString(fmt.Sprintf("- Generated: %s\n", rep.GeneratedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Package: `%s`\n", rep.Package))
	if jsonPath != "" {
		b.WriteString(fmt.Sprintf("- Raw test log: `%s`\n", jsonPath))
	}
	b.WriteString(fmt.Sprintf("- Providers: %d\n\n", rep.ProviderCount))

	b.WriteString("## Summary\n\n")
	b.WriteString(fmt.Sprintf("- Package status: %s\n", strings.ToUpper(rep.PackageStatus)))
	b.WriteString(fmt.Sprintf("- Tests passed: %d\n", rep.PassedCount))
	b.WriteString(fmt.Sprintf("- Tests failed: %d\n", rep.FailedCount))
	b.WriteString(fmt.Sprintf("- Tests skipped: %d\n", rep.SkippedCount))
	b.WriteString(fmt.Sprintf("- Schema coverage: %s\n", rep.SchemaCoverage))
	b.WriteString(fmt.Sprintf("- Golden update check: %s\n", rep.GoldenCheck))
	b.WriteString(fmt.Sprintf("- Provider parity: %s\n", rep.ProviderParity))
	b.WriteString(fmt.Sprintf("- Golden parity: %s\n\n", rep.GoldenParity))

	b.WriteString("## Schema Coverage Details\n\n")
	if rep.SchemaDiffErr != "" {
		b.WriteString(fmt.Sprintf("- Apprise schemas: ERROR (%s)\n\n", rep.SchemaDiffErr))
	} else {
		if rep.AppriseRoot != "" {
			b.WriteString(fmt.Sprintf("- Apprise source root: `%s`\n", rep.AppriseRoot))
		}
		b.WriteString(fmt.Sprintf("- Python schemas: %d\n", len(rep.PythonSchemas)))
		b.WriteString(fmt.Sprintf("- Go schemas: %d\n", len(rep.GoSchemas)))
		b.WriteString(fmt.Sprintf("- Missing in Go: %d\n", len(rep.MissingSchemas)))
		b.WriteString(fmt.Sprintf("- Extra in Go: %d\n\n", len(rep.ExtraSchemas)))
		b.WriteString("| Type | Schemas |\n")
		b.WriteString("| --- | --- |\n")
		b.WriteString(fmt.Sprintf("| Missing in Go | %s |\n", strings.Join(rep.MissingSchemas, ", ")))
		b.WriteString(fmt.Sprintf("| Extra in Go | %s |\n\n", strings.Join(rep.ExtraSchemas, ", ")))
	}

	b.WriteString("## HTTP Coverage\n\n")
	b.WriteString("| Schema | Parity Tests | Status |\n")
	b.WriteString("| --- | --- | --- |\n")
	httpTests := []string{
		"TestProviderRequestParity",
		"TestProviderGoldenRequests",
		"TestProviderManifestsCoverSupportedSchemas",
		"TestProviderBuildersMatchManifests",
	}
	for _, schema := range rep.HTTPSchemas {
		status := coverageStatus(rep.TopLevel, httpTests)
		b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", schema, strings.Join(httpTests, ", "), status))
	}

	b.WriteString("## Non-HTTP Coverage\n\n")
	b.WriteString("| Schema | Parity Tests | Status |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, schema := range rep.NonHTTPSchemas {
		tests := coverageTests(schema)
		status := coverageStatus(rep.TopLevel, tests)
		b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", schema, strings.Join(tests, ", "), status))
	}

	b.WriteString("\n## Parity Tests\n\n")
	b.WriteString("| Test | Status | Elapsed (s) |\n")
	b.WriteString("| --- | --- | --- |\n")
	tests := make([]string, 0, len(rep.TopLevel))
	for name := range rep.TopLevel {
		tests = append(tests, name)
	}
	sort.Strings(tests)
	for _, name := range tests {
		result := rep.TopLevel[name]
		b.WriteString(fmt.Sprintf("| %s | %s | %.3f |\n", name, strings.ToUpper(result.Status), result.Elapsed))
	}

	return b.String()
}

func coverageTests(schema string) []string {
	switch schema {
	case "aprs":
		return []string{"TestAprsParity"}
	case "growl":
		return []string{"TestGrowlParity"}
	case "mqtt", "mqtts":
		return []string{"TestMQTTParity"}
	case "mailto":
		return []string{"TestMailtoSMTPParity"}
	case "mailtos":
		return []string{"TestMailtoStartTLSParity", "TestMailtoSSLParity"}
	case "smpp", "smpps":
		return []string{"TestSMPPParity"}
	case "syslog":
		return []string{"TestSyslogParity"}
	case "rsyslog":
		return []string{"TestRSyslogParity"}
	case "dbus", "glib", "gio", "gnome", "kde", "qt", "macosx", "windows":
		return []string{"TestLocalNotifyParity"}
	default:
		return []string{}
	}
}

func coverageStatus(results map[string]testResult, tests []string) string {
	if len(tests) == 0 {
		return "UNKNOWN"
	}
	for _, test := range tests {
		if result, ok := results[test]; !ok || result.Status != "pass" {
			return "FAIL"
		}
	}
	return "PASS"
}
