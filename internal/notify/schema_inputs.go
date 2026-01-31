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
	applyTokenDefaults(specs.tokens, specs.templates, tokenValues, target)

	for name, spec := range specs.tokens {
		if alias := specAlias(spec); alias != "" {
			aliases[name] = alias
			continue
		}
		mapTo := specMapTo(spec, name)
		if raw, ok := tokenValues[name]; ok {
			applySchemaValue(values, mapTo, raw, spec, false)
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
			applySchemaValue(values, mapTo, raw, spec, true)
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

	ensureEmptyKwargs(specs, kwargs)

	ensureListDefaults(specs, values)

	adjustSchemaValues(specs, target, values)

	ApplySchemaOverrides(schema, target, values)

	return SchemaInputs{
		Values:  values,
		Kwargs:  kwargs,
		Aliases: aliases,
	}, nil
}

func adjustSchemaValues(specs schemaSpecs, target *ParsedURL, values map[string]SchemaValue) {
	if target == nil {
		return
	}
	if target.Host == "" {
		sourceValue, ok := values["source"]
		if ok {
			if sourceStr, ok := sourceValue.Value.(string); ok && sourceStr != "" {
				if targetsValue, ok := values["targets"]; ok {
					if targets, ok := targetsValue.Value.([]string); ok {
						values["targets"] = schemaValueList(append([]string{sourceStr}, targets...))
						delete(values, "source")
					}
				}
			}
		}
	}

	if subValue, ok := values["subscriber"]; ok {
		subStr, ok := subValue.Value.(string)
		if ok && target.Host != "" {
			if subStr == target.Host || subStr == "" {
				values["subscriber"] = schemaValueAny(target.User + "@" + target.Host)
			}
		}
	}

	if _, ok := values["apikey"]; !ok {
		if projectValue, ok := values["project"]; ok {
			if projectStr, ok := projectValue.Value.(string); ok && projectStr != "" {
				values["apikey"] = schemaValueAny(projectStr)
			}
		}
	}
	if _, ok := values["project"]; !ok {
		if specHasMapTo(specs, "project") && target.Host != "" {
			values["project"] = schemaValueAny(target.Host)
		}
	}

	if needsApikeyFromTargets(specs, values) {
		if targetsValue, ok := values["targets"]; ok {
			if targets, ok := targetsValue.Value.([]string); ok && len(targets) > 0 {
				values["apikey"] = schemaValueAny(targets[0])
				values["targets"] = schemaValueList(append([]string{}, targets[1:]...))
			}
		}
	}

	if fpValue, ok := values["fullpath"]; ok {
		pathSpec, hasPath := specs.tokens["path"]
		if hasPath && specMapTo(pathSpec, "path") == "fullpath" {
			if tokenValue, ok := values["token"]; ok {
				fpStr, fpOk := fpValue.Value.(string)
				tokenStr, tokenOk := tokenValue.Value.(string)
				if fpOk && tokenOk && fpStr != "" && tokenStr != "" {
					if !strings.HasSuffix(fpStr, "/") && strings.HasSuffix(target.Path, "/"+tokenStr) {
						values["fullpath"] = schemaValueAny(fpStr + "/")
					}
				}
			}
		}
	}

	if needsEmailRebuild(specs, values) {
		email := strings.TrimSpace(target.Password)
		host := strings.TrimSpace(target.Host)
		if email != "" && host != "" && strings.Contains(host, ".") {
			values["email"] = schemaValueAny(email + "@" + host)
		}
	}

	baseURL := baseURLFromParsed(target)
	if baseURL != "" {
		if shouldSetBaseURL(specs, "url") {
			mapTo := specMapTo(specs.args["url"], "url")
			if _, ok := values[mapTo]; !ok {
				values[mapTo] = schemaValueAny(baseURL)
			}
		}
	}

	switch strings.ToLower(strings.TrimSpace(target.Scheme)) {
	case "mailto", "mailtos":
		if _, ok := values["from_addr"]; !ok {
			values["from_addr"] = schemaValueAny("")
		}
		if _, ok := values["smtp_host"]; !ok {
			values["smtp_host"] = schemaValueAny("")
		}
	case "gotify", "gotifys":
		if _, ok := values["fullpath"]; !ok {
			values["fullpath"] = schemaValueAny("/")
		}
	case "mmost", "mmosts":
		if _, ok := values["channels"]; !ok {
			values["channels"] = schemaValueList([]string{})
		}
	case "msteams":
		if _, ok := values["version"]; !ok {
			if spec, ok := specs.args["version"]; ok {
				if def, ok := specDefault(spec); ok {
					values["version"] = schemaValueInt(coerceInt(def))
				}
			}
		}
	case "napi", "notificationapi", "sendpulse":
		if _, ok := values["from_addr"]; !ok {
			values["from_addr"] = schemaValueAny(nil)
		}
	case "ses":
		if _, ok := target.Query["from"]; !ok {
			delete(values, "from_addr")
		}
	case "xbmc", "xbmcs":
		if portValue, ok := values["port"]; !ok || portValue.Value == nil {
			values["port"] = schemaValueInt(8080)
		}
	case "seven":
		if _, ok := values["label"]; !ok {
			values["label"] = schemaValueAny(nil)
		}
	case "sfr":
		if _, ok := values["sender"]; !ok {
			values["sender"] = schemaValueAny("")
		}
		if _, ok := values["timeout"]; !ok {
			values["timeout"] = schemaValueAny("")
		}
		if _, ok := values["lang"]; !ok {
			values["lang"] = schemaValueAny("")
		}
		if _, ok := values["media"]; !ok {
			values["media"] = schemaValueAny("")
		}
		if _, ok := values["voice"]; !ok {
			values["voice"] = schemaValueAny("")
		}
	case "revolt":
		if _, ok := values["link"]; !ok {
			if baseURL := baseURLFromParsed(target); baseURL != "" {
				values["link"] = schemaValueAny(baseURL)
			}
		}
	}
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
		if alias, ok := raw.(string); ok {
			return strings.TrimSpace(alias)
		}
		return ""
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
	raw, ok := spec["default"]
	if !ok || raw == nil {
		return nil, false
	}
	return raw, true
}

func specRequired(spec map[string]any) bool {
	if spec == nil {
		return false
	}
	raw, ok := spec["required"]
	if !ok || raw == nil {
		return false
	}
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		return parseBool(value, false)
	default:
		return parseBool(fmt.Sprint(value), false)
	}
}

