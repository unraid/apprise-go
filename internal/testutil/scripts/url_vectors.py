#!/usr/bin/env python3
"""Extract upstream's own URL vectors and record what upstream does with them.

Upstream's tests/test_plugin_*.py each declare an ``apprise_url_tests`` tuple of
(url, metadata) pairs. Those URLs are the part this port cannot produce for
itself: every fixture in internal/parity/providers was written here, so it can
only prove parity on URL shapes someone here thought to try. Upstream's vectors
were written by the people who wrote the plugins, and they lean heavily on the
malformed, the half-specified, and the deliberately hostile.

The URL list is taken from the test files. The expected result is NOT taken from
the test metadata -- it is taken from actually calling Apprise.instantiate(), so
the oracle is upstream's real parser rather than this script's reading of a test
declaration. The declared ``instance`` is still recorded so the two can be
cross-checked; a disagreement means the extraction is wrong, not the port.

Emits JSON to stdout: [{schema, url, accepted, declared, source}, ...]
"""

from __future__ import annotations

import argparse
import ast
import json
import logging
import os
import sys


def _silence() -> None:
    logging.disable(logging.CRITICAL)
    os.environ.setdefault("APPRISE_TEST", "1")


def _urls_from_source(path: str) -> list[str]:
    """Pull the string URLs out of a test module's apprise_url_tests.

    Parsed with ast rather than imported: importing the test module drags in
    pytest fixtures, optional third-party dependencies, and module-level
    monkeypatching, and a module that fails to import for any of those reasons
    would silently contribute zero vectors. The tuple is a literal, so reading
    it statically is both sufficient and total.
    """
    with open(path, encoding="utf-8") as handle:
        tree = ast.parse(handle.read(), filename=path)

    urls: list[str] = []
    for node in ast.walk(tree):
        if not isinstance(node, ast.Assign):
            continue
        targets = [t.id for t in node.targets if isinstance(t, ast.Name)]
        if "apprise_url_tests" not in targets:
            continue
        if not isinstance(node.value, (ast.Tuple, ast.List)):
            continue
        for element in node.value.elts:
            if not isinstance(element, (ast.Tuple, ast.List)) or not element.elts:
                continue
            first = element.elts[0]
            if isinstance(first, ast.Constant) and isinstance(first.value, str):
                urls.append(first.value)
    return urls


def _declared_instance(path: str) -> dict[str, str]:
    """Map url -> the declared ``instance`` name, for cross-checking only."""
    with open(path, encoding="utf-8") as handle:
        tree = ast.parse(handle.read(), filename=path)

    declared: dict[str, str] = {}
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
            if not isinstance(element, (ast.Tuple, ast.List)) or len(element.elts) < 2:
                continue
            first, second = element.elts[0], element.elts[1]
            if not (isinstance(first, ast.Constant) and isinstance(first.value, str)):
                continue
            if not isinstance(second, ast.Dict):
                continue
            for key, value in zip(second.keys, second.values):
                if not (isinstance(key, ast.Constant) and key.value == "instance"):
                    continue
                if isinstance(value, ast.Name):
                    declared[first.value] = value.id
                elif isinstance(value, ast.Constant) and value.value is None:
                    declared[first.value] = "None"
                elif isinstance(value, ast.Attribute):
                    declared[first.value] = value.attr
    return declared


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--apprise-root", required=True)
    parser.add_argument(
        "--schema",
        action="append",
        default=[],
        help="restrict output to these schemas (repeatable); default is all",
    )
    args = parser.parse_args()

    _silence()
    sys.path.insert(0, args.apprise_root)

    from apprise import Apprise
    from apprise.plugins import NotifyBase

    tests_dir = os.path.join(args.apprise_root, "tests")
    if not os.path.isdir(tests_dir):
        print(f"no tests directory at {tests_dir}", file=sys.stderr)
        return 2

    wanted = {s.strip().lower() for s in args.schema if s.strip()}
    records: list[dict[str, object]] = []
    seen: set[str] = set()

    for name in sorted(os.listdir(tests_dir)):
        if not (name.startswith("test_plugin_") and name.endswith(".py")):
            continue
        path = os.path.join(tests_dir, name)
        declared = _declared_instance(path)

        for url in _urls_from_source(path):
            if url in seen:
                continue
            seen.add(url)

            schema = url.split("://", 1)[0].strip().lower() if "://" in url else ""
            if not schema:
                continue
            if wanted and schema not in wanted:
                continue

            # Ground truth: what upstream's parser actually does with this URL.
            try:
                obj = Apprise.instantiate(url, suppress_exceptions=False)
                accepted = isinstance(obj, NotifyBase)
            except Exception:
                accepted = False

            records.append({
                "schema": schema,
                "url": url,
                "accepted": accepted,
                "declared": declared.get(url, ""),
                "source": name,
            })

    json.dump(records, sys.stdout)
    return 0


if __name__ == "__main__":
    sys.exit(main())
