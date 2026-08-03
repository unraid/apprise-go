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

    protocols = details.get("tokens", {}).get("schema", {}).get("values", [])
    secure = [p for p in protocols if p.endswith("s") and p[:-1] in protocols]
    plain = [p for p in protocols if p not in secure]

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
