#!/usr/bin/env python3
"""Probe upstream's real behaviour for integer arguments with a declared range.

Upstream declares 172 arguments as ``{"type": "int", "min": N, "max": M}``. As
with choice arguments, the declaration does not tell you what happens when a
value falls outside it: some plugins raise, some clamp to the bound, and some
ignore the range entirely and use whatever was passed. Enforcing all of them
would reject URLs upstream accepts; enforcing none lets ?port=65536 or
?version=0 through to a request that cannot work.

So each argument is put to upstream three ways -- a non-numeric value, one below
the minimum, one above the maximum -- and what Apprise.instantiate() does with
each is recorded.

Emits JSON: [{schema, arg, min, max, rejects_nonnumeric, rejects_below,
rejects_above, base}, ...]
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit

NON_NUMERIC = "zzinvalidzz"


def _with_arg(url: str, arg: str, value: str) -> str:
    parts = urlsplit(url)
    query = [
        (k, v)
        for k, v in parse_qsl(parts.query, keep_blank_values=True)
        if k != arg
    ]
    query.append((arg, value))
    return urlunsplit(
        (parts.scheme, parts.netloc, parts.path, urlencode(query), parts.fragment)
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--apprise-root", required=True)
    parser.add_argument("--cases-root", required=True)
    parser.add_argument("--schema-json", required=True)
    args = parser.parse_args()

    logging.disable(logging.CRITICAL)
    sys.path.insert(0, args.apprise_root)

    from apprise import Apprise
    from apprise.plugins import NotifyBase

    with open(args.schema_json, encoding="utf-8") as handle:
        details = json.load(handle)

    base_for: dict[str, str] = {}
    for name in sorted(os.listdir(args.cases_root)):
        cases_path = os.path.join(args.cases_root, name, "cases.json")
        if not os.path.isfile(cases_path):
            continue
        with open(cases_path, encoding="utf-8") as handle:
            try:
                cases = json.load(handle)
            except json.JSONDecodeError:
                continue
        for case in cases:
            url = case.get("url", "")
            if "://" in url:
                base_for.setdefault(url.split("://", 1)[0].lower(), url)

    def accepted(url: str) -> bool:
        try:
            return isinstance(
                Apprise.instantiate(url, suppress_exceptions=False), NotifyBase
            )
        except Exception:
            return False

    records = []
    entries = details.get("schemas", details)
    if isinstance(entries, dict):
        entries = list(entries.values())

    for entry in entries:
        if not isinstance(entry, dict):
            continue
        protocols = []
        for key in ("protocols", "secure_protocols"):
            value = entry.get(key) or []
            if isinstance(value, str):
                protocols.append(value)
            elif isinstance(value, list):
                protocols.extend(v for v in value if isinstance(v, str))

        entry_args = (
            entry.get("details", {}).get("args", {})
            if "details" in entry
            else entry.get("args", {})
        )
        if not isinstance(entry_args, dict):
            continue

        for schema in protocols:
            schema = schema.lower()
            base = base_for.get(schema)
            if not base or not accepted(base):
                continue

            for arg, spec in entry_args.items():
                if not isinstance(spec, dict) or spec.get("type") != "int":
                    continue
                low, high = spec.get("min"), spec.get("max")
                # An int argument with no declared bounds is still an int:
                # telegram's ?topic= raises on "invalid" rather than ignoring
                # it, and skipping unbounded arguments missed that entirely.

                record = {
                    "schema": schema,
                    "arg": arg,
                    "min": low,
                    "max": high,
                    "rejects_nonnumeric": not accepted(
                        _with_arg(base, arg, NON_NUMERIC)
                    ),
                    "rejects_below": None,
                    "rejects_above": None,
                    "base": base,
                }
                if isinstance(low, int):
                    record["rejects_below"] = not accepted(
                        _with_arg(base, arg, str(low - 1))
                    )
                if isinstance(high, int):
                    record["rejects_above"] = not accepted(
                        _with_arg(base, arg, str(high + 1))
                    )

                records.append(record)

    json.dump(records, sys.stdout)
    return 0


if __name__ == "__main__":
    sys.exit(main())
