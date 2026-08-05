package notify

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// choiceArgsJSON lists the choice-valued arguments upstream actually rejects
// bad values for, together with what it accepts as a good one.
//
// Choice arguments cannot be enforced by a single rule. Upstream declares
// {"type": "choice:string", "values": [...]} on 491 arguments, but only the 93
// recorded here reject a value outside the set -- the rest silently fall back
// to a default or ignore the list entirely. Enforcing all of them would break
// the other 398; enforcing none lets a typo like ?mode=webook select the
// default mode and send anyway, which is how a notification quietly goes out
// the wrong way.
//
// internal/testutil/scripts/choice_probe.py regenerates this from upstream, and
// TestChoiceArgTableCurrent fails when the two disagree.
//
//go:embed data/choice_args.json
var choiceArgsJSON []byte

type choiceArgRule struct {
	// Values are the choices upstream declares for the argument.
	Values []string `json:"values"`
	// Aliases are short forms upstream keeps in a separate map and matches
	// with input.startswith(alias) -- octopush turns "p", "sms_p", "smsp" and
	// "+" into "sms_premium", which is why ?type=premium works even though
	// "premium" is not a declared value.
	Aliases []string `json:"aliases"`
}

var (
	choiceArgsOnce  sync.Once
	choiceArgsTable map[string]map[string]choiceArgRule
)

func choiceArgs() map[string]map[string]choiceArgRule {
	choiceArgsOnce.Do(func() {
		if err := json.Unmarshal(choiceArgsJSON, &choiceArgsTable); err != nil {
			panic(fmt.Sprintf("decode choice arg table: %v", err))
		}
	})
	return choiceArgsTable
}

// ChoiceArgRules exposes the table for parity tests.
func ChoiceArgRules() map[string]map[string]choiceArgRule {
	return choiceArgs()
}

// resolveChoiceValue maps a supplied value onto one of the declared choices.
//
// Upstream matches three different ways and this reproduces all of them, in the
// order upstream applies them:
//
//   - exact match against a declared value;
//   - the input is a prefix of a declared value, upstream's
//     next(v for v in VALUES if v.startswith(value)) idiom, so ?visibility=pub
//     selects "public";
//   - the input starts with an alias key, upstream's
//     next(value for key, value in MAP.items() if input.startswith(key))
//     idiom, so octopush's ?type=premium selects "sms_premium" via the "p" key.
//
// Returns the canonical declared value and whether anything matched.
func resolveChoiceValue(value string, rule choiceArgRule) (string, bool) {
	if value == "" {
		return "", false
	}
	for _, v := range rule.Values {
		if strings.EqualFold(v, value) {
			return v, true
		}
	}
	lowered := strings.ToLower(value)
	for _, v := range rule.Values {
		if strings.HasPrefix(strings.ToLower(v), lowered) {
			return v, true
		}
	}
	for _, alias := range rule.Aliases {
		if alias != "" && strings.HasPrefix(lowered, strings.ToLower(alias)) {
			return value, true
		}
	}
	return "", false
}

// ChoiceArgError reports a choice argument whose value upstream would reject.
type ChoiceArgError struct {
	Schema string
	Arg    string
	Value  string
	Values []string
}

func (e *ChoiceArgError) Error() string {
	return fmt.Sprintf("invalid %s value %q for %s:// (expected one of %s)",
		e.Arg, e.Value, e.Schema, strings.Join(e.Values, ", "))
}

// applyChoiceArgs validates the choice arguments present on the URL and
// canonicalizes any that were supplied as a prefix, so the provider code sees
// the full declared value rather than the abbreviation the user typed.
func applyChoiceArgs(parsed *ParsedURL) error {
	if parsed == nil {
		return nil
	}
	rules, ok := choiceArgs()[strings.ToLower(parsed.Scheme)]
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
			// An empty value is upstream's "not supplied" -- it falls through
			// to the default rather than failing the choice check.
			continue
		}

		resolved, matched := resolveChoiceValue(value, rule)
		if !matched {
			return &ChoiceArgError{
				Schema: parsed.Scheme,
				Arg:    arg,
				Value:  value,
				Values: rule.Values,
			}
		}
		if resolved != raw {
			parsed.Query[arg] = resolved
		}
	}
	return nil
}
