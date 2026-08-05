#!/usr/bin/env python3
"""Probe which schemas require the URL to carry a real hostname.

Upstream's NotifyBase.parse_url verifies the host by default and returns None
when it is empty or not a valid hostname, which is why json:// and
mailgun://:@/ are rejected outright. Plugins whose URL has no host -- the ones
where the authority is an api key or a phone number -- opt out with
verify_host=False.

Whether a given plugin opted out is not visible in the schema metadata, so it is
asked of upstream directly: each schema is offered a URL with no host and one
with a syntactically invalid host, and what upstream does with them is recorded.

Emits JSON: {schema: {"rejects_empty": bool, "rejects_invalid": bool}, ...}
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys

INVALID_HOST = "-_"


def _replace_host(url: str, value: str) -> str | None:
    try:
        schema, rest = url.split("://", 1)
    except ValueError:
        return None

    tail = ""
    for idx, ch in enumerate(rest):
        if ch in "/?#":
            tail = rest[idx:]
            rest = rest[:idx]
            break

    userinfo, at, _ = rest.rpartition("@")
    return f"{schema}://{userinfo}{at}{value}{tail}"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--apprise-root", required=True)
    parser.add_argument("--cases-root", required=True)
    args = parser.parse_args()

    logging.disable(logging.CRITICAL)
    sys.path.insert(0, args.apprise_root)

    from apprise import Apprise
    from apprise.plugins import NotifyBase, N_MGR

    # Which schemas actually put a hostname in the host position. A plugin
    # whose authority is an api key or a device id also refuses a malformed
    # value there, but for its own reasons -- dot://apikey@device_id carries an
    # underscore quite legitimately. Applying hostname rules to those rejects
    # working URLs, so the check is limited to schemas whose template names
    # {host}.
    hostname_schemas: set[str] = set()
    ambiguous: set[str] = set()
    for entry in N_MGR.plugins():
        protocols = []
        for attr in ("protocol", "secure_protocol"):
            value = getattr(entry, attr, None)
            if isinstance(value, str):
                protocols.append(value)
            elif isinstance(value, (list, tuple)):
                protocols.extend(v for v in value if isinstance(v, str))
        for template in getattr(entry, "templates", None) or ():
            if not template.startswith("{schema}://"):
                continue
            authority = template[len("{schema}://"):].split("/")[0].split("?")[0]
            hostpart = authority.rpartition("@")[2].split(":")[0]
            if hostpart == "{host}":
                hostname_schemas.update(p.lower() for p in protocols)
            elif hostpart.startswith("{"):
                # Another template puts something that is not a hostname in the
                # same position -- sendpulse takes a client id there as well as
                # a server -- so which rules apply depends on the individual
                # URL and cannot be decided per schema.
                ambiguous.update(p.lower() for p in protocols)

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

    # Upstream's own test URLs, as a corpus of shapes known to be valid.
    # Templates are documentation and do not cover every accepted form:
    # sendpulse declares {host} in both of its templates and still accepts
    # sendpulse://client_id/cs1/?user=..., where the authority is a client id.
    # Any rule that would reject a URL upstream accepts is dropped.
    from apprise.utils.parse import is_hostname, parse_url as _parse_url
    import ast as _ast

    accepted_by_schema: dict[str, list[str]] = {}
    tests_dir = os.path.join(args.apprise_root, "tests")
    if os.path.isdir(tests_dir):
        for name in sorted(os.listdir(tests_dir)):
            if not (name.startswith("test_plugin_") and name.endswith(".py")):
                continue
            try:
                with open(os.path.join(tests_dir, name), encoding="utf-8") as handle:
                    tree = _ast.parse(handle.read())
            except (OSError, SyntaxError):
                continue
            for node in _ast.walk(tree):
                if not isinstance(node, _ast.Assign):
                    continue
                if "apprise_url_tests" not in [
                    t.id for t in node.targets if isinstance(t, _ast.Name)
                ]:
                    continue
                if not isinstance(node.value, (_ast.Tuple, _ast.List)):
                    continue
                for element in node.value.elts:
                    if not isinstance(element, (_ast.Tuple, _ast.List)) or not element.elts:
                        continue
                    first = element.elts[0]
                    if not (
                        isinstance(first, _ast.Constant)
                        and isinstance(first.value, str)
                        and "://" in first.value
                    ):
                        continue
                    if accepted(first.value):
                        accepted_by_schema.setdefault(
                            first.value.split("://", 1)[0].lower(), []
                        ).append(first.value)

    def hostname_rule_conflicts(schema: str) -> bool:
        for candidate in accepted_by_schema.get(schema, []):
            parsed = _parse_url(candidate, verify_host=False)
            if not parsed:
                continue
            host = (parsed.get("host") or "").strip()
            if host and not is_hostname(host):
                return True
        return False

    out: dict[str, dict[str, bool]] = {}
    for schema, base in sorted(base_for.items()):
        if not accepted(base):
            continue

        invalid = _replace_host(base, INVALID_HOST)
        if invalid is None:
            continue

        # The bare form, rather than the base URL with its host removed: a
        # base URL keeps its path and credentials, and upstream may accept it
        # for reasons that have nothing to do with the host being absent.
        rejects_empty = not accepted(f"{schema}://")
        rejects_invalid = (
            schema in hostname_schemas
            and schema not in ambiguous
            and not accepted(invalid)
            and not hostname_rule_conflicts(schema)
        )
        if not (rejects_empty or rejects_invalid):
            continue

        out[schema] = {
            "rejects_empty": rejects_empty,
            "rejects_invalid": rejects_invalid,
        }

    json.dump(out, sys.stdout)
    return 0


if __name__ == "__main__":
    sys.exit(main())
