"""Send one XMPP notification through upstream Apprise.

The stanzas themselves are recorded by the Go capture server the URL points
at; this only drives upstream and reports whether it believed it succeeded.
"""

import argparse
import json
import sys

import logging
import os

import apprise


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--url")
    parser.add_argument(
        "--check",
        action="store_true",
        help="report whether the XMPP plugin and its dependency are available",
    )
    parser.add_argument("--body", default="")
    parser.add_argument("--title", default="")
    parser.add_argument(
        "--repeat",
        type=int,
        default=1,
        help="send this many times through one notifier, which is what "
        "keepalive changes the behaviour of",
    )
    args = parser.parse_args()

    if args.check:
        # The adapter imports slixmpp lazily, so importing it here is what
        # actually proves the dependency is installed.
        import slixmpp  # noqa: F401

        from apprise.plugins.xmpp.adapter import SlixmppAdapter  # noqa: F401

        print(json.dumps({"success": True}, ensure_ascii=True))
        return

    if os.environ.get("APPRISE_XMPP_TRACE"):
        logging.basicConfig(level=logging.DEBUG, stream=sys.stderr)

    apobj = apprise.Apprise()
    apobj.add(args.url)
    success = True
    for _ in range(max(1, args.repeat)):
        success = apobj.notify(body=args.body, title=args.title) and success
    print(json.dumps({"success": bool(success)}, ensure_ascii=True))


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(json.dumps({"success": False, "error": str(exc)}, ensure_ascii=True))
        sys.exit(1)
