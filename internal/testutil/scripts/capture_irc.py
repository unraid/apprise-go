"""Send one IRC notification through upstream Apprise.

The command stream is recorded by the Go capture server the URL points at;
this only drives upstream and reports whether it believed it succeeded.
"""

import argparse
import json
import logging
import os
import sys

import apprise


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--url")
    parser.add_argument("--body", default="")
    parser.add_argument("--title", default="")
    parser.add_argument(
        "--check",
        action="store_true",
        help="report whether the IRC plugin is available",
    )
    args = parser.parse_args()

    if args.check:
        from apprise.plugins.irc.base import NotifyIRC  # noqa: F401

        print(json.dumps({"success": True}, ensure_ascii=True))
        return

    if os.environ.get("APPRISE_IRC_TRACE"):
        logging.basicConfig(level=logging.DEBUG, stream=sys.stderr)

    apobj = apprise.Apprise()
    apobj.add(args.url)
    success = apobj.notify(body=args.body, title=args.title)
    print(json.dumps({"success": bool(success)}, ensure_ascii=True))


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(json.dumps({"success": False, "error": str(exc)}, ensure_ascii=True))
        sys.exit(1)
