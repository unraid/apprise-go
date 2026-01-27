package parity

import (
	"os"
	"testing"
)

func maybeParallel(t *testing.T) {
	if os.Getenv("APPRISE_PARITY_SERIAL") != "" {
		return
	}
	t.Parallel()
}
