package notify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// renderNotifyTemplate reads a JSON payload template, substitutes the standard
// app_* tokens plus any the URL supplied, and returns the parsed object.
//
// Templates define the entire message, so a malformed one has to be an error
// rather than a silent fallback: sending the plugin's own payload instead
// would quietly ignore what the user asked for.
func renderNotifyTemplate(
	path string,
	extra map[string]string,
	body, title string,
	notifyType NotifyType,
	imageSize string,
) (map[string]any, error) {
	path = strings.TrimPrefix(strings.TrimSpace(path), "file://")
	if path == "" {
		return nil, fmt.Errorf("missing template path")
	}
	if !filepath.IsAbs(path) {
		if moduleRoot, ok := findModuleRoot(); ok {
			path = filepath.Join(moduleRoot, path)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("template %s could not be read: %w", path, err)
	}

	tokens := map[string]string{}
	for key, value := range extra {
		tokens[key] = value
	}
	tokens["app_body"] = body
	tokens["app_title"] = title
	tokens["app_type"] = string(notifyType)
	tokens["app_id"] = "Apprise"
	tokens["app_desc"] = appriseAppDesc
	tokens["app_color"] = appriseColor(notifyType)
	// app_color_hex names the hex variant unambiguously alongside the decimal
	// app_color_int, which embeds require.
	tokens["app_color_hex"] = appriseColor(notifyType)
	tokens["app_color_int"] = strconv.Itoa(appriseColorInt(notifyType))
	tokens["app_image_url"] = appriseImageURL(notifyType, imageSize)
	tokens["app_url"] = appriseAppURL
	// Templates are always JSON, so substitutions are JSON-escaped.
	tokens["app_mode"] = "json"

	var payload map[string]any
	rendered := applyTemplateTokens(string(data), tokens)
	if err := json.Unmarshal([]byte(rendered), &payload); err != nil {
		return nil, fmt.Errorf("template %s contains invalid JSON: %w", path, err)
	}

	return payload, nil
}
