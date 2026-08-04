import argparse
import base64
import json
import os
import sys
from pathlib import Path

script_dir = Path(__file__).resolve().parent
repo_root = script_dir.parents[2]
providers_root = repo_root / "internal" / "parity" / "providers"

sys.path.insert(0, str(script_dir))

def apprise_source_root():
    env_root = os.environ.get("APPRISE_SOURCE_ROOT", "").strip()
    if env_root:
        candidate = Path(env_root)
        if (candidate / "apprise").is_dir():
            return candidate
    candidate = repo_root.parent / "apprise"
    if (candidate / "apprise").is_dir():
        return candidate
    return None


apprise_root = apprise_source_root()
if apprise_root:
    sys.path.insert(0, str(apprise_root))

from apprise.common import NotifyType  # noqa: E402
from capture_request import capture_request  # noqa: E402

DEFAULT_ENV = {
    "APPRISE_FIXED_TIME": "2024-01-01T00:00:00Z",
    "APPRISE_OAUTH_NONCE": "parity-nonce",
    "APPRISE_OAUTH_TIMESTAMP": "1704067200",
    "APPRISE_VAPID_TEST_JWT": "parity.jwt.token",
    "APPRISE_VAPID_TEST_PUBLIC_KEY": "parity-public-key",
    "APPRISE_VAPID_TEST_ENCRYPTED": "cGFyaXR5LXZhcGlk",
    "APPRISE_SIMPLEPUSH_TEST_IV": "00112233445566778899AABBCCDDEEFF",
}
UPSTREAM_ASSET_BASE = (
    "https://github.com/caronc/apprise/raw/master/apprise/assets/themes/default/"
)
LOCAL_ASSET_BASE = (
    "https://raw.githubusercontent.com/unraid/apprise-go/main/assets/themes/default/"
)
UPSTREAM_APP_URL = "https://github.com/caronc/apprise"
LOCAL_APP_URL = "https://github.com/unraid/apprise-go"


# The Go comparison rewrites every multipart boundary to this before diffing,
# so writing it into the golden keeps the file stable across captures. A
# generated boundary would differ on every run and --check could never pass.
PARITY_BOUNDARY = "APPRISE-PARITY-BOUNDARY"


def normalize_multipart(specs):
    """Rewrite generated multipart boundaries to a fixed one."""
    import re

    for spec in specs:
        headers = spec.get("headers") or {}
        key = next(
            (k for k in headers if k.lower() == "content-type"),
            None,
        )
        if key is None or "multipart/" not in headers[key].lower():
            continue

        match = re.search(r"boundary=([^;]+)", headers[key])
        if not match:
            continue

        boundary = match.group(1).strip('"')
        if not boundary:
            continue

        headers[key] = re.sub(
            r"boundary=[^;]+", f"boundary={PARITY_BOUNDARY}", headers[key]
        )
        for field in ("body", "body_b64"):
            if field == "body_b64" and spec.get(field):
                # Decode, swap, re-encode so the bytes stay faithful.
                raw = base64.b64decode(spec[field])
                spec[field] = base64.b64encode(
                    raw.replace(boundary.encode(), PARITY_BOUNDARY.encode())
                ).decode("ascii")
            elif spec.get(field):
                spec[field] = spec[field].replace(boundary, PARITY_BOUNDARY)

    return specs


def apply_default_env():
    for key, value in DEFAULT_ENV.items():
        os.environ.setdefault(key, value)


def parse_notify_type(raw):
    value = (raw or "").strip().lower()
    if value == "success":
        return NotifyType.SUCCESS
    if value == "warning":
        return NotifyType.WARNING
    if value == "failure":
        return NotifyType.FAILURE
    return NotifyType.INFO


def rewrite_values(value):
    if isinstance(value, str):
        return value.replace(UPSTREAM_ASSET_BASE, LOCAL_ASSET_BASE).replace(
            UPSTREAM_APP_URL, LOCAL_APP_URL
        )
    if isinstance(value, list):
        return [rewrite_values(entry) for entry in value]
    if isinstance(value, dict):
        return {key: rewrite_values(entry) for key, entry in value.items()}
    return value


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "providers",
        nargs="*",
        help="Provider names to update (default: all)",
    )
    parser.add_argument(
        "--check",
        action="store_true",
        help="Verify golden files match generated output without writing",
    )
    return parser.parse_args()


def main():
    args = parse_args()
    apply_default_env()

    parity_root = repo_root / "internal" / "parity"
    os.chdir(parity_root)

    provider_dirs = [p for p in providers_root.iterdir() if p.is_dir()]
    if args.providers:
        wanted = {p.strip() for p in args.providers if p.strip()}
        provider_dirs = [p for p in provider_dirs if p.name in wanted]
        missing = wanted - {p.name for p in provider_dirs}
        if missing:
            raise SystemExit(
                "Unknown provider(s): " + ", ".join(sorted(missing))
            )
    if not provider_dirs:
        raise SystemExit(f"No provider dirs found under {providers_root}")

    changed = []
    for provider_dir in sorted(provider_dirs):
        cases_path = provider_dir / "cases.json"
        if not cases_path.exists():
            continue
        cases = json.loads(cases_path.read_text())
        if not cases:
            raise SystemExit(f"No cases in {cases_path}")

        # Headers the provider cannot reproduce across runs — a signature over
        # a random nonce, say — are written as a placeholder. Without this the
        # golden differs on every capture and --check can never pass. The Go
        # side only asserts these are present, never equal; what they contain
        # is pinned by a vector test instead.
        manifest_path = provider_dir / "manifest.json"
        volatile_headers = set()
        if manifest_path.exists():
            manifest = json.loads(manifest_path.read_text())
            volatile_headers = {
                str(h).strip().lower()
                for h in (manifest.get("volatile_headers") or [])
            }

        golden_cases = []
        for case in cases:
            # {repo} keeps a template path in a fixture portable; apprise
            # resolves a relative path against the home directory, so the URL
            # has to carry an absolute one by the time it is parsed.
            def expand(value):
                return value.replace("%7Brepo%7D", str(repo_root)).replace(
                    "{repo}", str(repo_root)
                )

            case_url = expand(case["url"])
            case_attach = [expand(a) for a in case.get("attachments", [])]
            payload = capture_request(
                case_url,
                case.get("body", ""),
                case.get("title", ""),
                parse_notify_type(case.get("type")),
                None,
                case_attach or None,
            )
            specs = normalize_multipart(rewrite_values(payload.get("requests", [])))
            if volatile_headers:
                for spec in specs:
                    headers = spec.get("headers") or {}
                    for name in list(headers):
                        if name.strip().lower() in volatile_headers:
                            headers[name] = "<volatile>"
            golden_cases.append({"name": case["name"], "requests": specs})

        golden_path = provider_dir / "golden.json"
        rendered = json.dumps(golden_cases, indent=2, sort_keys=True)
        if args.check:
            existing = ""
            if golden_path.exists():
                existing = golden_path.read_text()
            if existing != rendered:
                changed.append(provider_dir.name)
        else:
            golden_path.write_text(rendered)

    if args.check and changed:
        raise SystemExit(
            "Golden files out of date: " + ", ".join(sorted(changed))
        )


if __name__ == "__main__":
    main()
