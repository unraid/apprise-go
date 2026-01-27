package notify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type MSTeamsTarget struct {
	team           string
	tokenA         string
	tokenB         string
	tokenC         string
	tokenD         string
	version        int
	includeImage   bool
	templatePath   string
	templateTokens map[string]string
}

func NewMSTeamsTarget(target *ParsedURL) (*MSTeamsTarget, error) {
	entries := splitPath(target.Path)

	rawHost := strings.TrimSpace(target.Host)
	tokenA := ""
	if rawHost == "" {
		return nil, fmt.Errorf("missing team")
	}
	team := ""
	tokenB := ""
	tokenC := ""
	tokenD := ""
	if strings.TrimSpace(target.User) != "" {
		tokenA = strings.TrimSpace(target.User) + "@" + rawHost
		if len(entries) > 0 {
			tokenB = entries[0]
		}
		if len(entries) > 1 {
			tokenC = entries[1]
		}
		if len(entries) > 2 {
			tokenD = entries[2]
		}
	} else if strings.Contains(rawHost, "@") {
		tokenA = rawHost
		if len(entries) > 0 {
			tokenB = entries[0]
		}
		if len(entries) > 1 {
			tokenC = entries[1]
		}
		if len(entries) > 2 {
			tokenD = entries[2]
		}
	} else {
		team = rawHost
		if len(entries) > 0 {
			tokenA = entries[0]
		}
		if len(entries) > 1 {
			tokenB = entries[1]
		}
		if len(entries) > 2 {
			tokenC = entries[2]
		}
		if len(entries) > 3 {
			tokenD = entries[3]
		}
	}

	version := 1
	if team != "" {
		version = 2
	}
	if tokenD != "" {
		version = 3
	}
	if rawVersion := strings.TrimSpace(target.Query["version"]); rawVersion != "" {
		if rawVersion == "1" {
			version = 1
		} else if rawVersion == "2" {
			version = 2
		} else if rawVersion == "3" {
			version = 3
		} else {
			return nil, fmt.Errorf("invalid version: %s", rawVersion)
		}
	}
	if team == "" && version > 1 {
		return nil, fmt.Errorf("missing team")
	}

	includeImage := parseBool(target.Query["image"], true)

	templatePath := strings.TrimSpace(target.Query["template"])
	templateTokens := map[string]string{}
	for key, value := range target.QueryPayload {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		templateTokens[key] = value
	}

	return &MSTeamsTarget{
		team:           team,
		tokenA:         tokenA,
		tokenB:         tokenB,
		tokenC:         tokenC,
		tokenD:         tokenD,
		version:        version,
		includeImage:   includeImage,
		templatePath:   templatePath,
		templateTokens: templateTokens,
	}, nil
}

func (m *MSTeamsTarget) Send(body, title string, notifyType NotifyType) error {
	spec, err := m.BuildRequest(body, title, notifyType)
	if err != nil {
		return err
	}

	return SendRequest(spec)
}

