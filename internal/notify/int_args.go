package notify

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// intArgsJSON lists the integer arguments upstream actually rejects bad values
// for, together with the bounds it enforces.
//
// Same shape as the choice-argument table, and the same reason for existing.
// Upstream declares 213 arguments as {"type": "int", "min": N, "max": M}, but
// only the 9 recorded here reject a value outside the declared range -- the
// rest clamp to the bound or ignore the range entirely. Enforcing all of them
// would reject URLs upstream accepts; enforcing none lets ?version=0 through to
// a request that cannot work.
//
// Note that the three flavors differ per argument: some reject a non-numeric
// value but tolerate an out-of-range one, and ttl is the reverse. Each is
// recorded separately rather than assumed to move together.
//
// internal/testutil/scripts/int_probe.py regenerates this from upstream, and
// TestIntArgTableCurrent fails when the two disagree.
//
//go:embed data/int_args.json
var intArgsJSON []byte

type intArgRule struct {
	// RejectsNonNumeric reports whether a value that is not an integer at all
	// is an error rather than a fallback to the default.
	RejectsNonNumeric bool `json:"rejects_nonnumeric"`
	// Min and Max are the bounds upstream enforces. Absent means upstream
	// accepts values past that end even though the schema declares a bound.
	Min *int `json:"min,omitempty"`
	Max *int `json:"max,omitempty"`
}

var (
	intArgsOnce  sync.Once
	intArgsTable map[string]map[string]intArgRule
)

func intArgs() map[string]map[string]intArgRule {
	intArgsOnce.Do(func() {
		if err := json.Unmarshal(intArgsJSON, &intArgsTable); err != nil {
			panic(fmt.Sprintf("decode int arg table: %v", err))
		}
	})
	return intArgsTable
}

// IntArgRules exposes the table for parity tests.
func IntArgRules() map[string]map[string]intArgRule {
	return intArgs()
}

// IntArgError reports an integer argument whose value upstream would reject.
type IntArgError struct {
	Schema string
	Arg    string
	Value  string
	Reason string
}

func (e *IntArgError) Error() string {
	return fmt.Sprintf("invalid %s value %q for %s:// (%s)",
		e.Arg, e.Value, e.Schema, e.Reason)
}

// applyIntArgs validates the bounded integer arguments present on the URL.
func applyIntArgs(parsed *ParsedURL) error {
	if parsed == nil {
		return nil
	}
	rules, ok := intArgs()[strings.ToLower(parsed.Scheme)]
	if !ok {
		return nil
	}

	for arg, rule := range rules {
		raw, present := parsed.Query[arg]
		if !present {
			continue
		}
		value := strings.TrimSpace(raw)
		if value == "" {
			// Upstream reads an empty value as "not supplied".
			continue
		}

		parsedValue, err := strconv.Atoi(value)
		if err != nil {
			if rule.RejectsNonNumeric {
				return &IntArgError{
					Schema: parsed.Scheme, Arg: arg, Value: value,
					Reason: "expected an integer",
				}
			}
			continue
		}
		if rule.Min != nil && parsedValue < *rule.Min {
			return &IntArgError{
				Schema: parsed.Scheme, Arg: arg, Value: value,
				Reason: fmt.Sprintf("minimum is %d", *rule.Min),
			}
		}
		if rule.Max != nil && parsedValue > *rule.Max {
			return &IntArgError{
				Schema: parsed.Scheme, Arg: arg, Value: value,
				Reason: fmt.Sprintf("maximum is %d", *rule.Max),
			}
		}
	}
	return nil
}
