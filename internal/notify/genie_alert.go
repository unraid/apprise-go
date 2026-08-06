package notify

import (
	"fmt"
	"regexp"
	"strings"
)

// Opsgenie and Jira Service Management are one plugin upstream wearing two
// names — jira.py is a copy of opsgenie.py, and says so in its own comment
// ("OpsGenie port - so keep us/en support for easy transition"). They share
// the GenieKey auth scheme, the alert payload, the target prefix syntax, the
// action mapping and the batching; only the endpoint and one header differ.
// Keeping the common parts here is what stops the two drifting apart.

const (
	genieBatchSize      = 50
	genieMessageMaxLen  = 130
	genieDefaultActions = "map"
)

// genieActions is ordered so that a prefix match resolves the way upstream's
// does; "map" is deliberately first and is excluded from mapping values.
var genieActions = []string{"map", "new", "close", "delete", "acknowledge", "note"}

var geniePriorityMap = map[string]int{
	"l":  1,
	"m":  2,
	"n":  3,
	"h":  4,
	"e":  5,
	"1":  1,
	"2":  2,
	"3":  3,
	"4":  4,
	"5":  5,
	"p1": 1,
	"p2": 2,
	"p3": 3,
	"p4": 4,
	"p5": 5,
}

// genieDefaultMap is upstream's notify-type to action mapping. Info adds a
// note to an existing alert rather than closing one.
var genieDefaultMap = map[NotifyType]string{
	NotifyInfo:    "note",
	NotifySuccess: "close",
	NotifyWarning: "new",
	NotifyFailure: "new",
}

var genieNotifyTypes = []NotifyType{NotifyInfo, NotifySuccess, NotifyWarning, NotifyFailure}

// genieAlert holds everything the two services parse identically.
type genieAlert struct {
	apiKey    string
	action    string
	mapping   map[NotifyType]string
	priority  int
	details   map[string]string
	entity    string
	alias     string
	tags      []string
	targets   []map[string]string
	user      string
	batchSize int
}

func parseGenieAlert(target *ParsedURL) (genieAlert, error) {
	alert := genieAlert{}

	// The API key is the host, or ?apikey= which is easier to express in YAML.
	alert.apiKey = strings.TrimSpace(target.Query["apikey"])
	if alert.apiKey == "" {
		alert.apiKey = strings.TrimSpace(target.Host)
	}
	if alert.apiKey == "" {
		return alert, fmt.Errorf("missing apikey")
	}

	action, err := parseGenieAction(target.Query["action"])
	if err != nil {
		return alert, err
	}
	alert.action = action

	mapping, err := parseGenieMapping(target.QueryPayload)
	if err != nil {
		return alert, err
	}
	alert.mapping = mapping

	alert.priority = parseGeniePriority(target.Query["priority"])

	// +key=value pairs become extra alert details.
	alert.details = map[string]string{}
	for key, value := range target.QueryAdd {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		alert.details[key] = value
	}

	alert.tags = []string{}
	if tagValue := strings.TrimSpace(target.Query["tags"]); tagValue != "" {
		alert.tags = append(alert.tags, parseDelimitedList(tagValue)...)
	}

	entries := splitPath(target.Path)
	if toValue := strings.TrimSpace(target.Query["to"]); toValue != "" {
		entries = append(entries, parseDelimitedList(toValue)...)
	}
	alert.targets = parseGenieTargets(entries)

	alert.entity = strings.TrimSpace(target.Query["entity"])
	alert.alias = strings.TrimSpace(target.Query["alias"])
	alert.user = strings.TrimSpace(target.User)

	alert.batchSize = 1
	if parseBoolWithDefault(target.Query["batch"], false) {
		alert.batchSize = genieBatchSize
	}

	return alert, nil
}

// resolveAction turns a notification type into the action to perform.
func (g *genieAlert) resolveAction(notifyType NotifyType) string {
	if g.action != "map" {
		return g.action
	}
	if action, ok := g.mapping[notifyType]; ok {
		return action
	}

	return genieDefaultMap[notifyType]
}