func (m *MSTeamsTarget) BuildRequest(body, title string, notifyType NotifyType) (RequestSpec, error) {
	if m.tokenA == "" || m.tokenB == "" || m.tokenC == "" {
		return RequestSpec{}, fmt.Errorf("missing tokens")
	}

	payload := map[string]any{}
	if strings.TrimSpace(m.templatePath) != "" {
		templatePayload, err := m.buildTemplatePayload(body, title, notifyType)
		if err != nil {
			return RequestSpec{}, err
		}
		payload = templatePayload
	} else {
		var imageURL any = nil
		if m.includeImage {
			imageURL = appriseImageURL(notifyType, "72x72")
		}
		payload = map[string]any{
			"@type":    "MessageCard",
			"@context": "https://schema.org/extensions",
			"summary":  "Apprise Notifications",
			"themeColor": appriseColor(
				notifyType,
			),
			"sections": []any{
				map[string]any{
					"activityImage": imageURL,
					"activityTitle": title,
					"text":          body,
				},
			},
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return RequestSpec{}, err
	}

	url := ""
	switch m.version {
	case 1:
		url = fmt.Sprintf("https://outlook.office.com/webhook/%s/IncomingWebhook/%s/%s", m.tokenA, m.tokenB, m.tokenC)
	case 2:
		url = fmt.Sprintf("https://%s.webhook.office.com/webhookb2/%s/IncomingWebhook/%s/%s", m.team, m.tokenA, m.tokenB, m.tokenC)
	case 3:
		tokenD := m.tokenD
		if tokenD == "" {
			tokenD = "None"
		}
		url = fmt.Sprintf("https://%s.webhook.office.com/webhookb2/%s/IncomingWebhook/%s/%s/%s", m.team, m.tokenA, m.tokenB, m.tokenC, tokenD)
	default:
		return RequestSpec{}, fmt.Errorf("unsupported version: %d", m.version)
	}

	return RequestSpec{
		Method: "POST",
		URL:    url,
		Headers: map[string]string{
			"User-Agent":   "Apprise",
			"Accept":       "*/*",
			"Content-Type": "application/json",
		},
		Body: string(data),
	}, nil
}

func (m *MSTeamsTarget) buildTemplatePayload(body, title string, notifyType NotifyType) (map[string]any, error) {
	path := strings.TrimSpace(m.templatePath)
	if strings.HasPrefix(path, "file://") {
		path = strings.TrimPrefix(path, "file://")
	}
	if path != "" && !filepath.IsAbs(path) {
		if moduleRoot, ok := findModuleRoot(); ok {
			path = filepath.Join(moduleRoot, path)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	tokens := map[string]string{}
	for key, value := range m.templateTokens {
		tokens[key] = value
	}
	tokens["app_body"] = body
	tokens["app_title"] = title
	tokens["app_type"] = string(notifyType)
	tokens["app_id"] = "Apprise"
	tokens["app_desc"] = "Apprise Notifications"
	tokens["app_color"] = appriseColor(notifyType)
	tokens["app_image_url"] = appriseImageURL(notifyType, "72x72")
	tokens["app_url"] = appriseAppURL
	tokens["app_mode"] = "json"

	rendered := applyTemplateTokens(string(data), tokens)

	var payload map[string]any
	if err := json.Unmarshal([]byte(rendered), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func init() {
	RegisterSchemaEntryOrdered(109, SchemaEntry{
		"attachment_support": false,
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
					"default":  "markdown",
					"map_to":   "format",
					"name":     "Notify Format",
					"private":  false,
					"required": false,
					"type":     "choice:string",
					"values":   []string{"html", "markdown", "text"},
				},
				"image": map[string]any{
					"default":  false,
					"map_to":   "include_image",
					"name":     "Include Image",
					"private":  false,
					"required": false,
					"type":     "bool",
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
				"rto": map[string]any{
					"default":  4,
					"map_to":   "rto",
					"name":     "Socket Read Timeout",
					"private":  false,
					"required": false,
					"type":     "float",
				},
				"store": map[string]any{
					"default":  true,
					"map_to":   "store",
					"name":     "Persistent Storage",
					"private":  false,
					"required": false,
					"type":     "bool",
				},
				"template": map[string]any{
					"map_to":   "template",
					"name":     "Template Path",
					"private":  true,
					"required": false,
					"type":     "string",
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
				"version": map[string]any{
					"default":  2,
					"map_to":   "version",
					"name":     "Version",
					"private":  false,
					"required": false,
					"type":     "choice:int",
					"values":   []any{1, 2, 3},
				},
			},
			"kwargs": map[string]any{
				"tokens": map[string]any{
					"map_to":   "tokens",
					"name":     "Template Tokens",
					"prefix":   ":",
					"private":  false,
					"required": false,
					"type":     "string",
				},
			},
			"templates": []string{"{schema}://{team}/{token_a}/{token_b}/{token_c}", "{schema}://{token_a}/{token_b}/{token_c}"},
			"tokens": map[string]any{
				"schema": map[string]any{
					"default":  "msteams",
					"map_to":   "schema",
					"name":     "Schema",
					"private":  false,
					"required": true,
					"type":     "choice:string",
					"values":   []string{"msteams"},
				},
				"team": map[string]any{
					"map_to":   "team",
					"name":     "Team Name",
					"private":  false,
					"regex":    []string{"^[A-Z0-9_-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"token_a": map[string]any{
					"map_to":   "token_a",
					"name":     "Token A",
					"private":  true,
					"regex":    []string{"^[A-Z0-9-]+@[A-Z0-9-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"token_b": map[string]any{
					"map_to":   "token_b",
					"name":     "Token B",
					"private":  true,
					"regex":    []string{"^[a-z0-9]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"token_c": map[string]any{
					"map_to":   "token_c",
					"name":     "Token C",
					"private":  true,
					"regex":    []string{"^[a-z0-9-]+$", "i"},
					"required": true,
					"type":     "string",
				},
				"token_d": map[string]any{
					"map_to":   "token_d",
					"name":     "Token D",
					"private":  true,
					"regex":    []string{"^V2[a-zA-Z0-9-_]+$", "i"},
					"required": false,
					"type":     "string",
				},
			},
		},
		"enabled":   true,
		"protocols": nil,
		"requirements": map[string]any{
			"details":              "",
			"packages_recommended": []any{},
			"packages_required":    []any{},
		},
		"secure_protocols": []string{"msteams"},
		"service_name":     "MSTeams",
		"service_url":      "https://teams.micrsoft.com/",
		"setup_url":        "https://appriseit.com/services/msteams/",
	})
}