func applySchemaAliasValue(values map[string]SchemaValue, specs schemaSpecs, alias string, raw string) {
	if alias == "" {
		return
	}
	if spec, ok := specs.args[alias]; ok {
		applySchemaValue(values, specMapTo(spec, alias), raw, spec, true)
		return
	}
	if spec, ok := specs.tokens[alias]; ok {
		applySchemaValue(values, specMapTo(spec, alias), raw, spec, false)
	}
}

var queryBoolRawMapTos = map[string]struct{}{
	"store": {},
}

var queryNumberMapTos = map[string]struct{}{
	"border":   {},
	"duration": {},
}

var queryListMapTos = map[string]struct{}{
	"txgroups": {},
}

func applySchemaValue(values map[string]SchemaValue, mapTo string, raw any, spec map[string]any, fromQuery bool) {
	if mapTo == "" {
		return
	}

	if isListType(spec) && raw == nil {
		return
	}
	if raw == nil {
		values[mapTo] = schemaValueAny(nil)
		return
	}

	if !fromQuery {
		if rawStr, ok := raw.(string); ok {
			if shouldApplyChoiceDefault(mapTo) && !valueAllowed(spec, rawStr) {
				if def, ok := specDefault(spec); ok {
					raw = def
					rawStr = coerceString(def)
				}
			}
			if (mapTo == "fullpath" || mapTo == "entity_id") && rawStr != "" && !strings.HasPrefix(rawStr, "/") {
				raw = "/" + rawStr
			}
		}
	}

	if isListType(spec) || (fromQuery && isQueryListMapTo(mapTo)) {
		list := coerceList(raw, spec)
		if existing, ok := values[mapTo]; ok {
			if existingList, ok := existing.Value.([]string); ok {
				values[mapTo] = schemaValueList(append(existingList, list...))
				return
			}
		}
		values[mapTo] = schemaValueList(list)
		return
	}

	if fromQuery && isQueryBoolRawMapTo(mapTo) {
		values[mapTo] = schemaValueAny(coerceString(raw))
		return
	}

	switch normalizeType(specType(spec)) {
	case "bool":
		values[mapTo] = schemaValueBool(coerceBool(raw))
	case "int":
		if fromQuery && !isQueryNumberMapTo(mapTo) {
			values[mapTo] = schemaValueAny(coerceString(raw))
		} else {
			values[mapTo] = schemaValueInt(coerceInt(raw))
		}
	case "float":
		if fromQuery && !isQueryNumberMapTo(mapTo) {
			values[mapTo] = schemaValueAny(coerceString(raw))
		} else {
			values[mapTo] = schemaValueFloat(coerceFloat(raw))
		}
	default:
		values[mapTo] = schemaValueAny(coerceString(raw))
	}
}

