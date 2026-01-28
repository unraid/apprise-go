import hashlib
import json
import os
import re
import subprocess
import sys
import urllib.parse
from pathlib import Path

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
    "123e4567-e89b-42d3-a456-426614174000",
    "+15555550123",
    "15555550123",
    "topic",
    "channel",
    "ABC@DEF",
    "user@localhost",
]

MATRIX_T2BOT_TOKEN = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

ROOT_DIR = Path(__file__).resolve().parents[3]
FIXTURES_DIR = ROOT_DIR / "internal" / "testutil" / "fixtures"

CACHE_VERSION = 5
CACHE_ENV = "APPRISE_CASES_CACHE"
CACHE_DIR_ENV = "APPRISE_CASES_CACHE_DIR"
CACHE_SUBDIR = ".tmp/pycases"


def cache_enabled():
    value = os.environ.get(CACHE_ENV, "").strip().lower()
    if value in {"0", "false", "no", "off"}:
        return False
    return True


def find_repo_root(start):
    current = start
    while True:
        if (current / "go.mod").exists() and (current / "internal").is_dir():
            return current
        parent = current.parent
        if parent == current:
            return None
        current = parent


def cache_dir():
    explicit = os.environ.get(CACHE_DIR_ENV, "").strip()
    if explicit:
        return Path(explicit)
    here = Path(__file__).resolve()
    root = find_repo_root(here)
    if root is None:
        return Path.cwd() / CACHE_SUBDIR
    return root / CACHE_SUBDIR


def apprise_repo_root():
    try:
        import apprise as apprise_module

        path = Path(apprise_module.__file__).resolve()
    except Exception:
        return None

    current = path.parent
    while True:
        if (current / ".git").exists():
            return current
        if (current / "pyproject.toml").exists() and (current / "apprise").is_dir():
            return current
        parent = current.parent
        if parent == current:
            return None
        current = parent


def apprise_git_sha():
    root = apprise_repo_root()
    if root is None:
        return ""
    try:
        output = subprocess.check_output(
            ["git", "-C", str(root), "rev-parse", "HEAD"],
            stderr=subprocess.DEVNULL,
        )
    except Exception:
        return ""
    return output.decode("utf-8", "replace").strip()


def cache_key(requested):
    try:
        import apprise as apprise_module

        apprise_version = getattr(apprise_module, "__version__", "")
    except Exception:
        apprise_version = ""
    payload = {
        "version": CACHE_VERSION,
        "script": "schema_exercise_cases",
        "requested": sorted(requested),
        "apprise_version": apprise_version,
        "apprise_sha": apprise_git_sha(),
        "python_version": sys.version,
    }
    digest = hashlib.sha256(
        json.dumps(payload, sort_keys=True, separators=(",", ":")).encode("utf-8")
    ).hexdigest()
    return digest


def load_cache(requested):
    if not cache_enabled():
        return None, None
    digest = cache_key(requested)
    root = cache_dir()
    path = root / f"{digest}.json"
    if not path.exists():
        return None, path
    try:
        return json.loads(path.read_text(encoding="utf-8")), path
    except Exception:
        return None, path


def store_cache(path, cases):
    if path is None:
        return
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        tmp = path.with_suffix(path.suffix + f".tmp.{os.getpid()}")
        tmp.write_text(json.dumps(cases, separators=(",", ":")), encoding="utf-8")
        os.replace(tmp, path)
    except Exception:
        return


def fixture_path(name):
    return str(FIXTURES_DIR / name)


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
    if "gateway" in lowered or "threema" in lowered:
        return "ABCDEFGH"
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
    if "keyfile" in lowered:
        return fixture_path("vapid_test_key.pem")
    if "subfile" in lowered:
        return fixture_path("vapid_test_sub.json")
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


def targets_required(tokens):
    if not isinstance(tokens, dict):
        return False
    spec = tokens.get("targets")
    return isinstance(spec, dict) and bool(spec.get("required"))


def template_has_targets(template, tokens):
    if "{targets}" in template:
        return True
    if not isinstance(tokens, dict):
        return False
    for name, spec in tokens.items():
        if not isinstance(spec, dict):
            continue
        if spec.get("map_to") == "targets" and f"{{{name}}}" in template:
            return True
    return False


