# Parity Reports

Run the parity report generator to execute all parity tests and emit a
Markdown + JSON report for auditing:

```sh
GOCACHE=$PWD/.gocache go run ./internal/tools/parity_report \
  -out reports/parity_report.md \
  -json reports/parity_report.json
```

- `reports/parity_report.md` summarizes pass/fail status, non-HTTP coverage, and
  top-level parity tests.
- `reports/parity_report.json` captures the raw `go test -json` output for
  provenance.

These files are generated artifacts; check them in only when you want a
snapshot tied to a specific commit.
