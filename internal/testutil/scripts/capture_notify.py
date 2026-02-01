import argparse
import json
import sys

import apprise


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", required=True)
    parser.add_argument("--body", default="")
    parser.add_argument("--title", default="")
    parser.add_argument("--type", default="info")
    parser.add_argument("--ca", default="")
    args = parser.parse_args()

    if args.ca:
        try:
            from apprise.plugins.mqtt import NotifyMQTT

            NotifyMQTT.CA_CERTIFICATE_FILE_LOCATIONS = property(
                lambda self: [args.ca]
            )
        except Exception:
            pass

    apobj = apprise.Apprise()
    apobj.add(args.url)
    success = apobj.notify(
        body=args.body, title=args.title, notify_type=args.type
    )
    print(json.dumps({"success": bool(success)}, ensure_ascii=True))


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(json.dumps({"success": False, "error": str(exc)}, ensure_ascii=True))
        sys.exit(1)
