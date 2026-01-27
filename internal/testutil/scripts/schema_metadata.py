import json
import sys

from apprise.plugins import N_MGR
from apprise.plugins import details as plugin_details


def normalize(value):
    if isinstance(value, (set, frozenset)):
        return sorted(value, key=lambda item: str(item))
    if isinstance(value, tuple):
        return [normalize(item) for item in value]
    if isinstance(value, bytes):
        return value.decode("utf-8", errors="replace")
    if isinstance(value, dict):
        return {str(k): normalize(v) for k, v in value.items()}
    if isinstance(value, list):
        return [normalize(v) for v in value]
    if not isinstance(value, (str, int, float, bool, type(None))):
        return str(value)
    return value


def main():
    schemas = list(N_MGR.schemas()) if hasattr(N_MGR, "schemas") else list(N_MGR)
    requested = set(s.strip().lower() for s in sys.argv[1:] if s.strip())
    if requested:
        schemas = [s for s in schemas if s.lower() in requested]

    output = {}
    for schema in sorted(schemas):
        plugin = N_MGR[schema]
        details = plugin_details(plugin)
        output[schema.lower()] = normalize(details)

    print(json.dumps(output, ensure_ascii=True, sort_keys=True))


if __name__ == "__main__":
    main()
