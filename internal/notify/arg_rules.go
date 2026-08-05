package notify

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// argRulesJSON covers the two kinds of argument validation the choice and
// integer tables do not: bounded floats, and arguments carrying their own
// regex.
//
// pushward's ?volume= must land in 0.0 to 1.0 and raises outside it rather than
// clamping; strmlabs validates ?currency= and hassio validates ?nid=. As
// everywhere else here, a declaration is not a rule -- upstream declares 152
// bounded floats and 11 regex arguments, and only the handful recorded here are
// actually enforced.
//
// internal/testutil/scripts/arg_probe.py regenerates this, and
// TestArgRuleTableCurrent fails when the two disagree.
//
//go:embed data/arg_rules.json
var argRulesJSON []byte

type argRule struct {
	Kind       string   `json:"kind"`
	Min        *float64 `json:"min,omitempty"`
	Max        *float64 `json:"max,omitempty"`
	Pattern    string   `json:"pattern,omitempty"`
	IgnoreCase bool     `json:"ignorecase,omitempty"`
}

var (
	argRulesOnce  sync.Once
	argRulesTable map[string]map[string]argRule
	argRulesRe    map[string]map[string]*regexp.Regexp
)

func argRules() (map[string]map[string]argRule, map[string]map[string]*regexp.Regexp) {
	argRulesOnce.Do(func() {
		if err := json.Unmarshal(argRulesJSON, &argRulesTable); err != nil {
			panic(fmt.Sprintf("decode arg rule table: %v", err))
		}
		argRulesRe = map[string]map[string]*regexp.Regexp{}
		for schema, rules := range argRulesTable {
			for arg, rule := range rules {
				if rule.Kind != "regex" || rule.Pattern == "" {
					continue
				}
				pattern := rule.Pattern
				if rule.IgnoreCase {
					pattern = "(?i)" + pattern
				}
				compiled, err := regexp.Compile(pattern)
				if err != nil {
					continue
				}
				if argRulesRe[schema] == nil {
					argRulesRe[schema] = map[string]*regexp.Regexp{}
				}
				argRulesRe[schema][arg] = compiled
			}
		}
	})
	return argRulesTable, argRulesRe
}

// ArgRules exposes the table for parity tests.
func ArgRules() map[string]map[string]argRule {
	table, _ := argRules()
	return table
}

// ArgRuleError reports an argument whose value upstream would reject.
type ArgRuleError struct {
	Schema string
	Arg    string
	Value  string
	Reason string
}

func (e *ArgRuleError) Error() string {
	return fmt.Sprintf("invalid %s value %q for %s:// (%s)",
		e.Arg, e.Value, e.Schema, e.Reason)
}

// applyArgRules validates bounded float and regex-guarded arguments.
func applyArgRules(parsed *ParsedURL) error {
	if parsed == nil {
		return nil
	}
	schema := strings.ToLower(parsed.Scheme)
	table, compiled := argRules()
	rules, ok := table[schema]
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

		switch rule.Kind {
		case "float":
			number, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return &ArgRuleError{
					Schema: schema, Arg: arg, Value: value,
					Reason: "expected a number",
				}
			}
			if rule.Min != nil && number < *rule.Min {
				return &ArgRuleError{
					Schema: schema, Arg: arg, Value: value,
					Reason: fmt.Sprintf("minimum is %g", *rule.Min),
				}
			}
			if rule.Max != nil && number > *rule.Max {
				return &ArgRuleError{
					Schema: schema, Arg: arg, Value: value,
					Reason: fmt.Sprintf("maximum is %g", *rule.Max),
				}
			}

		case "regex":
			re := compiled[schema][arg]
			if re != nil && !re.MatchString(value) {
				return &ArgRuleError{
					Schema: schema, Arg: arg, Value: value,
					Reason: "does not match the expected format",
				}
			}
		}
	}
	return nil
}
