# Changelog for apprise-go

This project maintains its own release version. Compatibility with upstream
`caronc/apprise` releases is tracked separately via
`internal/version.UpstreamVersion`.
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
