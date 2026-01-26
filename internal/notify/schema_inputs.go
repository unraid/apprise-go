package notify

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type SchemaInputs struct {
	Values  map[string]SchemaValue
	Kwargs  map[string]map[string]string
	Aliases map[string]string
}

func SchemaInputsFromURL(schema, raw string) (SchemaInputs, error) {
	parsed, err := ParseURL(raw)
	if err != nil {
		return SchemaInputs{}, err
	}
	return SchemaInputsForParsed(schema, parsed)
}

func SchemaInputsForParsed(schema string, target *ParsedURL) (SchemaInputs, error) {
	entry, ok := SchemaEntryForSchema(schema)
	if !ok {
		return SchemaInputs{}, fmt.Errorf("schema not found: %s", schema)
	}

	specs, err := parseSchemaSpecs(entry)
	if err != nil {
		return SchemaInputs{}, err
	}

	values := map[string]SchemaValue{}
	aliases := map[string]string{}
	kwargs := map[string]map[string]string{}

	tokenValues := matchSchemaTemplates(specs.templates, specs.tokens, target)
	applyTokenDefaults(specs.tokens, tokenValues, target)

	for name, spec := range specs.tokens {
		if alias := specAlias(spec); alias != "" {
			aliases[name] = alias
			continue
		}
		mapTo := specMapTo(spec, name)
		if raw, ok := tokenValues[name]; ok {
			applySchemaValue(values, mapTo, raw, spec)
		}
	}

	for name, spec := range specs.args {
		if alias := specAlias(spec); alias != "" {
			aliases[name] = alias
			if raw, ok := target.Query[strings.ToLower(name)]; ok {
				applySchemaAliasValue(values, specs, alias, raw)
			}
			continue
		}
		mapTo := specMapTo(spec, name)
		if raw, ok := target.Query[strings.ToLower(name)]; ok {
			applySchemaValue(values, mapTo, raw, spec)
			continue
		}
		if def, ok := specDefault(spec); ok {
			if _, exists := values[mapTo]; !exists {
				applySchemaValue(values, mapTo, def, spec)
			}
		}
	}

	for name, spec := range specs.kwargs {
		if alias := specAlias(spec); alias != "" {
			aliases[name] = alias
			continue
		}
		mapTo := specMapTo(spec, name)
		prefix := specPrefix(spec)
		source := map[string]string{}
		switch prefix {
		case "+":
			source = target.QueryAdd
		case "-":
			source = target.QueryDel
		case ":":
			source = target.QueryPayload
		case "":
			source = target.Query
		}
		if len(source) == 0 {
			continue
		}
		out := map[string]string{}
		for key, value := range source {
			out[key] = value
		}
		if len(out) > 0 {
			kwargs[mapTo] = out
		}
	}

	ApplySchemaOverrides(schema, target, values)

	return SchemaInputs{
		Values:  values,
		Kwargs:  kwargs,
		Aliases: aliases,
	}, nil
}

func (s SchemaInputs) ValuesMap() map[string]any {
	values := map[string]any{}
	for key, value := range s.Values {
		values[key] = value.Value
	}
	return values
}

type schemaSpecs struct {
	templates []string
	tokens    map[string]map[string]any
	args      map[string]map[string]any
	kwargs    map[string]map[string]any
}

func parseSchemaSpecs(entry SchemaEntry) (schemaSpecs, error) {
	details, ok := entry["details"].(map[string]any)
	if !ok {
		return schemaSpecs{}, fmt.Errorf("schema entry missing details")
	}

	templates := []string{}
	if rawTemplates, ok := details["templates"]; ok {
		switch typed := rawTemplates.(type) {
		case []string:
			templates = append(templates, typed...)
		case []any:
			for _, item := range typed {
				if item == nil {
					continue
				}
				templates = append(templates, fmt.Sprint(item))
			}
		}
	}

	return schemaSpecs{
		templates: templates,
		tokens:    castSpecMap(details["tokens"]),
		args:      castSpecMap(details["args"]),
		kwargs:    castSpecMap(details["kwargs"]),
	}, nil
}

