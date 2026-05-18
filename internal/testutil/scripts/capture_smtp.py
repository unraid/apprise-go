import argparse
import json
import sys

import apprise
from apprise import AppriseAsset
from apprise.common import NotifyFormat


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", required=True)
    parser.add_argument("--body", default="")
    parser.add_argument("--title", default="")
    parser.add_argument("--body-format", default="")
    args = parser.parse_args()

    asset_kwargs = {}
    body_format = args.body_format.strip().lower()
    if body_format:
        asset_kwargs["body_format"] = NotifyFormat(body_format)

    apobj = apprise.Apprise(asset=AppriseAsset(**asset_kwargs))
    apobj.add(args.url)
    success = apobj.notify(body=args.body, title=args.title)
    print(json.dumps({"success": bool(success)}, ensure_ascii=True))


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(json.dumps({"success": False, "error": str(exc)}, ensure_ascii=True))
        sys.exit(1)