def target_alias(args):
    if not isinstance(args, dict):
        return None
    for name, spec in args.items():
        if isinstance(spec, dict) and spec.get("alias_of") == "targets":
            return name
    for name, spec in args.items():
        if isinstance(spec, dict) and spec.get("map_to") == "targets":
            return name
    return None


def ensure_targets(url, template, tokens, args):
    if template_has_targets(template, tokens):
        return url
    alias = target_alias(args)
    if alias is None:
        if not targets_required(tokens):
            return url
        alias = "targets"
    targets_spec = tokens.get("targets") if isinstance(tokens, dict) else None
    value = sample_for_token("targets", targets_spec, tokens)
    return ensure_query_params(url, [(alias, value)])


def normalize_mailgun_url(url):
    parsed = urllib.parse.urlsplit(url)
    if parsed.scheme.lower() != "mailgun":
        return url
    netloc = parsed.netloc
    if "@" not in netloc or ":" not in netloc:
        return url
    userinfo, hostpart = netloc.rsplit("@", 1)
    host, apikey = hostpart.split(":", 1)
    if not apikey:
        return url
    path = (parsed.path or "/").lstrip("/")
    if path:
        new_path = f"/{apikey}/{path}"
    else:
        new_path = f"/{apikey}/"
    new_netloc = f"{userinfo}@{host}"
    return urllib.parse.urlunsplit(
        (parsed.scheme, new_netloc, new_path, parsed.query, parsed.fragment)
    )


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

    plus_match = re.search(r"^\^?([A-Za-z0-9]*)(\[[^\]]+\])\+([^$]*)\$?$", regex)
    if plus_match:
        prefix = plus_match.group(1) or ""
        class_group = plus_match.group(2).strip("[]")
        suffix = plus_match.group(3) or ""
        sample_char = "a"
        lowered = class_group.lower()
        if "0-9" in lowered and ("a-z" in lowered or "a-f" in lowered):
            sample_char = "a"
        elif "0-9" in lowered and "a-z" not in lowered and "a-f" not in lowered:
            sample_char = "1"
        elif "a-z" in lowered or "a-f" in lowered:
            sample_char = "a"
        elif "A-Z" in class_group:
            sample_char = "A"
        candidate = prefix + (sample_char * 8) + suffix
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
        candidate = sample_char * size
        if regex.rstrip("$").endswith("=="):
            candidate += "=="
        return candidate

    range_match = re.search(r"\{(\d+)(?:,(\d+))?\}", regex)
    if not range_match:
        range_match = re.search(r"\{(\d+),\}", regex)
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
        candidate = sample_char * size
        if regex.rstrip("$").endswith("=="):
            candidate += "=="
        return candidate

    return "token"


def sample_for_spec(name, spec):
    value = sample_for_name(name)
    if isinstance(spec, dict):
        mapped_name = spec.get("map_to")
        if isinstance(mapped_name, str) and mapped_name and mapped_name != name:
            mapped_value = sample_for_name(mapped_name)
            if value == "token" and mapped_value != "token":
                value = mapped_value
        default = spec.get("default")
        value_from_values = False
        label = str(spec.get("name") or "")
        lowered_name = name.lower()
        label_lower = label.lower()
        lock_value = False
        if "template" in lowered_name and (
            "path" in label_lower or "file" in label_lower
        ):
            value = fixture_path("workflow_template.json")
            lock_value = True
        if "keyfile" in lowered_name:
            if "oauth" in label_lower:
                value = fixture_path("fcm_test_key.json")
            else:
                value = fixture_path("vapid_test_key.pem")
            lock_value = True
        if "subfile" in lowered_name:
            value = fixture_path("vapid_test_sub.json")
            lock_value = True
        if default is not None:
            default_value = query_value_from_default(default, spec)
            if default_value is not None:
                value = default_value
        values = spec.get("values")
        if values and default is None:
            if isinstance(values, (list, tuple)) and values:
                value = str(values[0])
                value_from_values = True
            elif isinstance(values, (set, frozenset)) and values:
                value = str(sorted(values)[0])
                value_from_values = True
        arg_type = str(spec.get("type") or "").lower()
        if (
            arg_type.startswith("int") or arg_type.startswith("float")
        ) and not value_from_values:
            value = "1"
        else:
            suggested = sample_for_label(label)
            if suggested and not lock_value and "subscriber" not in lowered_name:
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
            group_list = sorted(str(item) for item in group)
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