func castSpecMap(raw any) map[string]map[string]any {
	out := map[string]map[string]any{}
	if raw == nil {
		return out
	}
	value, ok := raw.(map[string]any)
	if !ok {
		return out
	}
	for key, entry := range value {
		spec, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		out[key] = spec
	}
	return out
}

func specAlias(spec map[string]any) string {
	if spec == nil {
		return ""
	}
	if raw, ok := spec["alias_of"]; ok && raw != nil {
		return strings.TrimSpace(fmt.Sprint(raw))
	}
	return ""
}

func specMapTo(spec map[string]any, fallback string) string {
	if spec == nil {
		return fallback
	}
	if raw, ok := spec["map_to"]; ok && raw != nil {
		value := strings.TrimSpace(fmt.Sprint(raw))
		if value != "" {
			return value
		}
	}
	return fallback
}

func specType(spec map[string]any) string {
	if spec == nil {
		return "string"
	}
	if raw, ok := spec["type"]; ok && raw != nil {
		value := strings.TrimSpace(fmt.Sprint(raw))
		if value != "" {
			return value
		}
	}
	return "string"
}

func specPrefix(spec map[string]any) string {
	if spec == nil {
		return ""
	}
	if raw, ok := spec["prefix"]; ok && raw != nil {
		return fmt.Sprint(raw)
	}
	return ""
}

func specDefault(spec map[string]any) (any, bool) {
	if spec == nil {
		return nil, false
	}
	if raw, ok := spec["default"]; ok {
		if raw == nil {
			return nil, false
		}
		return raw, true
	}
	return nil, false
}

func applySchemaAliasValue(values map[string]SchemaValue, specs schemaSpecs, alias string, raw string) {
	if alias == "" {
		return
	}
	if spec, ok := specs.args[alias]; ok {
		applySchemaValue(values, specMapTo(spec, alias), raw, spec)
		return
	}
	if spec, ok := specs.tokens[alias]; ok {
		applySchemaValue(values, specMapTo(spec, alias), raw, spec)
	}
}

func applySchemaValue(values map[string]SchemaValue, mapTo string, raw any, spec map[string]any) {
	if mapTo == "" {
		return
	}

	if isListType(spec) {
		list := coerceList(raw, spec)
		if len(list) == 0 {
			return
		}
		if existing, ok := values[mapTo]; ok {
			if existingList, ok := existing.Value.([]string); ok {
				values[mapTo] = schemaValueList(append(existingList, list...))
				return
			}
		}
		values[mapTo] = schemaValueList(list)
		return
	}

	switch normalizeType(specType(spec)) {
	case "bool":
		values[mapTo] = schemaValueBool(coerceBool(raw))
	case "int":
		values[mapTo] = schemaValueInt(coerceInt(raw))
	case "float":
		values[mapTo] = schemaValueFloat(coerceFloat(raw))
	default:
		values[mapTo] = schemaValueAny(coerceString(raw))
	}
}

func normalizeType(value string) string {
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "list") {
		return "list"
	}
	if strings.HasPrefix(lower, "choice") {
		if strings.Contains(lower, "bool") {
			return "bool"
		}
		if strings.Contains(lower, "int") {
			return "int"
		}
		if strings.Contains(lower, "float") {
			return "float"
		}
		return "string"
	}
	if strings.HasPrefix(lower, "bool") {
		return "bool"
	}
	if strings.HasPrefix(lower, "int") {
		return "int"
	}
	if strings.HasPrefix(lower, "float") {
		return "float"
	}
	return "string"
}

func isListType(spec map[string]any) bool {
	return normalizeType(specType(spec)) == "list"
}

func coerceString(raw any) string {
	if raw == nil {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return value
	default:
		return fmt.Sprint(value)
	}
}

func coerceBool(raw any) bool {
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		return parseBool(value, false)
	default:
		return parseBool(fmt.Sprint(value), false)
	}
}

