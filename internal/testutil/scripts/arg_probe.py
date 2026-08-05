#!/usr/bin/env python3
"""Probe the two remaining kinds of argument validation upstream performs.

The choice and integer tables cover most of what a URL's query string can get
wrong, but two kinds were left out:

  float arguments with a declared range -- pushward's ?volume= must land in
  0.0 to 1.0, and a value outside it raises rather than being clamped;

  arguments carrying their own regex -- strmlabs validates ?name= and
  ?currency=, hassio validates ?nid=.

As with every other table here, the declaration is not the rule: each candidate
is put to upstream and only the ones it genuinely rejects for are recorded, then
checked against upstream's own accepted URLs so a rule that would reject a
working URL is dropped.

Emits JSON: [{schema, arg, kind, min, max, pattern, ignorecase}, ...]
"""

from __future__ import annotations

import argparse
import ast
import json
import logging
import os
import re
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


def _accepted_vectors(apprise_root: str, accepted) -> dict[str, list[str]]:
    """Upstream's own test URLs, grouped by schema, keeping the accepted ones."""
    out: dict[str, list[str]] = {}
    tests_dir = os.path.join(apprise_root, "tests")
    if not os.path.isdir(tests_dir):
        return out

    for name in sorted(os.listdir(tests_dir)):
        if not (name.startswith("test_plugin_") and name.endswith(".py")):
            continue
        try:
            with open(os.path.join(tests_dir, name), encoding="utf-8") as handle:
                tree = ast.parse(handle.read())
        except (OSError, SyntaxError):
            continue
        for node in ast.walk(tree):
            if not isinstance(node, ast.Assign):
                continue
            if "apprise_url_tests" not in [
                t.id for t in node.targets if isinstance(t, ast.Name)
            ]:
                continue
            if not isinstance(node.value, (ast.Tuple, ast.List)):
                continue
            for element in node.value.elts:
                if not isinstance(element, (ast.Tuple, ast.List)) or not element.elts:
                    continue
                first = element.elts[0]
                if not (
                    isinstance(first, ast.Constant)
                    and isinstance(first.value, str)
                    and "://" in first.value
                ):
                    continue
                if accepted(first.value):
                    out.setdefault(
                        first.value.split("://", 1)[0].lower(), []
                    ).append(first.value)
    return out


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--apprise-root", required=True)
    parser.add_argument("--cases-root", required=True)
    args = parser.parse_args()

    logging.disable(logging.CRITICAL)
    sys.path.insert(0, args.apprise_root)

    from apprise import Apprise
    from apprise.plugins import NotifyBase, N_MGR
    from apprise.utils.parse import parse_url as _parse_url

    base_for: dict[str, str] = {}
    for name in sorted(os.listdir(args.cases_root)):
        path = os.path.join(args.cases_root, name, "cases.json")
        if not os.path.isfile(path):
            continue
        with open(path, encoding="utf-8") as handle:
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

    vectors = _accepted_vectors(args.apprise_root, accepted)

    def conflicts(schema: str, arg: str, check) -> bool:
        """Would this rule reject a URL upstream accepts?"""
        for candidate in vectors.get(schema, []):
            parsed = _parse_url(candidate, verify_host=False)
            if not parsed:
                continue
            value = (parsed.get("qsd") or {}).get(arg)
            if value in (None, ""):
                continue
            if not check(str(value)):
                return True
        return False

    records = []
    for entry in N_MGR.plugins():
        protocols = []
        for attr in ("protocol", "secure_protocol"):
            value = getattr(entry, attr, None)
            if isinstance(value, str):
                protocols.append(value)
            elif isinstance(value, (list, tuple)):
                protocols.extend(v for v in value if isinstance(v, str))

        for schema in {p.lower() for p in protocols}:
            base = base_for.get(schema)
            if not base or not accepted(base):
                continue

            for arg, spec in (getattr(entry, "template_args", None) or {}).items():
                if not isinstance(spec, dict):
                    continue

                if spec.get("type") == "float" and (
                    "min" in spec or "max" in spec
                ):
                    low, high = spec.get("min"), spec.get("max")
                    rejects = not accepted(_with_arg(base, arg, NON_NUMERIC))
                    if isinstance(low, (int, float)):
                        rejects = rejects and not accepted(
                            _with_arg(base, arg, str(float(low) - 1))
                        )
                    if isinstance(high, (int, float)):
                        rejects = rejects and not accepted(
                            _with_arg(base, arg, str(float(high) + 1))
                        )
                    if not rejects:
                        continue

                    def in_range(value: str, low=low, high=high) -> bool:
                        try:
                            number = float(value)
                        except ValueError:
                            return False
                        if isinstance(low, (int, float)) and number < low:
                            return False
                        return not (
                            isinstance(high, (int, float)) and number > high
                        )

                    if conflicts(schema, arg, in_range):
                        continue

                    records.append({
                        "schema": schema,
                        "arg": arg,
                        "kind": "float",
                        "min": low,
                        "max": high,
                    })
                    continue

                regex = spec.get("regex")
                if not regex or not isinstance(regex, (list, tuple)):
                    continue
                pattern = regex[0]
                flags = regex[1] if len(regex) > 1 else ""
                if not isinstance(pattern, str):
                    continue
                if any(tok in pattern for tok in ("(?=", "(?!", "(?<")):
                    print(
                        f"skipping {schema}.{arg}: pattern needs lookaround",
                        file=sys.stderr,
                    )
                    continue

                if accepted(_with_arg(base, arg, "zz!!invalid!!zz")):
                    continue

                compiled = re.compile(
                    pattern, re.I if "i" in str(flags or "") else 0
                )
                if conflicts(schema, arg, lambda v: bool(compiled.match(v))):
                    print(
                        f"skipping {schema}.{arg}: would reject a URL upstream "
                        "accepts",
                        file=sys.stderr,
                    )
                    continue

                records.append({
                    "schema": schema,
                    "arg": arg,
                    "kind": "regex",
                    "pattern": pattern,
                    "ignorecase": "i" in str(flags or ""),
                })

    json.dump(records, sys.stdout)
    return 0


if __name__ == "__main__":
    sys.exit(main())
