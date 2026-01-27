import json
import re
import sys
import urllib.parse

from apprise.plugins import N_MGR
from apprise.plugins import details as plugin_details

CANDIDATES = [
    "token",
    "TOKEN",
    "12345",
    "12345678",
    "1234567890",
    "abcdef",
    "ABCDEF",
    "user",
    "user@example.com",
    "https://example.com/hook",
    "xoxb-12345-ABCDE",
    "us-west-2",
    "dG9rZW4=",
    "123e4567-e89b-12d3-a456-426614174000",
    "+15555550123",
    "15555550123",
    "topic",
    "channel",
    "ABC@DEF",
    "user@localhost",
]


def is_simple_template(template):
    if "://" not in template:
        return False
    _, rest = template.split("://", 1)
    authority, path = rest, ""
    if "/" in rest:
        authority, path = rest.split("/", 1)

    userinfo = ""
    hostpart = authority
    if "@" in authority:
        userinfo, hostpart = authority.rsplit("@", 1)

    if userinfo:
        if ":" in userinfo:
            user, pwd = userinfo.split(":", 1)
            if token_name(user) is None or token_name(pwd) is None:
                return False
        else:
            if token_name(userinfo) is None:
                return False

    if hostpart:
        if ":" in hostpart:
            host, port = hostpart.split(":", 1)
            if token_name(host) is None or token_name(port) is None:
                return False
        else:
            if "{" in hostpart or "}" in hostpart:
                if token_name(hostpart) is None:
                    return False

    if path:
        for segment in path.split("/"):
            if not segment:
                continue
            if "{" in segment or "}" in segment:
                if token_name(segment) is None:
                    return False

    return True


def token_name(segment):
    if (
        segment.startswith("{")
        and segment.endswith("}")
        and segment.count("{") == 1
        and segment.count("}") == 1
    ):
        return segment[1:-1]
    return None


def sample_for_name(name):
    lowered = name.lower()
    if lowered in ("tz", "timezone"):
        return "UTC"
    if "subscriber" in lowered:
        return "user@example.com"
    if lowered in ("host", "hostname", "domain"):
        return "example.com"
    if "host" in lowered:
        return "example.com"
    if lowered in ("port", "host_port"):
        return "443"
    if "port" in lowered:
        return "443"
    if "email" in lowered:
        return "user@example.com"
    if "from" in lowered and ("addr" in lowered or "email" in lowered):
        return "user@example.com"
    if "reply" in lowered:
        return "user@example.com"
    if "url" in lowered:
        return "https://example.com/hook"
    if "webhook" in lowered:
        return "token"
    if "region" in lowered:
        return "us-west-2"
    if "uuid" in lowered:
        return "123e4567-e89b-12d3-a456-426614174000"
    if "phone" in lowered:
        return "15555550123"
    if (
        "token" in lowered
        or "secret" in lowered
        or "apikey" in lowered
        or "api_key" in lowered
        or "access" in lowered
    ):
        return "token"
    if "user" in lowered or "account" in lowered or "login" in lowered:
        return "user"
    if "pass" in lowered or "pwd" in lowered:
        return "pass"
    if "channel" in lowered:
        return "channel"
    if "topic" in lowered:
        return "topic"
    if "room" in lowered:
        return "room"
    if "group" in lowered:
        return "group"
    return "token"


def sample_for_label(label):
    lowered = label.lower()
    if "timezone" in lowered:
        return "UTC"
    if "channel id" in lowered or ("channel" in lowered and "id" in lowered):
        return "123"
    if "email" in lowered:
        return "user@example.com"
    if "reply" in lowered:
        return "user@example.com"
    if "phone" in lowered or "sms" in lowered:
        return "15555550123"
    if "callsign" in lowered or "call sign" in lowered:
        return "AA1AA"
    if "url" in lowered:
        return "https://example.com/hook"
    if "webhook" in lowered:
        return "token"
    if "region" in lowered:
        return "us-west-2"
    if "token" in lowered or "secret" in lowered or "key" in lowered:
        return "token"
    if "channel" in lowered:
        return "channel"
    if "topic" in lowered:
        return "topic"
    if "room" in lowered:
        return "room"
    if "group" in lowered:
        return "group"
    return None


def match_regex(regex, flags, candidate):
    try:
        compiled = re.compile(regex, flags)
    except re.error:
        return False
    return compiled.fullmatch(candidate) is not None


