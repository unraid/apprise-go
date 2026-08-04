package parity

import (
	"sort"
	"strings"
	"testing"

	"github.com/unraid/apprise-go/internal/notify"
)

// TestAttachmentSupportIsImplemented checks that a provider advertising
// attachment support can actually transmit one.
//
// The attachment_support flag mirrors what upstream declares about the
// service. For most of this port's life nothing could send a file at all:
// Sender had no parameter for one, and the CLI parsed --attach and discarded
// it, so a notification reported success having delivered nothing. This test
// was written failing, naming every provider still in that state, and stayed
// failing until the list emptied.
//
// It is now a guard against the gap reopening. Adding a provider that
// advertises attachment support without implementing AttachmentSender fails
// here, which is better than the alternative: silently accepting a file and
// dropping it. Note it only proves a provider can be handed an attachment —
// that what it sends matches upstream is the golden fixtures' job, and that
// the file's bytes really travel is TestAttachmentGoldensCarryTheirFiles'.
func TestAttachmentSupportIsImplemented(t *testing.T) {
	defs := loadProviderDefinitions(t)

	advertising := map[string]bool{}
	for _, entry := range notify.SchemaEntries() {
		support, _ := entry["attachment_support"].(bool)
		for _, key := range []string{"protocols", "secure_protocols"} {
			values, ok := entry[key].([]string)
			if !ok {
				continue
			}
			for _, schema := range values {
				advertising[strings.ToLower(schema)] = support
			}
		}
	}

	checked, missing := 0, []string{}
	for name := range defs {
		def := defs[name]

		advertised := false
		for _, schema := range def.Schemas {
			if advertising[strings.ToLower(schema)] {
				advertised = true
				break
			}
		}
		if !advertised || len(def.Cases) == 0 {
			continue
		}

		builder, ok := providerBuilders[name]
		if !ok {
			continue
		}
		parsed, err := notify.ParseURL(def.Cases[0].URL)
		if err != nil {
			continue
		}
		target, err := builder(parsed)
		if err != nil {
			continue
		}

		checked++
		if _, sends := target.(notify.AttachmentSender); !sends {
			missing = append(missing, name)
		}
	}

	if checked == 0 {
		t.Fatal("no advertising provider could be built; the check is not running")
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("%d of %d providers advertise attachment support but cannot send one; "+
			"they accept --attach and deliver nothing:\n  %s",
			len(missing), checked, strings.Join(missing, ", "))
	}
}
