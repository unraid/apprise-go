# apprise-go

<div align="center">

**A Go port of Apprise - Push Notifications That Work with Everything**

[![GitHub Release](https://img.shields.io/github/v/release/unraid/apprise-go)](https://github.com/unraid/apprise-go/releases)
[![License](https://img.shields.io/github/license/unraid/apprise-go)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/unraid/apprise-go)](go.mod)
[![Parity](https://github.com/unraid/apprise-go/actions/workflows/parity.yml/badge.svg)](https://github.com/unraid/apprise-go/actions/workflows/parity.yml)

[Features](#features) • [Installation](#installation) • [Usage](#usage) • [Releases](#releases) • [Contributing](#contributing)

Latest parity report: [GitHub Actions parity workflow](https://github.com/unraid/apprise-go/actions/workflows/parity.yml)

</div>

---

## About

apprise-go is a Go port of [Apprise](https://github.com/caronc/apprise) by Chris Caron. It aims to provide the same powerful notification capabilities in a standalone, compiled binary with no runtime dependencies.

> **Note:** This project is not affiliated with or endorsed by the upstream Apprise project.

## Features

- **Multi-Platform Support**: Send notifications to 100+ services
- **Standalone Binary**: No Python runtime required
- **Cross-Platform**: Linux, macOS, and Windows builds
- **CLI Interface**: Easy command-line usage
- **Storage Support**: Persistent notification configurations
- **Schema Details**: Full service configuration information

## Installation

### Pre-built Binaries

Download the latest release for your platform from the [Releases page](https://github.com/unraid/apprise-go/releases).

#### macOS (ARM64)
```bash
# Download and extract (replace VERSION with the latest version)
curl -LO https://github.com/unraid/apprise-go/releases/download/vVERSION/apprise-go-darwin-arm64.zip
unzip apprise-go-darwin-arm64.zip
chmod +x apprise-go
sudo mv apprise-go /usr/local/bin/
```

#### Linux (AMD64)
```bash
# Download and extract (replace VERSION with the latest version)
curl -LO https://github.com/unraid/apprise-go/releases/download/vVERSION/apprise-go-linux-amd64
chmod +x apprise-go-linux-amd64
sudo mv apprise-go-linux-amd64 /usr/local/bin/apprise-go
```

#### Windows
Download the `.exe` file from the [Releases page](https://github.com/unraid/apprise-go/releases) and add it to your PATH.

### Build from Source

```bash
git clone https://github.com/unraid/apprise-go.git
cd apprise-go
go build -o apprise-go ./cmd/apprise
```

## Usage

### Library Usage

```go
package main

import (
	"log"

	apprise "github.com/unraid/apprise-go"
)

func main() {
	client := apprise.New()
	if err := client.Add("discord://webhook_id/webhook_token"); err != nil {
		log.Fatal(err)
	}

	if err := client.Send(
		"Something happened!",
		apprise.WithTitle("Alert"),
		apprise.WithNotifyType(apprise.NotifyWarning),
	); err != nil {
		log.Fatal(err)
	}
}
```

For one-off sends, use `apprise.Send(urls, body, options...)`.

### Basic Notification

```bash
# Send a simple notification
apprise -b "Hello from apprise!" "discord://webhook_id/webhook_token"
```

### Multiple Services

```bash
# Send to multiple services at once
apprise -b "Multi-platform notification" "discord://..." "slack://..."
```

### With Title

```bash
# Add a title to your notification
apprise -t "Alert" -b "Something happened!" "discord://..."
```

### Schema and Service Information

```bash
# Get full Apprise schema as JSON
apprise --schema

# Print details about all supported services
apprise --details
```

### Storage Support

```bash
# Store and reuse configurations
apprise --storage-path /path/to/config -b "Using saved config"
```

### Configuration Files

apprise-go supports Apprise-style TEXT and YAML configuration files. YAML tagged
URLs should use the upstream mapping form where the URL has a trailing colon:

```yaml
version: 1
groups:
  now: my_now_pers
urls:
  - tgram://token_id/chat_id/?format=markdown&mdv=v1:
      tag: my_now_pers
```

Do not put `tag` under a bare scalar URL; that is not valid YAML and will not be
loaded as a tagged Apprise URL.

## Releases

### Latest Release

The latest stable release can always be found on the [GitHub Releases page](https://github.com/unraid/apprise-go/releases/latest).

### Release Assets

Each release includes:
- **Pre-built binaries** for multiple platforms:
  - `apprise-go-darwin-amd64.zip` - macOS Intel (signed & notarized)
  - `apprise-go-darwin-arm64.zip` - macOS Apple Silicon (signed & notarized)
  - `apprise-go-linux-amd64` - Linux AMD64
  - `apprise-go-linux-arm64` - Linux ARM64
  - `apprise-go-linux-386` - Linux 32-bit
  - `apprise-go-linux-armv7` - Linux ARMv7 (Raspberry Pi, etc.)
  - `apprise-go-windows-amd64.exe` - Windows AMD64
  - `apprise-go-windows-arm64.exe` - Windows ARM64
  - `apprise-go-windows-386.exe` - Windows 32-bit
  - `apprise-go-freebsd-amd64` - FreeBSD AMD64
  - `apprise-go-freebsd-arm64` - FreeBSD ARM64
- **Source code** (zip and tar.gz)
- **SHA256 checksums** for verification

### Versioning

This project maintains its own semantic versioning independent of upstream Apprise. Compatibility with the upstream `caronc/apprise` project is tracked via `internal/version.UpstreamVersion`.

### Changelog

All notable changes are documented in [CHANGELOG.md](CHANGELOG.md). The changelog follows the [Keep a Changelog](https://keepachangelog.com/) format.

### Release Process

Releases are automated through GitHub Actions:
1. Version bumps and changelog updates use [Knope](https://knope.tech/)
2. The release workflow builds, signs (macOS), and notarizes (macOS) binaries
3. Assets are automatically uploaded to GitHub Releases
4. Upstream version parity is tracked via automated checks

See [PROCESS.md](PROCESS.md) for detailed release procedures.

## Project Structure

```
apprise-go/
├── cmd/           # Command-line application
├── internal/      # Internal Go packages
├── assets/        # Default notification icons and themes
├── dist/          # Build output directory
├── scripts/       # Build and release scripts
├── .github/       # GitHub Actions workflows
├── CHANGELOG.md   # Version history and changes
├── PROCESS.md     # Development and release processes
├── AGENTS.md      # Agent configuration documentation
└── README.md      # This file
```

## Attribution and License

- **Upstream Apprise** is created by Chris Caron and licensed under the BSD 2-Clause License
- **This project** reimplements Apprise behavior in Go and includes modifications
- See [LICENSE](LICENSE) for the full license text
- See [NOTICE.md](NOTICE.md) for additional attribution information

## Contributing

Contributions are welcome! Please see [PROCESS.md](PROCESS.md) for development guidelines and the release process.

### Quick Start for Contributors

```bash
# Clone the repository
git clone https://github.com/unraid/apprise-go.git
cd apprise-go

# Build the project
go build -o apprise-go ./cmd/apprise

# Run tests
go test ./...
```

## Support

- **Issues**: Report bugs or request features via [GitHub Issues](https://github.com/unraid/apprise-go/issues)
- **Upstream Apprise**: For questions about the original Python project, see [caronc/apprise](https://github.com/caronc/apprise)

## Related Projects

- [caronc/apprise](https://github.com/caronc/apprise) - The original Python implementation
- [caronc/apprise-api](https://github.com/caronc/apprise-api) - RESTful API for Apprise

---

<div align="center">

**Made with ❤️ by the developers of [Unraid](https://unraid.net)**

[Learn more about Unraid OS](https://unraid.net)

</div>
