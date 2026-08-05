package notify

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
)

// hostRequirementsJSON records, per schema, what upstream does with a URL that
// carries nothing and with one whose hostname is not a hostname.
//
// Upstream's NotifyBase.parse_url verifies the host by default and returns None
// when it is empty or malformed, which is why json:// and ntfys://user:web@-_/
// are rejected. Plugins whose authority is an api key or a phone number rather
// than a server opt out with verify_host=False, and that opt-out is not visible
// in the schema metadata -- so it is asked of upstream directly.
//
// internal/testutil/scripts/host_probe.py regenerates this, and
// TestHostRequirementTableCurrent fails when the two disagree.
//
//go:embed data/host_requirements.json
var hostRequirementsJSON []byte

type hostRequirement struct {
	// RejectsEmpty means upstream refuses a URL of the bare "schema://" form.
	// This is not the same as requiring a hostname: sns://?access=..&region=..
	// carries no host at all and is perfectly valid, so this only applies when
	// the URL carries nothing else either.
	RejectsEmpty bool `json:"rejects_empty"`
	// RejectsInvalid means a host that is present but is not a valid hostname
	// is an error rather than something to pass along to the request.
	RejectsInvalid bool `json:"rejects_invalid"`
}

var (
	hostRequirementsOnce  sync.Once
	hostRequirementsTable map[string]hostRequirement
)

func hostRequirements() map[string]hostRequirement {
	hostRequirementsOnce.Do(func() {
		if err := json.Unmarshal(hostRequirementsJSON, &hostRequirementsTable); err != nil {
			panic(fmt.Sprintf("decode host requirement table: %v", err))
		}
	})
	return hostRequirementsTable
}

// HostRequirements exposes the table for parity tests.
func HostRequirements() map[string]hostRequirement {
	return hostRequirements()
}

// hostLabel matches one label of a hostname, reproducing upstream's is_hostname.
//
// Hyphens and underscores are both allowed inside a label but not at either
// end -- is_hostname takes underscore=True by default, which is how a host of
// "client_id" is legal. Getting this wrong in the strict direction rejects
// working URLs, so the rule is copied rather than assumed.
var hostLabel = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9_-]{0,61}[A-Za-z0-9])?$`)

// isValidHostname reports whether the value would satisfy upstream's is_hostname.
func isValidHostname(host string) bool {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if host == "" || len(host) > 253 {
		return false
	}

	// An IP literal is a valid host. IPv6 arrives in brackets, and its colons
	// would otherwise fail every label check.
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		return net.ParseIP(host[1:len(host)-1]) != nil
	}
	if net.ParseIP(host) != nil {
		return true
	}
	for _, label := range strings.Split(host, ".") {
		if !hostLabel.MatchString(label) {
			return false
		}
	}
	return true
}

// EmptyURLError reports a URL that carries nothing at all.
type EmptyURLError struct{ Schema string }

func (e *EmptyURLError) Error() string {
	return fmt.Sprintf("%s:// carries no host, credentials or targets", e.Schema)
}

// InvalidHostError reports a host that is not a valid hostname.
type InvalidHostError struct {
	Schema string
	Host   string
}

func (e *InvalidHostError) Error() string {
	return fmt.Sprintf("invalid hostname %q for %s://", e.Host, e.Schema)
}

// applyHostRequirements checks the URL's authority against upstream's rules.
func applyHostRequirements(parsed *ParsedURL) error {
	if parsed == nil {
		return nil
	}
	rule, ok := hostRequirements()[strings.ToLower(parsed.Scheme)]
	if !ok {
		return nil
	}

	host := strings.TrimSpace(parsed.Host)

	// "Carries nothing" is deliberately strict: no host, no credentials, no
	// path and no query. A URL supplying everything through query arguments
	// has no host either and must not be caught here.
	if rule.RejectsEmpty && host == "" &&
		strings.TrimSpace(parsed.User) == "" &&
		strings.TrimSpace(parsed.Password) == "" &&
		strings.Trim(strings.TrimSpace(parsed.Path), "/") == "" &&
		len(parsed.Query) == 0 && len(parsed.QueryAdd) == 0 &&
		len(parsed.QueryDel) == 0 && len(parsed.QueryPayload) == 0 {
		return &EmptyURLError{Schema: parsed.Scheme}
	}

	if rule.RejectsInvalid && host != "" && !isValidHostname(host) {
		return &InvalidHostError{Schema: parsed.Scheme, Host: host}
	}

	return nil
}
