package notify

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const pushSaferDefaultIcon = 25

var pushSaferSoundMap = map[string]int{
	"ahem":             1,
	"alarmarmed":       51,
	"alarmdisarmed":    52,
	"applause":         2,
	"armed":            51,
	"arrow":            3,
	"baby":             4,
	"backupready":      53,
	"beep":             38,
	"beep1":            38,
	"beep2":            48,
	"beep3":            49,
	"beep4":            50,
	"bell":             5,
	"bicycle":          6,
	"bike":             6,
	"boing":            7,
	"buzzer":           8,
	"camera":           9,
	"carhorn":          10,
	"cashregister":     11,
	"chime":            12,
	"creakydoor":       13,
	"cuckoo":           14,
	"cuckooclock":      14,
	"cutinout":         40,
	"dclosed":          54,
	"disarmed":         52,
	"disconnect":       15,
	"dog":              16,
	"doorbell":         17,
	"doorbellrang":     60,
	"doorclosed":       54,
	"dooropen":         55,
	"dopen":            55,
	"echo":             45,
	"electric":         23,
	"fanfare":          18,
	"flickglass":       41,
	"goodye":           29,
	"gunshot":          19,
	"hello":            30,
	"hihat":            47,
	"honk":             20,
	"horn":             10,
	"jawharp":          21,
	"laser":            43,
	"lightoff":         59,
	"lighton":          58,
	"loff":             59,
	"lon":              58,
	"military":         26,
	"militarytrumpets": 26,
	"morse":            22,
	"no":               31,
	"ok":               32,
	"okay":             32,
	"ooohhhweee":       33,
	"radiotuner":       24,
	"silent":           0,
	"sirens":           25,
	"trumpets":         26,
	"ufo":              27,
	"warn":             34,
	"warning":          34,
	"wclosed":          56,
	"wee":              39,
	"weee":             39,
	"welcome":          35,
	"whah":             42,
	"whahwhah":         28,
	"windchime":        44,
	"windowclosed":     56,
	"windowopen":       57,
	"wopen":            57,
	"yeah":             36,
	"yes":              37,
	"zipper":           46,
}

var pushSaferSoundOrder = []string{
	"silent",
	"ahem",
	"applause",
	"arrow",
	"baby",
	"bell",
	"bicycle",
	"bike",
	"boing",
	"buzzer",
	"camera",
	"carhorn",
	"horn",
	"cashregister",
	"chime",
	"creakydoor",
	"cuckooclock",
	"cuckoo",
	"disconnect",
	"dog",
	"doorbell",
	"fanfare",
	"gunshot",
	"honk",
	"jawharp",
	"morse",
	"electric",
	"radiotuner",
	"sirens",
	"militarytrumpets",
	"military",
	"trumpets",
	"ufo",
	"whahwhah",
	"whah",
	"goodye",
	"hello",
	"no",
	"okay",
	"ok",
	"ooohhhweee",
	"warn",
	"warning",
	"welcome",
	"yeah",
	"yes",
	"beep",
	"beep1",
	"weee",
	"wee",
	"cutinout",
	"flickglass",
	"laser",
	"windchime",
	"echo",
	"zipper",
	"hihat",
	"beep2",
	"beep3",
	"beep4",
	"alarmarmed",
	"armed",
	"alarmdisarmed",
	"disarmed",
	"backupready",
	"dooropen",
	"dopen",
	"doorclosed",
	"dclosed",
	"windowopen",
	"wopen",
	"windowclosed",
	"wclosed",
	"lighton",
	"lon",
	"lightoff",
	"loff",
	"doorbellrang",
}

type PushSaferTarget struct {
	privateKey string
	targets    []string
	sound      *int
	vibration  *int
	secure     bool
}

func NewPushSaferTarget(target *ParsedURL) (*PushSaferTarget, error) {
	privateKey := strings.TrimSpace(target.Host)
	if privateKey == "" {
		return nil, fmt.Errorf("missing private key")
	}

	targets := splitPath(target.Path)
	if toRaw := strings.TrimSpace(target.Query["to"]); toRaw != "" {
		targets = append(targets, parseDelimitedList(toRaw)...)
	}
	if len(targets) == 0 {
		targets = []string{"a"}
	}

	sound := parsePushSaferSound(target.Query["sound"])
	vibration := parseOptionalInt(target.Query["vibration"])

	return &PushSaferTarget{
		privateKey: privateKey,
		targets:    targets,
		sound:      sound,
		vibration:  vibration,
		secure:     strings.EqualFold(target.Scheme, "psafers"),
	}, nil
}

func (p *PushSaferTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if len(p.targets) == 0 {
		return RequestSpec{}, fmt.Errorf("missing targets")
	}

	spec := p.buildSpec(body, title, notifyType, p.targets[0])
	return spec, nil
}

func (p *PushSaferTarget) Send(body, title string, notifyType NotifyType) error {
	if len(p.targets) == 0 {
		return fmt.Errorf("missing targets")
	}

	for _, recipient := range p.targets {
		spec := p.buildSpec(body, title, notifyType, recipient)
		if err := SendRequest(spec); err != nil {
			return err
		}
	}

	return nil
}

