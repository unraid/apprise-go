package assets

import "embed"

// NotificationImages keeps the default provider icons in the standalone binary.
//
//go:embed themes/default/apprise-*-256x256.png
var NotificationImages embed.FS
