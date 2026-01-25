import json

import apprise


def normalize(value):
    if isinstance(value, (set, frozenset)):
        return sorted(value, key=lambda item: str(item))
    if isinstance(value, tuple):
        return [normalize(item) for item in value]
    if isinstance(value, bytes):
        return value.decode("utf-8", errors="replace")
    if isinstance(value, dict):
        return {k: normalize(v) for k, v in value.items()}
    if isinstance(value, list):
        return [normalize(v) for v in value]
    if not isinstance(value, (str, int, float, bool, type(None))):
        return str(value)
    return value


def main():
    apobj = apprise.Apprise()
    details = apobj.details(show_requirements=True, show_disabled=True)
    asset = details.get("asset", {})
    image_path_mask = asset.get("image_path_mask")
    if isinstance(image_path_mask, str):
        marker = "apprise/assets/themes/"
        normalized = image_path_mask.replace("\\", "/")
        idx = normalized.find(marker)
        if idx != -1:
            asset["image_path_mask"] = normalized[idx:]
    details = normalize(details)
    print(json.dumps(details, ensure_ascii=True, sort_keys=True))


if __name__ == "__main__":
    main()
