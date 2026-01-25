package version

import "fmt"

const (
	Title     = "Apprise"
	License   = "BSD 2-Clause"
	Copyright = "Copyright (C) 2025 Chris Caron <lead2gold@gmail.com>"
)

// Override at build time with: -ldflags "-X github.com/unraid/apprise-go/internal/version.Version=1.2.3"
var Version = "0.1.4"

// UpstreamVersion tracks the caronc/apprise release version we are compatible with.
// Override at build time with: -ldflags "-X github.com/unraid/apprise-go/internal/version.UpstreamVersion=1.2.3"
var UpstreamVersion = "1.9.7"

func Message() string {
	return fmt.Sprintf("%s v%s\n%s\nThis code is licensed under the %s License.", Title, Version, Copyright, License)
}