func isQueryBoolRawMapTo(mapTo string) bool {
	_, ok := queryBoolRawMapTos[mapTo]
	return ok
}

func isQueryNumberMapTo(mapTo string) bool {
	_, ok := queryNumberMapTos[mapTo]
	return ok
}

func isQueryListMapTo(mapTo string) bool {
	_, ok := queryListMapTos[mapTo]
	return ok
}

func valueAllowed(spec map[string]any, raw string) bool {
	if spec == nil {
		return true
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	values, ok := spec["values"]
	if !ok || values == nil {
		return true
	}
	switch typed := values.(type) {
	case []string:
		for _, entry := range typed {
			if strings.EqualFold(entry, raw) {
				return true
			}
		}
	case []any:
		for _, entry := range typed {
			if strings.EqualFold(fmt.Sprint(entry), raw) {
				return true
			}
		}
	default:
		if strings.EqualFold(fmt.Sprint(typed), raw) {
			return true
		}
	}
	return false
}

func shouldApplyChoiceDefault(mapTo string) bool {
	return strings.Contains(strings.ToLower(mapTo), "mode")
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
	bestScore := -1
	bestMissing := int(^uint(0) >> 1)
	var bestValues map[string]any
	for _, template := range templates {
		values, score, missing, ok := matchSchemaTemplate(template, specs, target)
		if !ok {
			continue
		}
		if score > bestScore || (score == bestScore && missing < bestMissing) {
			bestScore = score
			bestMissing = missing
			bestValues = values
		}
	}
	if bestValues != nil {
		return bestValues
	}
	return map[string]any{}
}

func applyTokenDefaults(specs map[string]map[string]any, templates []string, values map[string]any, target *ParsedURL) {
	for name, spec := range specs {
		mapTo := specMapTo(spec, name)
		if _, ok := values[name]; ok {
			continue
		}
		switch mapTo {
		case "schema":
			values[name] = target.Scheme
		case "host":
			if target.Host != "" {
				values[name] = target.Host
			} else {
				values[name] = nil
			}
		case "user":
			if target.HasUser {
				values[name] = target.User
			} else {
				values[name] = nil
			}
		case "password":
			if target.HasPassword {
				values[name] = target.Password
			} else {
				values[name] = nil
			}
		case "port":
			if target.HasPort {
				values[name] = target.Port
			} else {
				values[name] = nil
			}
		}
		if _, ok := values[name]; ok {
			continue
		}
		if tokenInTemplates(templates, name) {
			continue
		}
		if _, ok := specDefault(spec); ok {
			continue
		}
		if specRequired(spec) {
			continue
		}
		if isListType(spec) {
			continue
		}
		if mapTo == "targets" || mapTo == "channels" {
			continue
		}
		values[name] = nil
	}
}

func ensureListDefaults(specs schemaSpecs, values map[string]SchemaValue) {
	listDefaultMapTos := map[string]struct{}{
		"targets": {},
	}
	listMapTos := map[string]struct{}{}
	for name, spec := range specs.tokens {
		if !isListType(spec) {
			continue
		}
		mapTo := specMapTo(spec, name)
		if mapTo != "" {
			if _, ok := listDefaultMapTos[mapTo]; !ok {
				continue
			}
			listMapTos[mapTo] = struct{}{}
		}
	}
	for name, spec := range specs.args {
		if !isListType(spec) {
			continue
		}
		mapTo := specMapTo(spec, name)
		if mapTo != "" {
			if _, ok := listDefaultMapTos[mapTo]; !ok {
				continue
			}
			listMapTos[mapTo] = struct{}{}
		}
	}

	for mapTo := range listMapTos {
		existing, ok := values[mapTo]
		if !ok || existing.Value == nil {
			values[mapTo] = schemaValueList([]string{})
			continue
		}
		switch typed := existing.Value.(type) {
		case []string:
			// already list
		case []any:
			out := make([]string, 0, len(typed))
			for _, item := range typed {
				if item == nil {
					continue
				}
				out = append(out, fmt.Sprint(item))
			}
			values[mapTo] = schemaValueList(out)
		case string:
			if typed == "" {
				values[mapTo] = schemaValueList([]string{})
			} else {
				values[mapTo] = schemaValueList([]string{typed})
			}
		default:
			values[mapTo] = schemaValueList([]string{fmt.Sprint(typed)})
		}
	}
}

func ensureEmptyKwargs(specs schemaSpecs, kwargs map[string]map[string]string) {
	defaultMapTos := map[string]struct{}{
		"custom":           {},
		"data_kwargs":      {},
		"details":          {},
		"headers":          {},
		"mapping":          {},
		"meta_extras":      {},
		"params":           {},
		"payload":          {},
		"payload_extras":   {},
		"postback":         {},
		"template_data":    {},
		"template_mapping": {},
		"tokens":           {},
	}
	for name, spec := range specs.kwargs {
		mapTo := specMapTo(spec, name)
		if mapTo == "" {
			continue
		}
		if _, ok := defaultMapTos[mapTo]; !ok {
			continue
		}
		if _, ok := kwargs[mapTo]; !ok {
			kwargs[mapTo] = map[string]string{}
		}
	}
}

type segmentPart struct {
	isToken bool
	value   string
}

func matchSchemaTemplate(template string, specs map[string]map[string]any, target *ParsedURL) (map[string]any, int, int, bool) {
	parts := strings.SplitN(template, "://", 2)
	if len(parts) != 2 {
		return nil, 0, 0, false
	}

	schemeTemplate := parts[0]
	rest := parts[1]

	values := map[string]any{}
	score := 0
	missing := 0

	if token, ok := exactToken(schemeTemplate); ok {
		values[token] = target.Scheme
		if target.Scheme != "" {
			score++
		}
	} else if !strings.EqualFold(schemeTemplate, target.Scheme) {
		return nil, 0, 0, false
	}

	authority := rest
	pathTemplate := ""
	queryTemplate := ""
	if idx := strings.Index(rest, "?"); idx != -1 {
		authority = rest[:idx]
		queryTemplate = rest[idx+1:]
	}
	if idx := strings.Index(authority, "/"); idx != -1 {
		pathTemplate = authority[idx+1:]
		authority = authority[:idx]
	}

	authorityScore, authorityMissing, ok := matchTemplateAuthority(authority, specs, target, values)
	if !ok {
		return nil, 0, 0, false
	}

	pathScore, pathMissing, ok := matchTemplatePath(pathTemplate, specs, target, values)
	if !ok {
		return nil, 0, 0, false
	}

	queryScore, queryMissing, ok := matchTemplateQuery(queryTemplate, specs, target, values)
	if !ok {
		return nil, 0, 0, false
	}

	score += authorityScore + pathScore + queryScore
	missing += authorityMissing + pathMissing + queryMissing
	return values, score, missing, true
}

func matchTemplateAuthority(template string, specs map[string]map[string]any, target *ParsedURL, values map[string]any) (int, int, bool) {
	score := 0
	missing := 0
	userinfoTemplate := ""
	hostTemplate := template
	if idx := strings.LastIndex(template, "@"); idx != -1 {
		userinfoTemplate = template[:idx]
		hostTemplate = template[idx+1:]
	}

	if userinfoTemplate == "" && target.HasUser && strings.Contains(hostTemplate, ":") {
		hostPart := hostTemplate
		portTemplate := ""
		if idx := strings.LastIndex(hostTemplate, ":"); idx != -1 {
			hostPart = hostTemplate[:idx]
			portTemplate = hostTemplate[idx+1:]
		}
		if hostToken, ok := exactToken(hostPart); ok {
			if portToken, ok := exactToken(portTemplate); ok {
				hostMapTo := specMapTo(specs[hostToken], hostToken)
				portMapTo := specMapTo(specs[portToken], portToken)
				if hostMapTo != "host" && portMapTo != "port" {
					values[hostToken] = target.User
					if target.User != "" {
						score++
					}
					portValue := target.Password
					if wantsEmailValue(specs[portToken], portToken, portMapTo) {
						if shouldSetEmailToken(portToken, portMapTo) && target.Host != "" {
							portValue = portValue + "@" + target.Host
						} else {
							return score, missing, true
						}
					}
					if portValue != "" {
						values[portToken] = portValue
						score++
					}
					return score, missing, true
				}
			}
		}
	}

	if userinfoTemplate != "" {
		userTemplate := userinfoTemplate
		passTemplate := ""
		if idx := strings.Index(userinfoTemplate, ":"); idx != -1 {
			userTemplate = userinfoTemplate[:idx]
			passTemplate = userinfoTemplate[idx+1:]
		}

		if token, ok := exactToken(userTemplate); ok {
			if target.HasUser {
				values[token] = target.User
				if target.User != "" {
					score++
				}
				if !tokenValueMatches(specs[token], values[token]) {
					return 0, 0, false
				}
			} else {
				missing++
				mapTo := specMapTo(specs[token], token)
				if wantsEmptyOnMissingUserinfo(mapTo, token) {
					values[token] = ""
				}
			}
		} else if userTemplate != "" && userTemplate != target.User {
			return 0, 0, false
		} else if userTemplate != "" {
			score++
		}

		if passTemplate != "" {
			if token, ok := exactToken(passTemplate); ok {
				if target.HasPassword {
					values[token] = target.Password
					if target.Password != "" {
						score++
					}
					if !tokenValueMatches(specs[token], values[token]) {
						return 0, 0, false
					}
				} else {
					missing++
					mapTo := specMapTo(specs[token], token)
					if wantsEmptyOnMissingUserinfo(mapTo, token) {
						values[token] = ""
					}
				}
			} else if passTemplate != target.Password {
				return 0, 0, false
			} else {
				score++
			}
		}
	}

	if hostTemplate == "" {
		return score, missing, true
	}

	portTemplate := ""
	hostPart := hostTemplate
	if idx := strings.LastIndex(hostTemplate, ":"); idx != -1 {
		hostPart = hostTemplate[:idx]
		portTemplate = hostTemplate[idx+1:]
	}

	if token, ok := exactToken(hostPart); ok {
		if target.Host != "" {
			values[token] = target.Host
			score++
			if !tokenValueMatches(specs[token], values[token]) {
				return 0, 0, false
			}
		} else {
			missing++
		}
	} else if hostPart != "" && !strings.EqualFold(hostPart, target.Host) {
		return 0, 0, false
	} else if hostPart != "" {
		score++
	}

	if portTemplate != "" {
		if token, ok := exactToken(portTemplate); ok {
			if target.HasPort {
				portValue := strconv.Itoa(target.Port)
				values[token] = portValue
				score++
				if !tokenValueMatches(specs[token], values[token]) {
					return 0, 0, false
				}
			} else {
				missing++
			}
		} else if !target.HasPort {
			return 0, 0, false
		} else if portTemplate != strconv.Itoa(target.Port) {
			return 0, 0, false
		} else {
			score++
		}
	}

	return score, missing, true
}

func matchTemplatePath(template string, specs map[string]map[string]any, target *ParsedURL, values map[string]any) (int, int, bool) {
	segments := splitPathSegmentsLocal(target.Path)
	if template == "" {
		return 0, 0, true
	}

	patternSegments := splitTemplateSegments(template)
	if len(patternSegments) == 0 {
		return 0, 0, true
	}

	score := 0
	missing := 0
	idx := 0
	for i, pattern := range patternSegments {
		if idx > len(segments) {
			return score, missing, false
		}
		if len(pattern) == 1 && pattern[0].isToken {
			token := pattern[0].value
			spec := specs[token]
			if isListType(spec) && (i == len(patternSegments)-1 || delimContains(spec, "/")) {
				remaining := []string{}
				if idx < len(segments) {
					remaining = append([]string(nil), segments[idx:]...)
					score += len(remaining)
				}
				values[token] = remaining
				idx = len(segments)
				continue
			}
			if idx >= len(segments) {
				missing++
				continue
			}
			segment := segments[idx]
			if !tokenValueMatches(spec, segment) {
				return score, missing, false
			}
			values[token] = segment
			if segment != "" {
				score++
			}
			idx++
			continue
		}

		if idx >= len(segments) {
			return score, missing, false
		}
		segment := segments[idx]
		matched, ok := matchComplexSegment(pattern, segment, specs)
		if !ok {
			return score, missing, false
		}
		for key, value := range matched {
			values[key] = value
			if value != "" {
				score++
			}
		}
		idx++
	}

	if idx != len(segments) {
		return score, missing, false
	}
	return score, missing, true
}

func matchTemplateQuery(template string, specs map[string]map[string]any, target *ParsedURL, values map[string]any) (int, int, bool) {
	if template == "" {
		return 0, 0, true
	}
	score := 0
	missing := 0
	pairs := strings.FieldsFunc(template, func(r rune) bool {
		return r == '&' || r == ';'
	})
	for _, pair := range pairs {
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		key := strings.TrimSpace(parts[0])
		val := ""
		if len(parts) == 2 {
			val = strings.TrimSpace(parts[1])
		}
		if key == "" {
			continue
		}
		queryKey := strings.ToLower(key)
		if token, ok := exactToken(val); ok {
			if raw, ok := target.Query[queryKey]; ok {
				values[token] = raw
				if raw != "" {
					score++
				}
			} else {
				missing++
			}
			continue
		}
		if raw, ok := target.Query[queryKey]; ok {
			if val != "" && raw != val {
				return 0, 0, false
			}
			if raw != "" {
				score++
			}
		} else if val != "" {
			missing++
		}
	}
	return score, missing, true
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

func tokenValueMatches(spec map[string]any, value any) bool {
	return true
}

func wantsEmailValue(spec map[string]any, token, mapTo string) bool {
	lowerToken := strings.ToLower(strings.TrimSpace(token))
	lowerMap := strings.ToLower(strings.TrimSpace(mapTo))
	if strings.Contains(lowerToken, "email") || strings.Contains(lowerMap, "email") {
		return true
	}
	if strings.Contains(lowerToken, "addr") || strings.Contains(lowerMap, "addr") {
		return true
	}
	if spec != nil {
		if raw, ok := spec["regex"]; ok && raw != nil {
			switch typed := raw.(type) {
			case []string:
				if len(typed) > 0 && strings.Contains(typed[0], "@") {
					return true
				}
			case []any:
				if len(typed) > 0 && strings.Contains(fmt.Sprint(typed[0]), "@") {
					return true
				}
			case string:
				if strings.Contains(typed, "@") {
					return true
				}
			}
		}
	}
	return false
}

func wantsEmptyOnMissingUserinfo(mapTo, token string) bool {
	lower := strings.ToLower(strings.TrimSpace(mapTo))
	lowerToken := strings.ToLower(strings.TrimSpace(token))
	if lower == "user" || lower == "password" {
		return false
	}
	if strings.Contains(lower, "key") || strings.Contains(lower, "secret") || strings.Contains(lower, "user") ||
		strings.Contains(lower, "pass") || strings.Contains(lower, "auth") || strings.Contains(lower, "account") ||
		strings.Contains(lowerToken, "key") || strings.Contains(lowerToken, "secret") || strings.Contains(lowerToken, "user") ||
		strings.Contains(lowerToken, "pass") || strings.Contains(lowerToken, "auth") || strings.Contains(lowerToken, "account") {
		return true
	}
	return false
}

func shouldSetEmailToken(token, mapTo string) bool {
	lower := strings.ToLower(strings.TrimSpace(mapTo))
	lowerToken := strings.ToLower(strings.TrimSpace(token))
	if strings.Contains(lower, "email") && strings.Contains(lower, "from") {
		return true
	}
	if strings.Contains(lowerToken, "email") && strings.Contains(lowerToken, "from") {
		return true
	}
	if strings.Contains(lower, "from") || strings.Contains(lowerToken, "from") {
		return true
	}
	if strings.Contains(lower, "addr") || strings.Contains(lowerToken, "addr") {
		return true
	}
	return false
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

func tokenInTemplates(templates []string, token string) bool {
	if token == "" {
		return false
	}
	needle := "{" + token + "}"
	for _, template := range templates {
		if strings.Contains(template, needle) {
			return true
		}
	}
	return false
}

func needsApikeyFromTargets(specs schemaSpecs, values map[string]SchemaValue) bool {
	apikeyValue, ok := values["apikey"]
	if ok && apikeyValue.Value != nil {
		if str, ok := apikeyValue.Value.(string); ok && str != "" {
			return false
		}
		if _, ok := apikeyValue.Value.(string); !ok {
			return false
		}
	}
	if !specHasMapTo(specs, "apikey") {
		return false
	}
	if !templateHasAuthorityPortToken(specs.templates, "apikey") {
		return false
	}
	return true
}

func needsEmailRebuild(specs schemaSpecs, values map[string]SchemaValue) bool {
	if !specHasToken(specs.tokens, "email") {
		return false
	}
	if !specHasToken(specs.tokens, "password") {
		return false
	}
	if !specHasToken(specs.tokens, "from_phone") && !specHasMapToTokens(specs.tokens, "source") {
		return false
	}
	if emailValue, ok := values["email"]; ok {
		if emailStr, ok := emailValue.Value.(string); ok {
			return !strings.Contains(emailStr, "@")
		}
		return true
	}
	return true
}

func specHasToken(specs map[string]map[string]any, name string) bool {
	_, ok := specs[name]
	return ok
}

func specHasMapTo(specs schemaSpecs, mapTo string) bool {
	for name, spec := range specs.tokens {
		if specMapTo(spec, name) == mapTo {
			return true
		}
	}
	for name, spec := range specs.args {
		if specMapTo(spec, name) == mapTo {
			return true
		}
	}
	return false
}

func specHasMapToTokens(specs map[string]map[string]any, mapTo string) bool {
	for name, spec := range specs {
		if specMapTo(spec, name) == mapTo {
			return true
		}
	}
	return false
}

func templateHasAuthorityPortToken(templates []string, token string) bool {
	needle := "{" + token + "}"
	for _, template := range templates {
		parts := strings.SplitN(template, "://", 2)
		if len(parts) != 2 {
			continue
		}
		rest := parts[1]
		authority := rest
		if idx := strings.Index(rest, "/"); idx != -1 {
			authority = rest[:idx]
		}
		if idx := strings.LastIndex(authority, ":"); idx != -1 {
			portPart := authority[idx+1:]
			if strings.Contains(portPart, needle) {
				return true
			}
		}
	}
	return false
}

func baseURLFromParsed(target *ParsedURL) string {
	if target == nil || target.Scheme == "" {
		return ""
	}
	var builder strings.Builder
	builder.WriteString(target.Scheme)
	builder.WriteString("://")
	if target.HasUser {
		builder.WriteString(target.User)
		if target.HasPassword {
			builder.WriteString(":")
			builder.WriteString(target.Password)
		}
		builder.WriteString("@")
	}
	builder.WriteString(target.Host)
	if target.HasPort {
		builder.WriteString(":")
		builder.WriteString(strconv.Itoa(target.Port))
	}
	if target.Path != "" {
		builder.WriteString(target.Path)
	}
	return builder.String()
}

func shouldSetBaseURL(specs schemaSpecs, argName string) bool {
	if argName == "" {
		return false
	}
	spec, ok := specs.args[argName]
	if !ok {
		return false
	}
	if specAlias(spec) != "" {
		return false
	}
	mapTo := specMapTo(spec, argName)
	return mapTo != ""
}

func delimContains(spec map[string]any, delim string) bool {
	for _, entry := range specDelims(spec) {
		if entry == delim {
			return true
		}
	}
	return false
}
