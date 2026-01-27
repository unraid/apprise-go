package parity

import (
	"os"
	"testing"
)

var defaultParityEnv = map[string]string{
	"APPRISE_FIXED_TIME":            "2024-01-01T00:00:00Z",
	"APPRISE_OAUTH_NONCE":           "parity-nonce",
	"APPRISE_OAUTH_TIMESTAMP":       "1704067200",
	"APPRISE_VAPID_TEST_JWT":        "parity.jwt.token",
	"APPRISE_VAPID_TEST_PUBLIC_KEY": "parity-public-key",
	"APPRISE_VAPID_TEST_ENCRYPTED":  "cGFyaXR5LXZhcGlk",
	"APPRISE_SIMPLEPUSH_TEST_IV":    "00112233445566778899AABBCCDDEEFF",
}

func TestMain(m *testing.M) {
	for key, value := range defaultParityEnv {
		if _, ok := os.LookupEnv(key); ok {
			continue
		}
		_ = os.Setenv(key, value)
	}

	os.Exit(m.Run())
}
