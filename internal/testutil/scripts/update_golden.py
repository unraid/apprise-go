import json
import os
import sys
from pathlib import Path

script_dir = Path(__file__).resolve().parent
repo_root = script_dir.parents[2]
providers_root = repo_root / "internal" / "parity" / "providers"

sys.path.insert(0, str(script_dir))

from capture_request import capture_request  # noqa: E402

from apprise.common import NotifyType  # noqa: E402

DEFAULT_ENV = {
    "APPRISE_FIXED_TIME": "2024-01-01T00:00:00Z",
    "APPRISE_OAUTH_NONCE": "parity-nonce",
    "APPRISE_OAUTH_TIMESTAMP": "1704067200",
    "APPRISE_VAPID_TEST_JWT": "parity.jwt.token",
    "APPRISE_VAPID_TEST_PUBLIC_KEY": "parity-public-key",
    "APPRISE_VAPID_TEST_ENCRYPTED": "cGFyaXR5LXZhcGlk",
}
UPSTREAM_ASSET_BASE = (
    "https://github.com/caronc/apprise/raw/master/apprise/assets/themes/default/"
)
LOCAL_ASSET_BASE = (
    "https://raw.githubusercontent.com/unraid/apprise-go/main/assets/themes/default/"
)
UPSTREAM_APP_URL = "https://github.com/caronc/apprise"
LOCAL_APP_URL = "https://github.com/unraid/apprise-go"


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


def main():
    apply_default_env()

    parity_root = repo_root / "internal" / "parity"
    os.chdir(parity_root)

    provider_dirs = [p for p in providers_root.iterdir() if p.is_dir()]
    if not provider_dirs:
        raise SystemExit(f"No provider dirs found under {providers_root}")

    for provider_dir in sorted(provider_dirs):
        cases_path = provider_dir / "cases.json"
        if not cases_path.exists():
            continue
        cases = json.loads(cases_path.read_text())
        if not cases:
            raise SystemExit(f"No cases in {cases_path}")

        golden_cases = []
        for case in cases:
            specs = capture_request(
                case["url"],
                case.get("body", ""),
                case.get("title", ""),
                parse_notify_type(case.get("type")),
            )
            golden_cases.append(
                {"name": case["name"], "requests": rewrite_values(specs)}
            )

        golden_path = provider_dir / "golden.json"
        golden_path.write_text(json.dumps(golden_cases, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
