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
        return {k: normalize(v) for k, v in value.items()}
    if isinstance(value, list):
        return [normalize(v) for v in value]
    if not isinstance(value, (str, int, float, bool, type(None))):
        return str(value)
    return value


def spec_map_to(spec, key):
    if not isinstance(spec, dict):
        return key
    value = spec.get("map_to")
    if isinstance(value, str) and value:
        return value
    return key


def main():
    if len(sys.argv) != 3:
        print("Usage: schema_inputs.py <schema> <url>", file=sys.stderr)
        sys.exit(2)

    schema = sys.argv[1].strip().lower()
    url = sys.argv[2]

    if schema not in N_MGR:
        print(f"Schema not found: {schema}", file=sys.stderr)
        sys.exit(2)

    plugin = N_MGR[schema]
    result = plugin.parse_url(url)
    if not result:
        print("parse_url returned no result", file=sys.stderr)
        sys.exit(2)

    details = plugin_details(plugin)
    values = {}
    kwargs = {}
    aliases = {}

    def handle_specs(specs):
        if not isinstance(specs, dict):
            return
        for key, spec in specs.items():
            if isinstance(spec, dict) and "alias_of" in spec:
                alias = spec.get("alias_of")
                if isinstance(alias, str) and alias:
                    aliases[key] = alias
                continue
            map_to = spec_map_to(spec, key)
            if map_to in result:
                values[map_to] = result.get(map_to)
            elif key in result:
                values[map_to] = result.get(key)

    def handle_kwargs(specs):
        if not isinstance(specs, dict):
            return
        for key, spec in specs.items():
            if isinstance(spec, dict) and "alias_of" in spec:
                alias = spec.get("alias_of")
                if isinstance(alias, str) and alias:
                    aliases[key] = alias
                continue
            map_to = spec_map_to(spec, key)
            value = result.get(map_to)
            if value is None:
                continue
            if isinstance(value, dict):
                kwargs[map_to] = {str(k): str(v) for k, v in value.items()}
            else:
                kwargs[map_to] = {map_to: str(value)}

    handle_specs(details.get("tokens"))
    handle_specs(details.get("args"))
    handle_kwargs(details.get("kwargs"))

    output = {
        "values": normalize(values),
        "kwargs": normalize(kwargs),
        "aliases": aliases,
    }
    print(json.dumps(output, ensure_ascii=True, sort_keys=True))


if __name__ == "__main__":
    main()