func (p *PushSaferTarget) buildSpec(body, title string, notifyType NotifyType, recipient string) RequestSpec {
	values := url.Values{}
	values.Set("t", title)
	values.Set("m", body)
	values.Set("i", strconv.Itoa(pushSaferDefaultIcon))
	values.Set("c", appriseColor(notifyType))
	values.Set("d", recipient)
	values.Set("k", p.privateKey)

	if p.sound != nil {
		values.Set("s", strconv.Itoa(*p.sound))
	}
	if p.vibration != nil {
		values.Set("v", strconv.Itoa(*p.vibration))
	}

	scheme := "http"
	if p.secure {
		scheme = "https"
	}

	return RequestSpec{
		Method: "POST",
		URL:    scheme + "://www.pushsafer.com/api",
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Content-Type": "application/x-www-form-urlencoded",
		},
		Body: values.Encode(),
	}
}

func parseOptionalInt(raw string) *int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &value
}

func parsePushSaferSound(raw string) *int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if value, err := strconv.Atoi(raw); err == nil {
		return &value
	}
	normalized := strings.ToLower(raw)
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, "_", "")
	normalized = strings.ReplaceAll(normalized, " ", "")
	for _, key := range pushSaferSoundOrder {
		if strings.HasPrefix(key, normalized) {
			if value, ok := pushSaferSoundMap[key]; ok {
				return &value
			}
		}
	}
	return nil
}

func init() {
	RegisterSchemaEntryOrdered(63, SchemaEntry{
		"attachment_support": true,
		"category":           "native",
		"details": map[string]any{
			"args": map[string]any{
				"cto": map[string]any{
					"default":  4,
					"map_to":   "cto",
					"name":     "Socket Connect Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"emojis": map[string]any{
					"default":  false,
					"map_to":   "emojis",
					"name":     "Interpret Emojis",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"format": map[string]any{
					"default":  "text",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"overflow": map[string]any{
					"default":  "upstream",
					"map_to":   "overflow",
					"name":     "Overflow Mode",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"split", "truncate", "upstream"},
				},
				"priority": map[string]any{
					"map_to":   "priority",
					"name":     "Priority",
					"private":  false,
					"required": false,
					"type":     "choice:int",
					"values":   []any{-2, -1, 0, 1, 2},
				},
				"rto": map[string]any{
					"default":  4,
					"map_to":   "rto",
					"name":     "Socket Read Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"sound": map[string]any{
					"map_to":   "sound",
					"name":     "Sound",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"silent", "ahem", "applause", "arrow", "baby", "bell", "bicycle", "bike", "boing", "buzzer", "camera", "carhorn", "horn", "cashregister", "chime", "creakydoor", "cuckooclock", "cuckoo", "disconnect", "dog", "doorbell", "fanfare", "gunshot", "honk", "jawharp", "morse", "electric", "radiotuner", "sirens", "militarytrumpets", "military", "trumpets", "ufo", "whahwhah", "whah", "goodye", "hello", "no", "okay", "ok", "ooohhhweee", "warn", "warning", "welcome", "yeah", "yes", "beep", "beep1", "weee", "wee", "cutinout", "flickglass", "laser", "windchime", "echo", "zipper", "hihat", "beep2", "beep3", "beep4", "alarmarmed", "armed", "alarmdisarmed", "disarmed", "backupready", "dooropen", "dopen", "doorclosed", "dclosed", "windowopen", "wopen", "windowclosed", "wclosed", "lighton", "lon", "lightoff", "loff", "doorbellrang"},
				},
				"store": map[string]any{
					"default":  true,
					"map_to":   "store",
					"name":     "Persistent Storage",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"to": map[string]any{
					"alias_of": "targets",
					"delim":    []string{",", " "},
				},
				"tz": map[string]any{
					"default":  nil,
					"map_to":   "tz",
					"name":     "Timezone",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"verify": map[string]any{
					"default":  true,
					"map_to":   "verify",
					"name":     "Verify SSL",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"vibration": map[string]any{
					"map_to":   "vibration",
					"name":     "Vibration",
					"private":  false,
					"required": false,
					"type":     "choice:int",
					"values":   []any{1, 2, 3},
				},
			},
			"kwargs":    map[string]any{},
			"templates": []string{"{schema}://{privatekey}", "{schema}://{privatekey}/{targets}"},
			"tokens": map[string]any{
				"privatekey": map[string]any{
					"map_to":   "privatekey",
					"name":     "Private Key",
					"private":  true,
					"required": true,
					"type":     "string",
				},
				"schema": map[string]any{
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"psafer", "psafers"},
				},
				"target_device": map[string]any{
					"map_to":   "targets",
					"name":     "Target Device",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"target_email": map[string]any{
					"map_to":   "targets",
					"name":     "Target Email",
					"private":  false,
					"required": false,
					"type":     "string",
				},
				"targets": map[string]any{
					"delim":    []string{"/"},
					"group":    []string{"target_device", "target_email"},
					"map_to":   "targets",
					"name":     "Targets",
					"private":  false,
					"required": false,
					"type":     "list:string",
				},
			},
		},
		"enabled":   true,
		"protocols": []string{"psafer"},
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"psafers"},
		"service_name":     "Pushsafer",
		"service_url":      "https://www.pushsafer.com/",
		"setup_url":        "https://appriseit.com/services/pushsafer/",
	})
}
