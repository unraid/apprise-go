"""Expand upstream's emoji map into literal codes.

Upstream keys the map by regex — ":(laughing|satisfied):" is one entry — and
compiles a single alternation over all of them. Expanding the alternations and
optional groups into literal codes gives the same lookups without carrying a
regex engine over 1800 branches, and the expansion is verified by running every
generated code back through upstream.
"""

import itertools
import json
import re
import sys


def expand(pattern):
    """Yield every literal string a simple pattern can match.

    Only the constructs upstream actually uses are handled: alternation and
    optional groups. Anything else raises, so a new construct is a loud
    failure rather than a silently dropped emoji.
    """
    tokens = []
    index = 0
    while index < len(pattern):
        char = pattern[index]

        if char == "\\":
            tokens.append([pattern[index + 1]])
            index += 2
            continue

        if char == "(":
            depth = 1
            end = index + 1
            while end < len(pattern) and depth:
                if pattern[end] == "(":
                    depth += 1
                elif pattern[end] == ")":
                    depth -= 1
                end += 1

            inner = pattern[index + 1 : end - 1]
            optional = end < len(pattern) and pattern[end] == "?"

            choices = []
            for alternative in split_alternatives(inner):
                choices.extend(expand(alternative))
            if optional:
                choices.append("")
                end += 1

            tokens.append(choices)
            index = end
            continue

        if index + 1 < len(pattern) and pattern[index + 1] == "?":
            tokens.append([char, ""])
            index += 2
            continue

        if char in "[]{}*+^$":
            raise ValueError(f"unsupported construct {char!r} in {pattern!r}")

        tokens.append([char])
        index += 1

    return ["".join(parts) for parts in itertools.product(*tokens)]


def split_alternatives(text):
    """Split on | at depth zero."""
    parts = []
    depth = 0
    current = ""
    for char in text:
        if char == "(":
            depth += 1
        elif char == ")":
            depth -= 1

        if char == "|" and depth == 0:
            parts.append(current)
            current = ""
            continue

        current += char

    parts.append(current)

    return parts


def main():
    from apprise.emojis import EMOJI_MAP, apply_emojis

    out = {}
    for pattern, emoji in EMOJI_MAP.items():
        for literal in expand(pattern):
            out[literal.lower()] = emoji

    # Every generated code has to survive a round trip through upstream, or
    # the expansion has invented something upstream would not match.
    for literal, emoji in out.items():
        if apply_emojis(literal) != emoji:
            raise ValueError(
                f"{literal!r} expands to {emoji!r} but upstream renders "
                f"{apply_emojis(literal)!r}"
            )

    print(json.dumps(out, ensure_ascii=False, sort_keys=True))


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(json.dumps({"error": str(exc)}, ensure_ascii=False))
        sys.exit(1)
