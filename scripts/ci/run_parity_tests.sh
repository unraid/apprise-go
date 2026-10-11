#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../.."

GOCACHE_DIR="${GOCACHE:-}"
if [[ -z "${GOCACHE_DIR}" ]]; then
  GOCACHE_DIR="$(mktemp -d)"
  trap 'rm -rf "${GOCACHE_DIR}"' EXIT
  export GOCACHE="${GOCACHE_DIR}"
fi

go run ./internal/tools/parity_report \
  -out reports/parity_report.md \
  -json reports/parity_report.json

# Compare runtime-dependent schema capabilities as well as request payloads;
# generated metadata can otherwise advertise macOS notification support on Linux.
parity_asset_root="$(.venv/bin/python -c 'from pathlib import Path; import apprise; print(Path(apprise.__file__).parent / "assets")')"
APPRISE_ASSET_ROOT="${APPRISE_ASSET_ROOT:-${parity_asset_root}}" \
  go test ./internal/notify ./internal/cli -run '^(TestSchemaMatchesPython|TestSchemaDetailsParity)$' -count=1

go test ./internal/notify -run TestAttachmentRequestParity -count=1 -v

# Exercise every HTTP schema option as well as the curated provider cases.
go test ./internal/parity -tags=e2e -run '^TestE2E' -count=1 \
  -timeout="${APPRISE_PARITY_TIMEOUT:-30m}"