def is_matrix_t2bot_template(schema, template):
    if schema.lower() not in ("matrix", "matrixs"):
        return False
    if "{token}" not in template:
        return False
    if "{host}" in template or "{targets}" in template:
        return False
    return True


def fill_template(template, schema, tokens):
    values = {"schema": schema}
    t2bot_template = is_matrix_t2bot_template(schema, template)
    for name, spec in tokens.items():
        if name == "schema":
            continue
        if t2bot_template and name == "token":
            values[name] = MATRIX_T2BOT_TOKEN
        else:
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
        values = spec.get("values")
        if isinstance(values, (list, tuple)) and values:
            return str(values[0])
        if isinstance(values, (set, frozenset)) and values:
            return str(sorted(values)[0])
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


def ensure_query_params(url, items):
    if not items:
        return url
    parsed = urllib.parse.urlsplit(url)
    existing = {
        k for k, _ in urllib.parse.parse_qsl(parsed.query, keep_blank_values=True)
    }
    missing = [(k, v) for k, v in items if k not in existing]
    return append_query(url, missing) if missing else url


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
        if schema.lower() == "mailgun":
            url = normalize_mailgun_url(url)
        if not is_matrix_t2bot_template(schema, template):
            url = ensure_targets(url, template, tokens, args)
        if can_parse(url):
            valid_templates.append(url)
            cases.append({"name": f"template-{len(cases) + 1}", "url": url})

    primary = valid_templates[0] if valid_templates else None

    if schema.lower() == "vapid":
        required = [
            ("keyfile", fixture_path("vapid_test_key.pem")),
            ("subfile", fixture_path("vapid_test_sub.json")),
        ]
        for entry in cases:
            entry["url"] = ensure_query_params(entry["url"], required)
        if primary:
            primary = ensure_query_params(primary, required)

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
                        template_url = primary
                        if name.lower() == "mode" and str(value).lower() == "cloud":
                            if isinstance(tokens, dict) and "app_token" in tokens:
                                for candidate in ordered_templates:
                                    if (
                                        "{app_token}" in candidate
                                        or "{app_id}" in candidate
                                    ):
                                        filled = fill_template(
                                            candidate, schema, tokens
                                        )
                                        if filled:
                                            template_url = filled
                                            break
                        if (
                            schema.lower() in ("matrix", "matrixs")
                            and name.lower() == "mode"
                            and str(value).lower() == "t2bot"
                        ):
                            for candidate in ordered_templates:
                                if is_matrix_t2bot_template(schema, candidate):
                                    filled = fill_template(candidate, schema, tokens)
                                    if filled:
                                        template_url = filled
                                        break
                        url = append_query(template_url, [(name, value)])
                        if name.lower() == "mode" and str(value).lower() == "oauth2":
                            keyfile_spec = (
                                tokens.get("keyfile")
                                if isinstance(tokens, dict)
                                else None
                            )
                            keyfile_value = sample_for_token(
                                "keyfile", keyfile_spec, tokens
                            )
                            url = ensure_query_params(url, [("keyfile", keyfile_value)])
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
            if schema.lower() == "msteams" and name.lower() == "template":
                value = fixture_path("msteams_template.json")
            url = append_query(primary, [(name, value)])
            if can_parse(url):
                cases.append({"name": f"arg-{name}", "url": url})

        for name, spec in kwargs.items():
            if not isinstance(spec, dict) or "alias_of" in spec:
                continue
            prefix = spec.get("prefix")
            if prefix not in ("+", "-", ":"):
                prefix = ""
            key_name = "key"
            lowered = name.lower()
            label_lower = str(spec.get("name") or "").lower()
            value = "value"
            if "mapping" in lowered:
                if "template" in lowered:
                    key_name = "1"
                else:
                    key_name = "info"
                    value = "INFO"
                    if "action" in label_lower:
                        value = "new"
            key = f"{prefix}{key_name}"
            url = append_query(primary, [(key, value)])
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

    cached, cache_path = load_cache(requested)
    if cached is not None:
        print(json.dumps(cached, ensure_ascii=True, sort_keys=True))
        return

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

    store_cache(cache_path, cases)
    print(json.dumps(cases, ensure_ascii=True, sort_keys=True))


if __name__ == "__main__":
    main()
