# Changelog for apprise-go

This project maintains its own release version. Compatibility with upstream
`caronc/apprise` releases is tracked separately via
`internal/version.UpstreamVersion`.
## 0.2.0 (2026-02-01)

### Features

- full parity with non-http endpoints (and parity reports) (#34)

### Fixes

- resolve codeql scanning warnings (#36)

## 0.1.10 (2026-01-31)

### Features

- add mailto SMTP support and parity coverage (#32)

## 0.1.9 (2026-01-28)

### Features

- add arm and 32 bit builds (#26)

## 0.1.8 (2026-01-28)

### Features

- cli add schema details and storage support (#24)

## 0.1.7 (2026-01-25)

### Features

- add windows builds

## 0.1.6 (2026-01-25)

### Fixes

- code signature flag single line input

## 0.1.5 (2026-01-25)

### Fixes

- hardened runtime flag

## 0.1.4 (2026-01-25)

### Fixes

- Build with -trimpath and add macOS notarize

## 0.1.3 (2026-01-25)

### Fixes

- swap all actions to version numbers as the shas must be complete

## 0.1.2 (2026-01-25)

### Fixes

- incorrect sha on build

## 0.1.1 (2026-01-25)

### Features

- Refactor image URL handling to raw.githubusercontent
- add macos builds
- build binary
- fully reproduce apprise in Go
- add form/xml notifiers and schema coverage
- add json notifier and cli runner
- initial commit

### Fixes

- update parity checks to ignore asset URL changes
- use upstream version instead of version
- update module golang.org/x/crypto to v0.47.0
