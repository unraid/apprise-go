import json
import re
import sys

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
    if segment.startswith("{") and segment.endswith("}") and segment.count("{") == 1 and segment.count("}") == 1:
        return segment[1:-1]
    return None


def sample_for_name(name):
    lowered = name.lower()
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
    if "url" in lowered or "webhook" in lowered:
        return "https://example.com/hook"
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


def match_regex(regex, flags, candidate):
    try:
        compiled = re.compile(regex, flags)
    except re.error:
        return False
    return compiled.fullmatch(candidate) is not None


def sample_for_regex(regex, flags):
    for candidate in CANDIDATES:
        if match_regex(regex, flags, candidate):
            return candidate

    length_match = re.match(r"^\\A?\\?\[([^\]]+)\]\\?\{(\d+)(?:,(\d+))?\}\\?\$?", regex)
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

    return "token"


def sample_for_token(name, spec):
    value = sample_for_name(name)
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
        # Avoid fragment markers in URL generation.
        if prefix in ("#", "?"):
            return value
        return prefix + value

    return value


def fill_template(template, schema, tokens):
    values = {"schema": schema}
    for name, spec in tokens.items():
        values[name] = sample_for_token(name, spec)

    url = template
    for name, value in values.items():
        url = url.replace("{" + name + "}", value)

    if "{" in url or "}" in url:
        return None

    return url


def generate_url(schema, plugin, info):
    details = info.get("details", {}) if isinstance(info, dict) else {}
    templates = details.get("templates") or []
    tokens = details.get("tokens") or {}

    for template in templates:
        if not isinstance(template, str):
            continue
        if not is_simple_template(template):
            continue
        url = fill_template(template, schema, tokens)
        if not url:
            continue
        results = plugin.parse_url(url)
        if results:
            return url

    for fallback in (
        f"{schema}://example.com",
        f"{schema}://token",
        f"{schema}://user:pass@example.com",
        f"{schema}://user:pass@example.com/target",
    ):
        results = plugin.parse_url(fallback)
        if results:
            return fallback

    return None


def main():
    failures = []
    cases = []

    for schema in sorted(N_MGR.keys()):
        plugin = N_MGR[schema]
        info = plugin_details(plugin)
        url = generate_url(schema, plugin, info)
        if not url:
            failures.append(schema)
            continue
        cases.append({"schema": schema, "url": url})

    if failures:
        print(
            "Failed to build URLs for schemas: " + ", ".join(failures), file=sys.stderr
        )
        raise SystemExit(1)

    print(json.dumps(cases, ensure_ascii=True, sort_keys=True))


if __name__ == "__main__":
    main()
