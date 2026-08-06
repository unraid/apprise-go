#!/usr/bin/env python3
"""Emit a Go schema entry literal from upstream's plugin details.

The schema entries are pure data derived from upstream, so transcribing them
by hand is both tedious and a source of drift. Generating them keeps
TestSchemaMetadataParity honest by construction.

Usage: schema_gen/main.py <schema> [order]
"""
import json
import subprocess
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[3]


def go_value(value, indent):
    pad = "\t" * indent
    inner = "\t" * (indent + 1)

    if isinstance(value, dict):
        if not value:
            return "map[string]any{}"
        lines = ["map[string]any{"]
        for key in sorted(value):
            lines.append(f'{inner}"{key}": {go_value(value[key], indent + 1)},')
        lines.append(pad + "}")
        return "\n".join(lines)

    if isinstance(value, list):
        if not value:
            return "[]any{}"
        if all(isinstance(v, str) for v in value):
            joined = ", ".join(json.dumps(v) for v in value)
            return f"[]string{{{joined}}}"
        joined = ", ".join(go_value(v, indent + 1) for v in value)
        return f"[]any{{{joined}}}"

    if value is None:
        return "nil"
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (int, float)):
        return json.dumps(value)
    return json.dumps(value)


PROTOCOL_SCRIPT = """
import json, sys
from apprise.plugins import N_MGR

plugin = N_MGR[sys.argv[1]]


def listed(value):
    if not value:
        return []
    return [value] if isinstance(value, str) else list(value)


print(json.dumps({
    "plain": listed(getattr(plugin, "protocol", None)),
    "secure": listed(getattr(plugin, "secure_protocol", None)),
}))
"""


def upstream_protocols(schema):
    """Read the plugin's own protocol attributes.

    Inferring this from the schema token values instead — treating a trailing
    's' whose stem is also present as the secure one — mislabels every
    HTTPS-only service that has a single scheme not ending in 's'. Kook is
    declared `secure_protocol` upstream and was emitted as a plain protocol.
    """
    out = subprocess.run(
        [str(REPO / ".venv" / "bin" / "python"), "-c", PROTOCOL_SCRIPT, schema],
        capture_output=True, text=True, cwd=REPO,
    )
    if out.returncode != 0:
        raise SystemExit(f"protocol lookup failed for {schema!r}: {out.stderr.strip()}")

    result = json.loads(out.stdout)
    return result["plain"], result["secure"]


def main():
    schema = sys.argv[1]
    order = sys.argv[2] if len(sys.argv) > 2 else "0"

    script = REPO / "internal" / "testutil" / "scripts" / "schema_metadata.py"
    out = subprocess.run(
        [str(REPO / ".venv" / "bin" / "python"), str(script), schema],
        capture_output=True, text=True, cwd=REPO,
    )
    details = json.loads(out.stdout).get(schema)
    if not details:
        raise SystemExit(f"upstream has no schema {schema!r}")

    plain, secure = upstream_protocols(schema)

    print("func init() {")
    print(f"\tRegisterSchemaEntryOrdered({order}, SchemaEntry{{")
    # attachment_support and category are apprise-go's own classification;
    # check them against the sibling providers rather than trusting these.
    print('\t\t"attachment_support": false,')
    print('\t\t"category":           "native",')
    print(f'\t\t"details": {go_value(details, 2)},')
    print('\t\t"enabled":   true,')
    if plain:
        print(f'\t\t"protocols": {go_value(plain, 2)},')
    print('\t\t"requirements": map[string]any{')
    print('\t\t\t"details":              "",')
    print('\t\t\t"packages_recommended": []any{},')
    print('\t\t\t"packages_required":    []any{},')
    print('\t\t},')
    if secure:
        print(f'\t\t"secure_protocols": {go_value(secure, 2)},')
    print('\t\t"service_name": "<service name>",')
    print("\t})")
    print("}")


main()
