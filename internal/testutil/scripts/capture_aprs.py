import argparse
import json
import sys

import apprise


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", required=True)
    parser.add_argument("--body", default="")
    parser.add_argument("--title", default="")
    parser.add_argument("--host", required=True)
    parser.add_argument("--port", required=True)
    args = parser.parse_args()

    from apprise.plugins import aprs as aprs_module

    aprs_module.APRS_LOCALES["EURO"] = args.host
    aprs_module.NotifyAprs.notify_port = int(args.port)

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
