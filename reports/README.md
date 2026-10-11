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

The CI parity script compares schema metadata against the running platform,
including the macOS notifier's capability flag.

The CI parity script also runs the expanded schema exercise group:

```sh
go test ./internal/parity -tags=e2e -run '^TestE2E' -count=1 -timeout=30m
```

Each Python request capture has a 30-second deadline. Aggregate golden checks
use the enclosing test's deadline. Matrix encrypted requests are authenticated
and decrypted by an independent receiver before their recovered events are
compared. The receiver is checked against upstream cryptographic vectors and
rejects corrupted signatures, MACs, recipients, and missing shared room keys.
The disposable receiver's keys are fixed; sender keys and ciphertext remain
random. Request and golden assertions are preserved.

Go rejects a webhook template's present `embeds` field unless it is an array,
including when `content` is present. Python Apprise 1.13.1 still accepts those
malformed fields. Valid templates and templates rejected by both implementations
retain full request sequence comparisons.
