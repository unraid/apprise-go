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
    "+15555550123",
    "15555550123",
    "topic",
    "channel",
    "ABC@DEF",
    "user@localhost",
]

CACHE_VERSION = 1
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


def cache_key():
    try:
        import apprise as apprise_module

        apprise_version = getattr(apprise_module, "__version__", "")
    except Exception:
        apprise_version = ""
    payload = {
        "version": CACHE_VERSION,
        "script": "schema_parity_cases",
        "apprise_version": apprise_version,
        "apprise_sha": apprise_git_sha(),
        "python_version": sys.version,
    }
    digest = hashlib.sha256(
        json.dumps(payload, sort_keys=True, separators=(",", ":")).encode("utf-8")
    ).hexdigest()
    return digest


def load_cache():
    if not cache_enabled():
        return None, None
    digest = cache_key()
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
    if "webhook" in lowered or "url" in lowered:
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

    if "xox" in regex.lower():
        candidate = "xoxb-12345-ABCDE"
        if match_regex(regex, flags, candidate):
            return candidate

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
    if isinstance(spec, dict) and spec.get("group"):
        group = spec.get("group")
        if isinstance(group, (list, tuple, set)) and group:
            value = sample_for_name(str(list(group)[0]))
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
        if name == "schema":
            continue
        values[name] = sample_for_token(name, spec)

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


def build_query(details):
    args = details.get("args") or {}
    params = {}
    for name, spec in args.items():
        if not isinstance(spec, dict):
            continue
        if "alias_of" in spec:
            continue
        if name in ("cache",):
            continue
        arg_type = str(spec.get("type") or "").lower()
        if arg_type.startswith("int") or arg_type.startswith("float"):
            continue
        if arg_type.startswith("choice:int") or arg_type.startswith("choice:float"):
            continue
        if arg_type == "string":
            continue
        default = spec.get("default", None)
        value = query_value_from_default(default, spec)
        if value is None:
            continue
        params[name] = value

    if not params:
        return ""
    return urllib.parse.urlencode(params, doseq=True)


def generate_url(schema, plugin, info):
    details = info if isinstance(info, dict) else {}
    templates = details.get("templates") or []
    tokens = details.get("tokens") or {}
    query = build_query(details)

    ordered_templates = sorted(
        (t for t in templates if isinstance(t, str)),
        key=lambda value: (is_simple_template(value), template_token_count(value)),
        reverse=True,
    )

    for template in ordered_templates:
        if not isinstance(template, str):
            continue
        url = fill_template(template, schema, tokens)
        if not url:
            continue
        if query:
            sep = "&" if "?" in url else "?"
            url = f"{url}{sep}{query}"
        results = plugin.parse_url(url)
        if results:
            return url

    for fallback in (
        f"{schema}://example.com",
        f"{schema}://token",
        f"{schema}://user:pass@example.com",
        f"{schema}://user:pass@example.com/target",
        f"{schema}://token:token@12345",
        f"{schema}://token:token/12345",
        f"{schema}://12345:ABC/12345",
    ):
        url = fallback
        if query:
            sep = "&" if "?" in url else "?"
            url = f"{url}{sep}{query}"
        results = plugin.parse_url(url)
        if results:
            return url

    return None


def main():
    failures = []
    cases = []

    cached, cache_path = load_cache()
    if cached is not None:
        print(json.dumps(cached, ensure_ascii=True, sort_keys=True))
        return

    schemas = list(N_MGR.schemas()) if hasattr(N_MGR, "schemas") else list(N_MGR)
    for schema in sorted(schemas):
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

    store_cache(cache_path, cases)
    print(json.dumps(cases, ensure_ascii=True, sort_keys=True))


if __name__ == "__main__":
    main()