func coerceInt(raw any) int {
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			return parsed
		}
	default:
		if parsed, err := strconv.Atoi(fmt.Sprint(value)); err == nil {
			return parsed
		}
	}
	return 0
}

func coerceFloat(raw any) float64 {
	switch value := raw.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case string:
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
			return parsed
		}
	default:
		if parsed, err := strconv.ParseFloat(fmt.Sprint(value), 64); err == nil {
			return parsed
		}
	}
	return 0
}

func coerceList(raw any, spec map[string]any) []string {
	if raw == nil {
		return nil
	}
	switch value := raw.(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if item == nil {
				continue
			}
			out = append(out, fmt.Sprint(item))
		}
		return out
	case string:
		return splitByDelims(value, specDelims(spec))
	default:
		return []string{fmt.Sprint(value)}
	}
}

func specDelims(spec map[string]any) []string {
	if spec == nil {
		return nil
	}
	if raw, ok := spec["delim"]; ok {
		switch typed := raw.(type) {
		case []string:
			return append([]string(nil), typed...)
		case []any:
			out := make([]string, 0, len(typed))
			for _, item := range typed {
				if item == nil {
					continue
				}
				out = append(out, fmt.Sprint(item))
			}
			return out
		case string:
			if typed != "" {
				return []string{typed}
			}
		}
	}
	return nil
}

func splitByDelims(raw string, delims []string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	if len(delims) == 0 {
		return []string{trimmed}
	}
	pattern := buildDelimRegex(delims)
	re := regexp.MustCompile(pattern)
	parts := re.Split(trimmed, -1)
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values = append(values, part)
	}
	return values
}

func buildDelimRegex(delims []string) string {
	uniq := map[string]struct{}{}
	for _, delim := range delims {
		delim = strings.TrimSpace(delim)
		if delim == "" {
			continue
		}
		uniq[regexp.QuoteMeta(delim)] = struct{}{}
	}
	if len(uniq) == 0 {
		return "[\\s,]+"
	}
	parts := make([]string, 0, len(uniq))
	for delim := range uniq {
		parts = append(parts, delim)
	}
	sort.Strings(parts)
	return "(?:" + strings.Join(parts, "|") + ")+"
}

func matchSchemaTemplates(templates []string, specs map[string]map[string]any, target *ParsedURL) map[string]any {
	for _, template := range templates {
		values, ok := matchSchemaTemplate(template, specs, target)
		if ok {
			return values
		}
	}
	return map[string]any{}
}

func applyTokenDefaults(specs map[string]map[string]any, values map[string]any, target *ParsedURL) {
	if _, ok := values["schema"]; !ok {
		values["schema"] = target.Scheme
	}
	if _, ok := values["host"]; !ok && target.Host != "" {
		values["host"] = target.Host
	}
	if _, ok := values["user"]; !ok && target.HasUser {
		values["user"] = target.User
	}
	if _, ok := values["password"]; !ok && target.HasPassword {
		values["password"] = target.Password
	}
	if _, ok := values["port"]; !ok && target.HasPort {
		values["port"] = target.Port
	}

	for name, spec := range specs {
		mapTo := specMapTo(spec, name)
		if _, ok := values[name]; ok {
			continue
		}
		if _, ok := values[mapTo]; ok {
			continue
		}
		if def, ok := specDefault(spec); ok {
			values[name] = def
		}
	}
}

type segmentPart struct {
	isToken bool
	value   string
}

func matchSchemaTemplate(template string, specs map[string]map[string]any, target *ParsedURL) (map[string]any, bool) {
	parts := strings.SplitN(template, "://", 2)
	if len(parts) != 2 {
		return nil, false
	}

	schemeTemplate := parts[0]
	rest := parts[1]

	values := map[string]any{}

	if token, ok := exactToken(schemeTemplate); ok {
		values[token] = target.Scheme
	} else if !strings.EqualFold(schemeTemplate, target.Scheme) {
		return nil, false
	}

	authority := rest
	pathTemplate := ""
	if idx := strings.Index(rest, "/"); idx != -1 {
		authority = rest[:idx]
		pathTemplate = rest[idx+1:]
	}

	if !matchTemplateAuthority(authority, specs, target, values) {
		return nil, false
	}

	if !matchTemplatePath(pathTemplate, specs, target, values) {
		return nil, false
	}

	return values, true
}

