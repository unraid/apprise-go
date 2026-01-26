import json
import sys

import apprise


def normalize(value):
    if isinstance(value, dict):
        return {str(k): normalize(v) for k, v in value.items()}
    if isinstance(value, (list, tuple)):
        return [normalize(v) for v in value]
    if isinstance(value, (set, frozenset)):
        items = [normalize(v) for v in value]
        return sorted(items, key=lambda x: json.dumps(x, sort_keys=True))
    if isinstance(value, (str, int, float, bool)) or value is None:
        return value
    return str(value)


if __name__ == "__main__":
    details = apprise.Apprise().details(show_requirements=True, show_disabled=True)
    normalized = normalize(details)
    json.dump(normalized, sys.stdout, sort_keys=True)
