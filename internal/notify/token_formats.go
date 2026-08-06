package notify

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// tokenFormatsJSON is upstream's credential format checks, and which URL field
// each one guards.
//
// Upstream validates its tokens with validate_regex() and raises when one does
// not match. Without this the port accepts stackfield://not-a-valid-uuid and
// sendgrid://invalid-api-key+*-d:user@example.com, builds a target from them,
// and issues a request that cannot succeed -- reporting a network failure for
// what is really a typo in the configuration.
//
// Only the 71 checks upstream was observed to actually enforce are recorded. A
// declared regex is not proof of enforcement: 12 of the 83 that upstream
// declares are never applied, and enforcing those would reject URLs upstream
// accepts.
//
// internal/testutil/scripts/token_regex.py regenerates this, and
// TestTokenFormatTableCurrent fails when the two disagree.
//
//go:embed data/token_formats.json
var tokenFormatsJSON []byte

type tokenFormatRule struct {
	Pattern    string `json:"pattern"`
	IgnoreCase bool   `json:"ignorecase"`
	Token      string `json:"token"`
	// OverrideArgs are query arguments that supply this token directly. When
	// one is present the URL field holds something else and must not be
	// checked against the credential's format.
	OverrideArgs []string `json:"override_args,omitempty"`
}

var (
	tokenFormatsOnce  sync.Once
	tokenFormatsTable map[string]map[string]tokenFormatRule
	tokenFormatsRe    map[string]map[string]*regexp.Regexp
)

func tokenFormats() (map[string]map[string]tokenFormatRule, map[string]map[string]*regexp.Regexp) {
	tokenFormatsOnce.Do(func() {
		if err := json.Unmarshal(tokenFormatsJSON, &tokenFormatsTable); err != nil {
			panic(fmt.Sprintf("decode token format table: %v", err))
		}
		tokenFormatsRe = map[string]map[string]*regexp.Regexp{}
		for schema, fields := range tokenFormatsTable {
			for field, rule := range fields {
				pattern := rule.Pattern
				if rule.IgnoreCase {
					pattern = "(?i)" + pattern
				}
				// Python's named groups spell differently to Go's.
				pattern = strings.ReplaceAll(pattern, "(?P<", "(?P<")
				compiled, err := regexp.Compile(pattern)
				if err != nil {
					// Patterns Go cannot compile are filtered out when the
					// table is generated, so reaching here means the table was
					// hand-edited. Skipping silently would leave a rule that
					// enforces nothing; TestTokenFormatTableCurrent fails on
					// exactly this.
					continue
				}
				if tokenFormatsRe[schema] == nil {
					tokenFormatsRe[schema] = map[string]*regexp.Regexp{}
				}
				tokenFormatsRe[schema][field] = compiled
			}
		}
	})
	return tokenFormatsTable, tokenFormatsRe
}

// TokenFormatRules exposes the table for parity tests.
func TokenFormatRules() map[string]map[string]tokenFormatRule {
	table, _ := tokenFormats()
	return table
}

// TokenFormatError reports a credential whose shape upstream would reject.
type TokenFormatError struct {
	Schema string
	Field  string
	Token  string
}

func (e *TokenFormatError) Error() string {
	return fmt.Sprintf("invalid %s for %s:// (in the url's %s field)",
		e.Token, e.Schema, e.Field)
}

// applyTokenFormats checks the credentials carried in the URL authority.
func applyTokenFormats(parsed *ParsedURL) error {
	if parsed == nil {
		return nil
	}
	table, compiled := tokenFormats()
	rules, ok := table[strings.ToLower(parsed.Scheme)]
	if !ok {
		return nil
	}

	for field, rule := range rules {
		re := compiled[strings.ToLower(parsed.Scheme)][field]
		if re == nil {
			continue
		}

		var value string
		switch field {
		case "host":
			value = parsed.Host
		case "user":
			value = parsed.User
		case "password":
			value = parsed.Password
		default:
			continue
		}

		// An absent field is a different complaint, raised by the provider
		// with the context to say what was missing.
		if strings.TrimSpace(value) == "" {
			continue
		}

		// A query argument carrying the credential frees this field to mean
		// something else.
		overridden := false
		for _, arg := range rule.OverrideArgs {
			if strings.TrimSpace(parsed.Query[arg]) != "" {
				overridden = true
				break
			}
		}
		if overridden {
			continue
		}
		if !re.MatchString(value) {
			return &TokenFormatError{
				Schema: parsed.Scheme,
				Field:  field,
				Token:  rule.Token,
			}
		}
	}
	return nil
}
