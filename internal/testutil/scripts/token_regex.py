#!/usr/bin/env python3
"""Extract upstream's credential format checks and which URL field they guard.

Upstream validates its tokens with validate_regex(value, *regex) and raises when
one does not match, which is why it rejects things like
stackfield://not-a-valid-uuid or sendgrid://invalid-api-key+*-d:user@example.com
while a port that only checks for emptiness accepts them and sends a request
that cannot succeed.

The regexes live in each plugin's template_tokens. Which URL field a token
occupies is not stated there -- it comes from the templates. The first template
is read to work out which token sits in the host position, which in the user
position and which in the password position, so the check can be applied
centrally instead of hand-copied into 150 providers.

Only host/user/password are emitted. Path tokens are positional and depend on
how each plugin splits its path, which is not something a shared validator can
know.

Emits JSON: [{schema, field, token, pattern, ignorecase, verified}, ...]
where `verified` records that upstream really does reject a value failing the
pattern -- a declared regex that upstream never enforces must not be enforced
here either.
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import re
import sys

# The first path-free segment of a template, e.g. "{apikey}" or
# "{user}:{password}@{host}".
AUTHORITY_RE = re.compile(r"^\{schema\}://(?P<authority>[^/?]*)")


def _authority_tokens(template: str) -> dict[str, str]:
    """Map url field -> token name for one template."""
    m = AUTHORITY_RE.match(template)
    if not m:
        return {}
    authority = m.group("authority")

    fields: dict[str, str] = {}

    # A template can carry no literal @ and still describe userinfo, because
    # one of its tokens supplies it: brevo's "{apikey}:{from_email}" becomes
    # brevo://abcd:user@example.com, where the apikey is the user field and the
    # email contributes both the password and the host. Reading the authority
    # positionally would call the apikey a hostname and check it against the
    # wrong value.
    if "@" not in authority and authority.count(":") == 1:
        first, _, second = authority.partition(":")
        if first.startswith("{") and second.startswith("{"):
            return {"user": first.strip("{}")}

    userinfo, _, hostpart = authority.rpartition("@")
    if userinfo:
        parts = userinfo.split(":")
        if len(parts) >= 1 and parts[0].startswith("{"):
            fields["user"] = parts[0].strip("{}")
        if len(parts) >= 2 and parts[1].startswith("{"):
            fields["password"] = parts[1].strip("{}")

    hostpart = hostpart.split(":")[0]
    if hostpart.startswith("{") and hostpart.endswith("}"):
        fields["host"] = hostpart.strip("{}")
    return fields


def _substitute(url: str, field: str, value: str) -> str | None:
    """Rewrite one authority field of a URL, leaving the rest intact."""
    try:
        schema, rest = url.split("://", 1)
    except ValueError:
        return None

    tail = ""
    for sep in ("/", "?"):
        idx = rest.find(sep)
        if idx != -1:
            tail = rest[idx:] + tail if sep == "/" else rest[idx:]
            rest = rest[:idx]
            break

    userinfo, at, host = rest.rpartition("@")
    user, colon, password = userinfo.partition(":")

    if field == "host":
        host = value
    elif field == "user":
        user, at = value, "@"
    elif field == "password":
        password, colon, at = value, ":", "@"
        if not user:
            return None
    else:
        return None

    userinfo = f"{user}{colon}{password}" if at else ""
    return f"{schema}://{userinfo}{at}{host}{tail}"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--apprise-root", required=True)
    parser.add_argument("--cases-root", required=True)
    args = parser.parse_args()

    logging.disable(logging.CRITICAL)
    sys.path.insert(0, args.apprise_root)

    from apprise import Apprise
    from apprise.plugins import NotifyBase, N_MGR
    from apprise.utils.parse import parse_url

    # A working URL per schema, so each candidate check can be verified against
    # upstream rather than trusted from the declaration.
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

    # Upstream's own test URLs, as a corpus of shapes known to be valid. A
    # check verified against a single base URL can still be wrong: resend's
    # apikey sits in the host of one template and in the user of another, so a
    # host check derived from the first rejects every URL written the second
    # way. Any candidate that would reject a URL upstream accepts is dropped.
    accepted_urls: dict[str, list[str]] = {}
    tests_dir = os.path.join(args.apprise_root, "tests")
    if os.path.isdir(tests_dir):
        import ast as _ast

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
                    url = first.value
                    if accepted(url):
                        accepted_urls.setdefault(
                            url.split("://", 1)[0].lower(), []
                        ).append(url)

    records = []
    seen: set[tuple[str, str]] = set()

    for entry in N_MGR.plugins():
        tokens = getattr(entry, "template_tokens", None) or {}
        templates = getattr(entry, "templates", None) or ()
        if not tokens or not templates:
            continue

        protocols = []
        for attr in ("protocol", "secure_protocol"):
            value = getattr(entry, attr, None)
            if isinstance(value, str):
                protocols.append(value)
            elif isinstance(value, (list, tuple)):
                protocols.extend(v for v in value if isinstance(v, str))

        # Fields are read from whichever template names them. A field is only
        # usable when every template that names it names the *same* token:
        # slack puts token_a in the host for a webhook URL and access_token
        # there for a bot URL, and the two have incompatible formats, so a
        # single check on the host would reject one form or the other. Where
        # the templates disagree, the shape depends on context this validator
        # does not have and the provider has to decide.
        candidates: dict[str, set[str]] = {}
        for template in templates:
            for field, token in _authority_tokens(template).items():
                candidates.setdefault(field, set()).add(token)
        fields = {
            field: next(iter(tokens))
            for field, tokens in candidates.items()
            if len(tokens) == 1
        }

        for schema in protocols:
            schema = schema.lower()
            base = base_for.get(schema)
            if not base or not accepted(base):
                continue

            for field, token in fields.items():
                spec = tokens.get(token)
                if not isinstance(spec, dict):
                    continue
                regex = spec.get("regex")
                if not regex or not isinstance(regex, (list, tuple)):
                    continue
                pattern = regex[0]
                flags = regex[1] if len(regex) > 1 else ""
                if not isinstance(pattern, str):
                    continue

                # Go's regexp engine has no lookaround. A pattern using it
                # cannot be enforced by the shared validator, and emitting it
                # anyway would leave a rule that silently matches nothing --
                # so it is dropped here, visibly, rather than failing to
                # compile at load time where nobody would notice. Only
                # exotel's from_phone is affected today; if that provider needs
                # the check it has to be written by hand.
                if any(tok in pattern for tok in ("(?=", "(?!", "(?<")):
                    print(
                        f"skipping {schema}.{field}: pattern needs lookaround, "
                        "which Go cannot compile",
                        file=sys.stderr,
                    )
                    continue
                if (schema, field) in seen:
                    continue
                seen.add((schema, field))

                # When a query argument supplies this token the URL field is
                # free to hold something else entirely --
                # wxpusher://123?token=AT_abc1234 puts an app id in the host
                # because ?token= carries the credential. That is a property of
                # the individual URL, not of the schema, so the argument names
                # are recorded and the check is skipped at validation time only
                # when one of them is actually present.
                override_args = sorted({
                    name
                    for name, arg_spec in (
                        getattr(entry, "template_args", None) or {}
                    ).items()
                    if isinstance(arg_spec, dict)
                    and (
                        arg_spec.get("alias_of") == token
                        or arg_spec.get("map_to") == token
                    )
                })

                # A declared regex is not proof upstream enforces it. Put a
                # value that cannot match into that field and see whether
                # upstream actually refuses the URL; if it shrugs, this port
                # must shrug too.
                bad = _substitute(base, field, "zz!!invalid!!zz")
                if bad is None or accepted(bad):
                    continue

                # Would this check reject something upstream accepts?
                compiled = re.compile(
                    pattern, re.I if "i" in str(flags or "") else 0
                )
                conflicts = False
                for candidate in accepted_urls.get(schema, []):
                    parsed = parse_url(candidate, verify_host=False)
                    if not parsed:
                        continue
                    if any(
                        parsed.get(arg) for arg in override_args
                    ) or (parsed.get("qsd") or {}).keys() & set(override_args):
                        continue
                    value = parsed.get(field)
                    if not value:
                        continue
                    if not compiled.match(str(value)):
                        conflicts = True
                        break
                if conflicts:
                    print(
                        f"skipping {schema}.{field}: would reject a URL "
                        "upstream accepts",
                        file=sys.stderr,
                    )
                    continue

                records.append({
                    "schema": schema,
                    "field": field,
                    "token": token,
                    "override_args": override_args,
                    "pattern": pattern,
                    "ignorecase": "i" in str(flags or ""),
                })

    json.dump(records, sys.stdout)
    return 0


if __name__ == "__main__":
    sys.exit(main())