// responderBatches splits the targets the way upstream iterates them. A target
// list with no entries still yields one batch, since the alert is created
// without responders in that case.
func (g *genieAlert) responderBatches() [][]map[string]string {
	if len(g.targets) == 0 {
		return [][]map[string]string{nil}
	}

	size := g.batchSize
	if size <= 0 {
		size = 1
	}

	batches := make([][]map[string]string, 0, (len(g.targets)+size-1)/size)
	for start := 0; start < len(g.targets); start += size {
		end := min(start+size, len(g.targets))
		batches = append(batches, g.targets[start:end])
	}

	return batches
}

func (g *genieAlert) buildPayload(body, title string, notifyType NotifyType, responders []map[string]string) map[string]any {
	details := map[string]string{}
	for key, value := range g.details {
		details[key] = value
	}
	if _, ok := details["type"]; !ok {
		details["type"] = string(notifyType)
	}

	// The alert message is the title, falling back to the body when a
	// notification has no title.
	message := title
	if strings.TrimSpace(message) == "" {
		message = body
	}
	if len(message) > genieMessageMaxLen {
		message = message[:genieMessageMaxLen-3] + "..."
	}

	payload := map[string]any{
		"source":      "Apprise Notifications",
		"message":     message,
		"description": body,
		"details":     details,
		"priority":    fmt.Sprintf("P%d", g.priority),
	}

	if len(g.tags) > 0 {
		payload["tags"] = g.tags
	}
	if g.entity != "" {
		payload["entity"] = g.entity
	}
	if g.alias != "" {
		payload["alias"] = g.alias
	}
	if g.user != "" {
		payload["user"] = g.user
	}
	if len(responders) > 0 {
		payload["responders"] = responders
	}

	return payload
}

func parseGenieAction(raw string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return genieDefaultActions, nil
	}
	for _, candidate := range genieActions {
		if strings.HasPrefix(candidate, normalized) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("invalid action: %s", raw)
}

// parseGenieMapping reads the :type=action overrides. Both sides are resolved
// by prefix the way upstream does, so :info=n selects "new" — the first action
// after "map", which is not itself a valid mapping value.
func parseGenieMapping(payload map[string]string) (map[NotifyType]string, error) {
	if len(payload) == 0 {
		return nil, nil
	}

	mapping := map[NotifyType]string{}
	for rawType, rawAction := range payload {
		key := strings.ToLower(strings.TrimSpace(rawType))
		if key == "" {
			continue
		}

		var notifyType NotifyType
		for _, candidate := range genieNotifyTypes {
			if strings.HasPrefix(string(candidate), key) {
				notifyType = candidate
				break
			}
		}
		if notifyType == "" {
			return nil, fmt.Errorf("invalid mapping key: %s", rawType)
		}

		value := strings.ToLower(strings.TrimSpace(rawAction))
		action := ""
		for _, candidate := range genieActions[1:] {
			if strings.HasPrefix(candidate, value) {
				action = candidate
				break
			}
		}
		if action == "" {
			return nil, fmt.Errorf("invalid mapping value for %s: %s", rawType, rawAction)
		}

		mapping[notifyType] = action
	}

	return mapping, nil
}

func parseGeniePriority(raw string) int {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return 3
	}
	for key, value := range geniePriorityMap {
		if strings.HasPrefix(normalized, key) {
			return value
		}
	}

	return 3
}

// parseGenieTargets reads the prefix that decides what kind of responder each
// entry is: @user, #team, *schedule, ^escalation. A UUID is passed as an id
// and anything else by name.
func parseGenieTargets(entries []string) []map[string]string {
	targets := []map[string]string{}
	for _, entry := range sortedUniqueTargets(entries) {
		target := strings.TrimSpace(entry)
		if len(target) < 2 {
			continue
		}

		prefix := target[:1]
		value := target
		switch prefix {
		case "@", "#", "*", "^":
			value = strings.TrimSpace(target[1:])
		default:
			// An unprefixed entry is treated as a user.
			prefix = "@"
		}

		if value == "" {
			continue
		}

		kind, nameKey := "user", "username"
		switch prefix {
		case "#":
			kind, nameKey = "team", "name"
		case "*":
			kind, nameKey = "schedule", "name"
		case "^":
			kind, nameKey = "escalation", "name"
		}

		if isUUID(value) {
			targets = append(targets, map[string]string{"type": kind, "id": value})
			continue
		}
		targets = append(targets, map[string]string{"type": kind, nameKey: value})
	}

	return targets
}

var uuidPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func isUUID(value string) bool {
	return uuidPattern.MatchString(strings.ToLower(value))
}
