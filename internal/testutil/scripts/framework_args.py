"""List the arguments every provider inherits from upstream's NotifyBase.

These are the framework-level knobs — overflow, retry, timeouts and the rest —
that each plugin declares without implementing, because the base class acts on
them. They are easy to miss when porting: the schema entry carries them, so
metadata parity passes whether or not anything behind them exists.
"""

import json
import sys

from apprise.plugins.base import NotifyBase


def main():
    out = {}
    for name, spec in NotifyBase.template_args.items():
        if "alias_of" in spec:
            # An alias points at another argument; it is not its own knob.
            continue

        default = spec.get("default")
        # Enum members serialise as their value rather than their repr.
        if hasattr(default, "value"):
            default = default.value

        out[name] = {
            "type": spec.get("type"),
            "default": default,
        }

    print(json.dumps(out, ensure_ascii=True, sort_keys=True))


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(json.dumps({"error": str(exc)}, ensure_ascii=True))
        sys.exit(1)
