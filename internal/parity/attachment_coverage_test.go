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

	// A provider outside the HTTP parity set still advertises the flag, and
	// mailto sat in exactly that blind spot: it declared attachment support,
	// sent nothing, and never appeared in the count because the walk below
	// only reaches providers with request fixtures. Anything here is covered
	// by a test of its own instead.
	elsewhere := map[string]string{
		"mailto":  "mailto_attachment_parity_test.go",
		"mailtos": "mailto_attachment_parity_test.go",
	}

	for schema, where := range elsewhere {
		if !advertising[schema] {
			t.Errorf("%s no longer advertises attachment support; "+
				"drop it from this list and from %s", schema, where)
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

	// Anything advertising the flag is either walked above or named in the
	// list of providers covered elsewhere. A new one in neither place is the
	// blind spot mailto used to sit in.
	walked := map[string]struct{}{}
	for name := range defs {
		for _, schema := range defs[name].Schemas {
			walked[strings.ToLower(schema)] = struct{}{}
		}
	}

	unwatched := []string{}
	for schema, supported := range advertising {
		if !supported {
			continue
		}
		if _, ok := walked[schema]; ok {
			continue
		}
		if _, ok := elsewhere[schema]; ok {
			continue
		}
		if notify.IsKnownGapSchema(schema) {
			continue
		}
		unwatched = append(unwatched, schema)
	}

	if len(unwatched) > 0 {
		sort.Strings(unwatched)
		t.Errorf("%d schemas advertise attachment support but no test checks "+
			"they can send one; give them a request fixture or a test of "+
			"their own and name them above:\n  %s",
			len(unwatched), strings.Join(unwatched, ", "))
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("%d of %d providers advertise attachment support but cannot send one; "+
			"they accept --attach and deliver nothing:\n  %s",
			len(missing), checked, strings.Join(missing, ", "))
	}
}
