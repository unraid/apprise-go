import argparse
import json
import sys

import apprise


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", required=True)
    parser.add_argument("--body", default="")
    parser.add_argument("--title", default="")
    parser.add_argument("--port", required=True)
    args = parser.parse_args()

    from apprise.plugins.smpp import NotifySMPP

    port = int(args.port)
    NotifySMPP.default_port = port
    NotifySMPP.default_secure_port = port

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
