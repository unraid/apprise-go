"""Dump the per-provider limits upstream's overflow logic works from.

These are class attributes rather than URL arguments, so they appear in no
schema entry and nothing else in the harness sees them. The port needs them to
split or truncate a body the same way upstream does.
"""

import json
import sys


def main():
    from apprise.plugins import N_MGR

    N_MGR.load_modules()

    out = {}
    for plugin in N_MGR.plugins():
        protocols = []
        for attr in ("protocol", "secure_protocol"):
            value = getattr(plugin, attr, None)
            if isinstance(value, str):
                protocols.append(value)
            elif isinstance(value, (list, tuple)):
                protocols.extend(v for v in value if isinstance(v, str))

        if not protocols:
            continue

        def value(name, fallback):
            """Read a class attribute, or report it as computed per instance.

            Some plugins define these as properties — XMPP's title_maxlen
            depends on ?subject=, for one — and a property cannot be evaluated
            off the class. Those are reported as null so the caller knows the
            constant does not exist rather than reading a placeholder as fact.
            """
            got = getattr(plugin, name, fallback)
            if isinstance(got, (int, float, bool)) or got is None:
                return got

            return None

        limits = {
            "body_maxlen": value("body_maxlen", 32768),
            "title_maxlen": value("title_maxlen", 250),
            "body_max_line_count": value("body_max_line_count", 0),
            "overflow_amalgamate_title": bool(
                value("overflow_amalgamate_title", False)
            ),
            "overflow_buffer": value("overflow_buffer", 0),
            "overflow_display_count_threshold": value(
                "overflow_display_count_threshold", 130
            ),
            "overflow_max_display_count_width": value(
                "overflow_max_display_count_width", 12
            ),
            "overflow_display_title_once": value(
                "overflow_display_title_once", None
            ),
            "notify_format": str(
                getattr(
                    getattr(plugin, "notify_format", ""), "value", ""
                )
                or ""
            ),
        }

        for protocol in protocols:
            out[protocol] = limits

    print(json.dumps(out, ensure_ascii=True, sort_keys=True))


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(json.dumps({"error": str(exc)}, ensure_ascii=True))
        sys.exit(1)