func matchTemplateAuthority(template string, specs map[string]map[string]any, target *ParsedURL, values map[string]any) bool {
	userinfoTemplate := ""
	hostTemplate := template
	if idx := strings.LastIndex(template, "@"); idx != -1 {
		userinfoTemplate = template[:idx]
		hostTemplate = template[idx+1:]
	}

	if userinfoTemplate == "" && target.HasUser {
		return false
	}
	if userinfoTemplate != "" && !target.HasUser {
		return false
	}

	if userinfoTemplate != "" {
		userTemplate := userinfoTemplate
		passTemplate := ""
		if idx := strings.Index(userinfoTemplate, ":"); idx != -1 {
			userTemplate = userinfoTemplate[:idx]
			passTemplate = userinfoTemplate[idx+1:]
		}

		if token, ok := exactToken(userTemplate); ok {
			values[token] = target.User
		} else if userTemplate != "" && userTemplate != target.User {
			return false
		}

		if passTemplate != "" {
			if !target.HasPassword {
				return false
			}
			if token, ok := exactToken(passTemplate); ok {
				values[token] = target.Password
			} else if passTemplate != target.Password {
				return false
			}
		} else if target.HasPassword {
			return false
		}
	}

	if hostTemplate == "" {
		return target.Host == ""
	}

	portTemplate := ""
	hostPart := hostTemplate
	if idx := strings.LastIndex(hostTemplate, ":"); idx != -1 {
		hostPart = hostTemplate[:idx]
		portTemplate = hostTemplate[idx+1:]
	}

	if portTemplate == "" && target.HasPort {
		return false
	}
	if portTemplate != "" && !target.HasPort {
		return false
	}

	if token, ok := exactToken(hostPart); ok {
		values[token] = target.Host
	} else if hostPart != "" && !strings.EqualFold(hostPart, target.Host) {
		return false
	}

	if portTemplate != "" {
		portValue := strconv.Itoa(target.Port)
		if token, ok := exactToken(portTemplate); ok {
			values[token] = portValue
		} else if portTemplate != portValue {
			return false
		}
	}

	return true
}

func matchTemplatePath(template string, specs map[string]map[string]any, target *ParsedURL, values map[string]any) bool {
	segments := splitPathSegmentsLocal(target.Path)
	if template == "" {
		return len(segments) == 0
	}

	patternSegments := splitTemplateSegments(template)
	if len(patternSegments) == 0 {
		return len(segments) == 0
	}

	idx := 0
	for i, pattern := range patternSegments {
		if idx > len(segments) {
			return false
		}
		if len(pattern) == 1 && pattern[0].isToken {
			token := pattern[0].value
			spec := specs[token]
			if isListType(spec) && (i == len(patternSegments)-1 || delimContains(spec, "/")) {
				if idx >= len(segments) {
					values[token] = []string{}
					idx = len(segments)
					continue
				}
				remaining := append([]string(nil), segments[idx:]...)
				values[token] = remaining
				idx = len(segments)
				continue
			}
			if idx >= len(segments) {
				return false
			}
			values[token] = segments[idx]
			idx++
			continue
		}

		if idx >= len(segments) {
			return false
		}
		segment := segments[idx]
		matched, ok := matchComplexSegment(pattern, segment, specs)
		if !ok {
			return false
		}
		for key, value := range matched {
			values[key] = value
		}
		idx++
	}

	return idx == len(segments)
}

func splitPathSegmentsLocal(rawPath string) []string {
	path := strings.Trim(rawPath, "/")
	if path == "" {
		return nil
	}
	parts := strings.Split(path, "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		decoded := decodeQueryValue(part)
		decoded = strings.TrimSpace(decoded)
		if decoded != "" {
			segments = append(segments, decoded)
		}
	}
	return segments
}