def sample_for_regex(regex, flags):
    if "@@@" in regex:
        return "user@example.com"

    if "@" in regex:
        for candidate in ("ABC@DEF", "user@localhost", "user@example.com"):
            if match_regex(regex, flags, candidate):
                return candidate

    if regex.lstrip("^").startswith("V2"):
        candidate = "V2ABC123"
        if match_regex(regex, flags, candidate):
            return candidate

    if "bot" in regex.lower() and ":" in regex:
        for candidate in ("bot12345:abcdef", "12345:abcdef"):
            if match_regex(regex, flags, candidate):
                return candidate

    if "xox" in regex.lower():
        candidate = "xoxb-12345-ABCDE"
        if match_regex(regex, flags, candidate):
            return candidate

    if "at_" in regex.lower():
        candidate = "AT_token"
        if match_regex(regex, flags, candidate):
            return candidate

    if "uid_" in regex.lower():
        candidate = "UID_token"
        if match_regex(regex, flags, candidate):
            return candidate

    for candidate in CANDIDATES:
        if match_regex(regex, flags, candidate):
            return candidate

    length_match = re.search(r"\[([^\]]+)\]\{(\d+)(?:,(\d+))?\}", regex)
    if length_match:
        charset = length_match.group(1)
        size = int(length_match.group(2))
        sample_char = "a"
        if "0-9" in charset and "A-Z" in charset:
            sample_char = "A"
        elif "0-9" in charset:
            sample_char = "1"
        elif "A-Z" in charset:
            sample_char = "A"
        elif "a-z" in charset:
            sample_char = "a"
        return sample_char * size

    range_match = re.search(r"\{(\d+)(?:,(\d+))?\}", regex)
    class_match = re.search(r"\[([^\]]+)\]", regex)
    if range_match and class_match:
        size = int(range_match.group(1))
        charset = class_match.group(1)
        sample_char = "a"
        if "0-9" in charset and "A-Z" in charset:
            sample_char = "A"
        elif "0-9" in charset:
            sample_char = "1"
        elif "A-Z" in charset:
            sample_char = "A"
        elif "a-z" in charset:
            sample_char = "a"
        return sample_char * size

    return "token"


def sample_for_spec(name, spec):
    value = sample_for_name(name)
    if isinstance(spec, dict):
        label = str(spec.get("name") or "")
        suggested = sample_for_label(label)
        if suggested and "subscriber" not in name.lower():
            value = suggested
    regex = None
    flags = 0
    raw_regex = spec.get("regex") if isinstance(spec, dict) else None
    if isinstance(raw_regex, (list, tuple)) and raw_regex:
        regex = raw_regex[0]
        if len(raw_regex) > 1 and isinstance(raw_regex[1], str):
            if "i" in raw_regex[1]:
                flags |= re.IGNORECASE

    if regex:
        if not match_regex(regex, flags, value):
            value = sample_for_regex(regex, flags)

    prefix = spec.get("prefix") if isinstance(spec, dict) else None
    if prefix and value and not value.startswith(prefix):
        if prefix in ("#", "?"):
            return value
        return prefix + value

    return value


def sample_for_token(name, spec, tokens):
    if isinstance(spec, dict) and spec.get("group"):
        group = spec.get("group")
        if isinstance(group, (list, tuple, set)) and group:
            group_list = [str(item) for item in group]
            candidate = ""
            for entry in group_list:
                if "id" in entry.lower():
                    candidate = entry
                    break
            if candidate == "" and group_list:
                candidate = group_list[0]
            group_spec = tokens.get(candidate) if isinstance(tokens, dict) else None
            if isinstance(group_spec, dict):
                return sample_for_spec(candidate, group_spec)
            name = candidate
    return sample_for_spec(name, spec)


def fill_template(template, schema, tokens):
    values = {"schema": schema}
    for name, spec in tokens.items():
        if name == "schema":
            continue
        values[name] = sample_for_token(name, spec, tokens)

    url = template
    for name, value in values.items():
        url = url.replace("{" + name + "}", value)

    if "{" in url or "}" in url:
        return None

    return url


def template_token_count(template):
    if not isinstance(template, str):
        return 0
    return len(re.findall(r"{[^{}]+}", template))


def query_value_from_default(default, spec):
    if default is None:
        return None
    if isinstance(default, bool):
        return "yes" if default else "no"
    if isinstance(default, (int, float)):
        return str(default)
    if isinstance(default, (list, tuple)):
        if not default:
            return None
        delim = ","
        raw_delim = spec.get("delim") if isinstance(spec, dict) else None
        if isinstance(raw_delim, (list, tuple)) and raw_delim:
            delim = raw_delim[0]
        elif isinstance(raw_delim, str) and raw_delim:
            delim = raw_delim
        return delim.join(str(item) for item in default)
    if isinstance(default, str):
        return default if default else None
    return str(default)


