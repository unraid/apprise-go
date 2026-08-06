#!/usr/bin/env python3
"""Probe upstream's real behaviour for every choice-valued argument.

Upstream's plugins declare choice arguments as
``{"type": "choice:string", "values": [...]}``. What they do with a value
outside that set is *not* uniform: most raise TypeError, some fall back to the
default, and some accept any string and only use the choice list for
documentation. Reading 150 plugins to find out is slow and error-prone, so this
asks upstream directly.

For each schema, a known-good URL is taken from the port's own parity cases and
re-issued with one argument set to a value that cannot be a legal choice. What
Apprise.instantiate() then does is the answer.

For the arguments that do reject, the alias maps upstream matches against are
read statically out of the plugin source. Upstream's common idiom matches on
prefix -- ``?mode=pri`` legitimately selects ``private`` -- and several plugins
layer a short-alias map on top of that.

Emits JSON: [{schema, arg, values, rejects_invalid, aliases, base}, ...]
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import sys
from urllib.parse import urlencode, urlsplit, urlunsplit, parse_qsl

INVALID = "zzinvalidzz"


def _with_arg(url: str, arg: str, value: str) -> str:
    parts = urlsplit(url)
    query = [(k, v) for k, v in parse_qsl(parts.query, keep_blank_values=True) if k != arg]
    query.append((arg, value))
    return urlunsplit((parts.scheme, parts.netloc, parts.path, urlencode(query), parts.fragment))


def _alias_maps(apprise_root: str) -> list[tuple[frozenset[str], set[str]]]:
    """Extract upstream's choice alias maps statically.

    Several plugins accept short aliases through a module-level dict and match
    with ``input.startswith(key)`` -- octopush's OCTOPUSH_TYPE_MAP turns "p",
    "sms_p", "smsp" and "+" into "sms_premium", which is why ?type=premium
    works despite not being a declared value.

    These are read out of the source rather than measured. Measuring them by
    instantiating URLs does not work: whether a given value is accepted depends
    on the rest of the URL (matrix rejects mode=slack on a t2bot-shaped URL for
    reasons that have nothing to do with the choice list), so a probe attributes
    unrelated failures to the argument under test.

    Returns (declared_values, alias_keys) pairs so a map can be matched to the
    argument whose value set it produces.
    """
    import ast as _ast

    results: list[tuple[frozenset[str], set[str]]] = []
    plugins_dir = os.path.join(apprise_root, "apprise", "plugins")
    if not os.path.isdir(plugins_dir):
        return results

    for name in sorted(os.listdir(plugins_dir)):
        if not name.endswith(".py"):
            continue
        try:
            with open(os.path.join(plugins_dir, name), encoding="utf-8") as handle:
                tree = _ast.parse(handle.read())
        except (OSError, SyntaxError):
            continue

        # Class-level string/int constants, so OctopushType.PREMIUM resolves.
        consts: dict[str, str] = {}
        for node in tree.body:
            if not isinstance(node, _ast.ClassDef):
                continue
            for item in node.body:
                if not isinstance(item, _ast.Assign) or not isinstance(
                    item.value, _ast.Constant
                ):
                    continue
                if not isinstance(item.value.value, (str, int)):
                    continue
                for target in item.targets:
                    if isinstance(target, _ast.Name):
                        consts[f"{node.name}.{target.id}"] = str(item.value.value)

        def resolve(node) -> str | None:
            if isinstance(node, _ast.Constant) and isinstance(node.value, (str, int)):
                return str(node.value)
            if isinstance(node, _ast.Attribute) and isinstance(node.value, _ast.Name):
                return consts.get(f"{node.value.id}.{node.attr}")
            return None

        for node in tree.body:
            if not isinstance(node, _ast.Assign) or not isinstance(node.value, _ast.Dict):
                continue
            keys: set[str] = set()
            produced: set[str] = set()
            ok = True
            for key, value in zip(node.value.keys, node.value.values):
                key_text = resolve(key)
                value_text = resolve(value)
                if key_text is None or value_text is None:
                    ok = False
                    break
                keys.add(key_text)
                produced.add(value_text)
            if ok and keys and produced:
                results.append((frozenset(produced), keys))

    return results


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--apprise-root", required=True)
    parser.add_argument("--cases-root", required=True)
    parser.add_argument("--schema-json", required=True, help="port's schema details JSON")
    args = parser.parse_args()

    logging.disable(logging.CRITICAL)
    sys.path.insert(0, args.apprise_root)

    from apprise import Apprise
    from apprise.plugins import NotifyBase

    with open(args.schema_json, encoding="utf-8") as handle:
        details = json.load(handle)

    # A usable base URL per schema, taken from the port's parity cases.
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
            if "://" not in url:
                continue
            schema = url.split("://", 1)[0].lower()
            base_for.setdefault(schema, url)

    def accepted(url: str) -> bool:
        try:
            return isinstance(
                Apprise.instantiate(url, suppress_exceptions=False), NotifyBase
            )
        except Exception:
            return False

    alias_maps = _alias_maps(args.apprise_root)

    records = []
    entries = details.get("schemas", details) if isinstance(details, dict) else details
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

        entry_args = entry.get("details", {}).get("args", {}) if "details" in entry else entry.get("args", {})
        if not isinstance(entry_args, dict):
            continue

        for schema in protocols:
            schema = schema.lower()
            base = base_for.get(schema)
            if not base:
                continue
            # Only probe schemas whose baseline URL upstream already accepts,
            # otherwise every probe trivially "rejects" for unrelated reasons.
            if not accepted(base):
                continue

            for arg, spec in entry_args.items():
                if not isinstance(spec, dict):
                    continue
                if not str(spec.get("type", "")).startswith("choice:"):
                    continue
                values = spec.get("values") or []
                if not isinstance(values, list) or not values:
                    continue
                values = [str(v) for v in values]

                rejects = not accepted(_with_arg(base, arg, INVALID))

                record = {
                    "schema": schema,
                    "arg": arg,
                    "values": values,
                    "rejects_invalid": rejects,
                    "base": base,
                }

                # Only arguments that actually reject need an accepted-value
                # set: for the rest nothing is enforced, so an incomplete set
                # cannot cause a false rejection.
                #
                # The declared values are not the accepted inputs. Several
                # plugins keep a separate alias map -- smseagle declares
                # priority values (0, 1) but accepts "normal", "high" and "+"
                # via SMSEAGLE_PRIORITY_MAP, and octopush accepts "premium"
                # for "sms_premium". Guessing which plugins do this is how you
                # ship a port that rejects working URLs, so the accepted set is
                # measured: candidates are drawn from the plugin's own source
                # and each one is put to upstream's parser.
                if rejects:
                    declared = frozenset(values)
                    aliases: set[str] = set()
                    for produced, keys in alias_maps:
                        # The map belongs to this argument if it produces the
                        # argument's declared values and nothing else.
                        if produced and produced <= declared:
                            aliases |= keys
                    record["aliases"] = sorted(aliases)

                records.append(record)

    json.dump(records, sys.stdout)
    return 0


if __name__ == "__main__":
    sys.exit(main())
