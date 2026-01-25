# AGENTS.md

This repo is the Go port of Apprise. Use this file as a quick runbook when resuming work.

Project layout
- Python reference: `../apprise`
- Go port: this directory (`apprise-go`)

Environment
- Python virtualenv: `apprise-go/.venv` with `apprise` + `cryptography` installed (recreate if repo path changes)
- Go module: `github.com/unraid/apprise-go`
- CLI binary name: `apprise` (drop-in goal)

Workflow
- Tests should compare Go behavior to the installed Python apprise using a local capture server.
- Request-spec parity is driven by provider folders in `internal/parity/providers/<provider>` with `manifest.json` and `cases.json`.
- Keep Go version strings aligned with upstream apprise (see `internal/version/version.go`).
- Request-spec parity uses `internal/testutil/scripts/capture_request.py` (keeps User-Agent only when Python explicitly sets it).
- Request-spec parity now compares full request sequences from Python and Go `Send` calls.
- Golden request fixtures live at `internal/parity/providers/<provider>/golden.json`. Regenerate with `.venv/bin/python internal/testutil/scripts/update_golden.py`.
- CI parity setup scripts: `scripts/ci/setup_parity_env.sh` and `scripts/ci/run_parity_tests.sh`.

Process log
- See `PROCESS.md` for current status, setup, and next steps.