def sample_arg_value(name, spec):
    arg_type = str(spec.get("type") or "").lower()
    if name.lower() in ("tz", "timezone") or "timezone" in arg_type:
        return "UTC"
    default = spec.get("default", None)
    if default is not None:
        return query_value_from_default(default, spec)
    if arg_type.startswith("choice:"):
        values = spec.get("values")
        if isinstance(values, (list, tuple)) and values:
            return str(values[0])
    if arg_type.startswith("bool"):
        return "yes"
    if arg_type.startswith("int"):
        return "1"
    if arg_type.startswith("float"):
        return "1"
    if arg_type.startswith("list"):
        return "a,b"
    return sample_for_spec(name, spec)


def encode_query(items):
    parts = []
    for key, value in items:
        if value is None:
            continue
        key = str(key)
        prefix = ""
        rest = key
        if key and key[0] in "+-:":
            prefix = key[0]
            rest = key[1:]
        encoded_key = prefix + urllib.parse.quote(rest, safe="")
        encoded_val = urllib.parse.quote(str(value), safe="")
        parts.append(encoded_key + "=" + encoded_val)
    return "&".join(parts)


def append_query(url, items):
    if not items:
        return url
    query = encode_query(items)
    if not query:
        return url
    sep = "&" if "?" in url else "?"
    return url + sep + query


def generate_cases(schema, plugin, details):
    cases = []
    templates = details.get("templates") or []
    tokens = details.get("tokens") or {}
    args = details.get("args") or {}
    kwargs = details.get("kwargs") or {}

    ordered_templates = sorted(
        (t for t in templates if isinstance(t, str)),
        key=lambda value: (is_simple_template(value), template_token_count(value)),
        reverse=True,
    )

    def can_parse(url):
        try:
            return bool(plugin.parse_url(url))
        except Exception:
            return False

    valid_templates = []
    for template in ordered_templates:
        url = fill_template(template, schema, tokens)
        if not url:
            continue
        if can_parse(url):
            valid_templates.append(url)
            cases.append({"name": f"template-{len(cases) + 1}", "url": url})

    primary = valid_templates[0] if valid_templates else None

    if primary:
        default_items = []
        for name, spec in args.items():
            if not isinstance(spec, dict) or "alias_of" in spec:
                continue
            value = query_value_from_default(spec.get("default", None), spec)
            if value is None:
                continue
            default_items.append((name, value))
        if default_items:
            url = append_query(primary, default_items)
            if can_parse(url):
                cases.append({"name": "defaults", "url": url})

        for name, spec in args.items():
            if not isinstance(spec, dict) or "alias_of" in spec:
                continue
            arg_type = str(spec.get("type") or "").lower()
            if arg_type.startswith("choice:"):
                values = spec.get("values") or []
                if isinstance(values, (list, tuple)):
                    for value in values:
                        url = append_query(primary, [(name, value)])
                        if can_parse(url):
                            cases.append({"name": f"choice-{name}-{value}", "url": url})
                continue
            if arg_type.startswith("bool"):
                for value in ("yes", "no"):
                    url = append_query(primary, [(name, value)])
                    if can_parse(url):
                        cases.append({"name": f"bool-{name}-{value}", "url": url})
                continue

            value = sample_arg_value(name, spec)
            if value is None:
                continue
            url = append_query(primary, [(name, value)])
            if can_parse(url):
                cases.append({"name": f"arg-{name}", "url": url})

        for name, spec in kwargs.items():
            if not isinstance(spec, dict) or "alias_of" in spec:
                continue
            prefix = spec.get("prefix")
            if prefix not in ("+", "-", ":"):
                prefix = ""
            key = f"{prefix}key"
            url = append_query(primary, [(key, "value")])
            if can_parse(url):
                cases.append({"name": f"kwargs-{name}", "url": url})

    if not cases:
        for fallback in (
            f"{schema}://example.com",
            f"{schema}://token",
            f"{schema}://user:pass@example.com",
            f"{schema}://user:pass@example.com/target",
        ):
            if can_parse(fallback):
                cases.append({"name": "fallback", "url": fallback})
                break

    return cases


def main():
    requested = set(s.strip().lower() for s in sys.argv[1:] if s.strip())
    schemas = list(N_MGR.schemas()) if hasattr(N_MGR, "schemas") else list(N_MGR)
    cases = []
    failures = []

    for schema in sorted(schemas):
        if requested and schema.lower() not in requested:
            continue
        plugin = N_MGR[schema]
        details = plugin_details(plugin) or {}
        generated = generate_cases(schema, plugin, details)
        if not generated:
            failures.append(schema)
            continue
        for case in generated:
            cases.append(
                {
                    "schema": schema.lower(),
                    "name": case["name"],
                    "url": case["url"],
                }
            )

    if failures:
        print(
            "Failed to build exercise URLs for schemas: " + ", ".join(failures),
            file=sys.stderr,
        )
        raise SystemExit(1)

    print(json.dumps(cases, ensure_ascii=True, sort_keys=True))


if __name__ == "__main__":
    main()