func splitTemplateSegments(template string) [][]segmentPart {
	parts := strings.Split(strings.Trim(template, "/"), "/")
	segments := make([][]segmentPart, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		segments = append(segments, parseSegmentParts(part))
	}
	return segments
}

func parseSegmentParts(segment string) []segmentPart {
	parts := []segmentPart{}
	for len(segment) > 0 {
		start := strings.Index(segment, "{")
		if start == -1 {
			parts = append(parts, segmentPart{isToken: false, value: segment})
			break
		}
		if start > 0 {
			parts = append(parts, segmentPart{isToken: false, value: segment[:start]})
			segment = segment[start:]
		}
		end := strings.Index(segment, "}")
		if end == -1 {
			parts = append(parts, segmentPart{isToken: false, value: segment})
			break
		}
		token := segment[1:end]
		parts = append(parts, segmentPart{isToken: true, value: token})
		segment = segment[end+1:]
	}
	return parts
}

func matchComplexSegment(pattern []segmentPart, segment string, specs map[string]map[string]any) (map[string]string, bool) {
	if len(pattern) == 1 && !pattern[0].isToken {
		if pattern[0].value == segment {
			return map[string]string{}, true
		}
		return nil, false
	}

	regex, tokens, ok := buildSegmentRegex(pattern, specs)
	if !ok {
		return nil, false
	}

	match := regex.FindStringSubmatch(segment)
	if match == nil {
		return nil, false
	}

	values := map[string]string{}
	for idx, token := range tokens {
		if idx+1 >= len(match) {
			continue
		}
		values[token] = match[idx+1]
	}
	return values, true
}

func buildSegmentRegex(pattern []segmentPart, specs map[string]map[string]any) (*regexp.Regexp, []string, bool) {
	var builder strings.Builder
	tokens := []string{}
	caseInsensitive := false
	for _, part := range pattern {
		if !part.isToken {
			builder.WriteString(regexp.QuoteMeta(part.value))
			continue
		}
		spec := specs[part.value]
		regex, flags := tokenRegex(spec, "[^/]+")
		if strings.Contains(flags, "i") {
			caseInsensitive = true
		}
		builder.WriteString("(")
		builder.WriteString(regex)
		builder.WriteString(")")
		tokens = append(tokens, part.value)
	}

	patternStr := builder.String()
	if caseInsensitive {
		patternStr = "(?i)" + patternStr
	}
	compiled, err := regexp.Compile("^" + patternStr + "$")
	if err != nil {
		return nil, nil, false
	}
	return compiled, tokens, true
}

func tokenRegex(spec map[string]any, fallback string) (string, string) {
	regex := ""
	flags := ""
	if spec != nil {
		if raw, ok := spec["regex"]; ok && raw != nil {
			switch typed := raw.(type) {
			case []string:
				if len(typed) > 0 {
					regex = typed[0]
				}
				if len(typed) > 1 {
					flags = typed[1]
				}
			case []any:
				if len(typed) > 0 {
					regex = fmt.Sprint(typed[0])
				}
				if len(typed) > 1 {
					flags = fmt.Sprint(typed[1])
				}
			case string:
				regex = typed
			}
		}
	}
	regex = strings.TrimSpace(regex)
	regex = strings.TrimPrefix(regex, "^")
	regex = strings.TrimSuffix(regex, "$")
	if regex == "" {
		regex = fallback
	}
	return regex, flags
}

func exactToken(value string) (string, bool) {
	if len(value) > 2 && strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
		inner := value[1 : len(value)-1]
		if strings.Contains(inner, "{") || strings.Contains(inner, "}") {
			return "", false
		}
		return inner, true
	}
	return "", false
}

func delimContains(spec map[string]any, delim string) bool {
	for _, entry := range specDelims(spec) {
		if entry == delim {
			return true
		}
	}
	return false
}
