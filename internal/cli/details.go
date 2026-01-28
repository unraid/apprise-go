package cli

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

var schemaPrefixRe = regexp.MustCompile(`^[^}]+}://`)

func PrintDetails(w io.Writer) error {
	schema, err := LoadSchema()
	if err != nil {
		return err
	}

	rawEntries, ok := schema["schemas"].([]any)
	if !ok {
		return fmt.Errorf("schema: missing schemas list")
	}

	entries := make([]map[string]any, 0, len(rawEntries))
	for _, entry := range rawEntries {
		if typed, ok := entry.(map[string]any); ok {
			entries = append(entries, typed)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(getString(entries[i], "service_name")) < strings.ToLower(getString(entries[j], "service_name"))
	})

	for _, entry := range entries {
		serviceName := getString(entry, "service_name")
		category := getString(entry, "category")
		enabled := getBool(entry, "enabled", true)
		if category == "custom" {
			enabled = false
		}

		protocols := append(
			stringSlice(entry["protocols"]),
			stringSlice(entry["secure_protocols"])...,
		)

		templates := templateList(entry)
		if len(protocols) == 1 {
			for idx, template := range templates {
				templates[idx] = schemaPrefixRe.ReplaceAllString(template, protocols[0]+"://")
			}
		}

		label := fmt.Sprintf("%s %-30s ", indicator(enabled), serviceName)
		if enabled && len(protocols) > 1 {
			fmt.Fprintf(w, "%s| Schema(s): %s\n", label, strings.Join(protocols, ", "))
		} else {
			fmt.Fprintln(w, label)
			if len(protocols) > 1 {
				fmt.Fprintf(w, "| Schema(s): %s\n", strings.Join(protocols, ", "))
			}
		}

		if !enabled {
			requirements := mapStringAny(entry["requirements"])
			if detail := getString(requirements, "details"); detail != "" {
				fmt.Fprintf(w, "   %s\n", detail)
			}
			if packages := stringSlice(requirements["packages_required"]); len(packages) > 0 {
				fmt.Fprintln(w, "   Python Packages Required:")
				for _, req := range packages {
					fmt.Fprintf(w, "     - %s\n", req)
				}
			}
			if packages := stringSlice(requirements["packages_recommended"]); len(packages) > 0 {
				fmt.Fprintln(w, "   Python Packages Recommended:")
				for _, req := range packages {
					fmt.Fprintf(w, "     - %s\n", req)
				}
			}
			if category == "native" {
				fmt.Fprintln(w)
				continue
			}
		}

		prefix := "   - "
		for _, template := range templates {
			fmt.Fprintf(w, "%s%s\n", prefix, template)
		}
		fmt.Fprintln(w)
	}

	return nil
}

func templateList(entry map[string]any) []string {
	details := mapStringAny(entry["details"])
	return stringSlice(details["templates"])
}

func indicator(enabled bool) string {
	if enabled {
		return "+"
	}
	return "-"
}

func getString(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	if value, ok := data[key]; ok {
		if s, ok := value.(string); ok {
			return s
		}
	}
	return ""
}

func getBool(data map[string]any, key string, fallback bool) bool {
	if data == nil {
		return fallback
	}
	if value, ok := data[key]; ok {
		if b, ok := value.(bool); ok {
			return b
		}
	}
	return fallback
}

func mapStringAny(value any) map[string]any {
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return nil
}

func stringSlice(value any) []string {
	if value == nil {
		return nil
	}
	if typed, ok := value.([]any); ok {
		out := make([]string, 0, len(typed))
		for _, entry := range typed {
			if s, ok := entry.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	if typed, ok := value.([]string); ok {
		return typed
	}
	return nil
}
