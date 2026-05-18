import argparse
import base64
import datetime
import hashlib
import inspect
import json
import os
import subprocess
import sys
import types
from pathlib import Path
from urllib.parse import parse_qs, urlparse

import requests


def ensure_yaml():
    try:
        import yaml  # noqa: F401

        return
    except Exception:
        pass

    yaml_stub = types.ModuleType("yaml")
    yaml_stub.safe_load = lambda *args, **kwargs: {}
    yaml_stub.safe_dump = lambda *args, **kwargs: ""
    sys.modules["yaml"] = yaml_stub


def ensure_markdown():
    try:
        import markdown  # noqa: F401

        return
    except Exception:
        pass

    markdown_stub = types.ModuleType("markdown")
    markdown_stub.markdown = lambda value, *args, **kwargs: value
    sys.modules["markdown"] = markdown_stub


def ensure_cryptography():
    try:
        import cryptography  # noqa: F401

        return
    except Exception:
        pass

    crypto = types.ModuleType("cryptography")
    exceptions = types.ModuleType("cryptography.exceptions")

    class UnsupportedAlgorithm(Exception):
        pass

    exceptions.UnsupportedAlgorithm = UnsupportedAlgorithm

    hazmat = types.ModuleType("cryptography.hazmat")
    backends = types.ModuleType("cryptography.hazmat.backends")
    hazmat.backends = backends

    primitives = types.ModuleType("cryptography.hazmat.primitives")
    asymmetric = types.ModuleType("cryptography.hazmat.primitives.asymmetric")
    hashes = types.ModuleType("cryptography.hazmat.primitives.hashes")
    serialization = types.ModuleType("cryptography.hazmat.primitives.serialization")
    primitives.asymmetric = asymmetric
    primitives.hashes = hashes
    primitives.serialization = serialization

    crypto.exceptions = exceptions
    crypto.hazmat = hazmat
    sys.modules["cryptography"] = crypto
    sys.modules["cryptography.exceptions"] = exceptions
    sys.modules["cryptography.hazmat"] = hazmat
    sys.modules["cryptography.hazmat.backends"] = backends
    sys.modules["cryptography.hazmat.primitives"] = primitives
    sys.modules["cryptography.hazmat.primitives.asymmetric"] = asymmetric
    sys.modules["cryptography.hazmat.primitives.hashes"] = hashes
    sys.modules["cryptography.hazmat.primitives.serialization"] = serialization


ensure_yaml()
ensure_markdown()
ensure_cryptography()

from apprise.common import NotifyFormat, NotifyType

import apprise
from apprise import AppriseAsset

DROP_HEADERS = {"x-apprise-id", "x-apprise-recursion-count"}
KEEP_HEADERS = {"content-type", "accept", "accepts", "authorization"}

BLUESKY_CREATED_AT = "2024-01-01T00:00:00Z"
CACHE_VERSION = 9
CACHE_ENV = "APPRISE_CAPTURE_CACHE"
CACHE_DIR_ENV = "APPRISE_CAPTURE_CACHE_DIR"
CACHE_SUBDIR = ".tmp/pycapture"


def cache_enabled():
    value = os.environ.get(CACHE_ENV, "").strip().lower()
    if value in {"0", "false", "no", "off"}:
        return False
    return True


def find_repo_root(start):
    current = start
    while True:
        if (current / "go.mod").exists() and (current / "internal").is_dir():
            return current
        parent = current.parent
        if parent == current:
            return None
        current = parent


def cache_dir():
    explicit = os.environ.get(CACHE_DIR_ENV, "").strip()
    if explicit:
        return Path(explicit)
    here = Path(__file__).resolve()
    root = find_repo_root(here)
    if root is None:
        return Path.cwd() / CACHE_SUBDIR
    return root / CACHE_SUBDIR


def apprise_repo_root():
    try:
        import apprise as apprise_module

        path = Path(apprise_module.__file__).resolve()
    except Exception:
        return None

    current = path.parent
    while True:
        if (current / ".git").exists():
            return current
        if (current / "pyproject.toml").exists() and (current / "apprise").is_dir():
            return current
        parent = current.parent
        if parent == current:
            return None
        current = parent


def apprise_git_sha():
    root = apprise_repo_root()
    if root is None:
        return ""
    try:
        output = subprocess.check_output(
            ["git", "-C", str(root), "rev-parse", "HEAD"],
            stderr=subprocess.DEVNULL,
        )
    except Exception:
        return ""
    return output.decode("utf-8", "replace").strip()


def cache_key(url, body, title, notify_type, body_format):
    notify_name = notify_type.name if hasattr(notify_type, "name") else str(notify_type)
    try:
        import apprise as apprise_module

        apprise_version = getattr(apprise_module, "__version__", "")
    except Exception:
        apprise_version = ""
    key = {
        "version": CACHE_VERSION,
        "url": url,
        "body": body,
        "title": title,
        "notify_type": notify_name,
        "body_format": body_format,
        "apprise_version": apprise_version,
        "apprise_sha": apprise_git_sha(),
        "python_version": sys.version,
        "requests_version": getattr(requests, "__version__", ""),
        "env": {
            "APPRISE_FIXED_TIME": os.environ.get("APPRISE_FIXED_TIME", ""),
            "APPRISE_OAUTH_NONCE": os.environ.get("APPRISE_OAUTH_NONCE", ""),
            "APPRISE_OAUTH_TIMESTAMP": os.environ.get("APPRISE_OAUTH_TIMESTAMP", ""),
            "APPRISE_VAPID_TEST_JWT": os.environ.get("APPRISE_VAPID_TEST_JWT", ""),
            "APPRISE_VAPID_TEST_PUBLIC_KEY": os.environ.get(
                "APPRISE_VAPID_TEST_PUBLIC_KEY", ""
            ),
            "APPRISE_VAPID_TEST_ENCRYPTED": os.environ.get(
                "APPRISE_VAPID_TEST_ENCRYPTED", ""
            ),
            "APPRISE_SIMPLEPUSH_TEST_IV": os.environ.get(
                "APPRISE_SIMPLEPUSH_TEST_IV", ""
            ),
        },
    }
    payload = json.dumps(key, sort_keys=True, separators=(",", ":")).encode("utf-8")
    # codeql[py/weak-sensitive-data-hashing] - cache key only.
    digest = hashlib.sha256(payload).hexdigest()
    return digest


def load_cache(url, body, title, notify_type, body_format):
    if not cache_enabled():
        return None, None
    digest = cache_key(url, body, title, notify_type, body_format)
    root = cache_dir()
    path = root / f"{digest}.json"
    if not path.exists():
        return None, path
    try:
        raw = path.read_text(encoding="utf-8")
        payload = json.loads(raw)
    except Exception:
        return None, path

    if isinstance(payload, list):
        payload = {"requests": payload, "success": None}
    return payload, path


def store_cache(path, payload):
    if path is None:
        return
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        tmp = path.with_suffix(path.suffix + f".tmp.{os.getpid()}")
        tmp.write_text(json.dumps(payload, separators=(",", ":")), encoding="utf-8")
        os.replace(tmp, path)
    except Exception:
        return


def normalize_headers(headers, keep_user_agent):
    normalized = {}
    for key, value in headers.items():
        if isinstance(value, bytes):
            value = value.decode("utf-8", "replace")
        lower = key.lower()
        if lower in DROP_HEADERS:
            continue
        if lower == "user-agent" and keep_user_agent:
            normalized[lower] = value
            continue
        if lower in KEEP_HEADERS or lower.startswith("x-"):
            normalized[lower] = value
    return normalized


def apply_fixed_time():
    raw = os.environ.get("APPRISE_FIXED_TIME", "").strip()
    if not raw:
        return

    value = raw
    if value.endswith("Z"):
        value = value[:-1] + "+00:00"
    try:
        fixed = datetime.datetime.fromisoformat(value)
    except ValueError:
        return

    if fixed.tzinfo is None:
        fixed = fixed.replace(tzinfo=datetime.timezone.utc)

    class FixedDatetime(datetime.datetime):
        @classmethod
        def now(cls, tz=None):
            if tz is None:
                return fixed.replace(tzinfo=None)
            return fixed.astimezone(tz)

    try:
        import apprise.plugins.ses as ses

        ses.datetime = FixedDatetime
    except Exception:
        pass

    try:
        import apprise.plugins.sns as sns

        sns.datetime = FixedDatetime
    except Exception:
        pass

    try:
        import time as time_module

        time_module.time = lambda: fixed.timestamp()
    except Exception:
        pass

    try:
        import apprise.plugins.dingtalk as dingtalk

        dingtalk.time.time = lambda: fixed.timestamp()
    except Exception:
        pass

    try:
        import apprise.plugins.fcm.oauth as fcm_oauth

        fcm_oauth.datetime = FixedDatetime
    except Exception:
        pass

    try:
        import uuid as uuid_module

        uuid_module.uuid4 = lambda: uuid_module.UUID(
            "00000000-0000-4000-8000-000000000000"
        )
    except Exception:
        pass


def apply_store_fix():
    try:
        from apprise.plugins import N_MGR
    except Exception:
        return

    schemas = list(N_MGR.schemas()) if hasattr(N_MGR, "schemas") else list(N_MGR)
    for schema in schemas:
        plugin = N_MGR[schema]
        prop = inspect.getattr_static(plugin, "url_identifier", None)
        if isinstance(prop, property) and prop.fset is None:
            plugin.url_identifier = property(prop.fget, lambda _self, _value: None)


def apply_oauth_fixes():
    nonce = os.environ.get("APPRISE_OAUTH_NONCE")
    timestamp = os.environ.get("APPRISE_OAUTH_TIMESTAMP")
    if not nonce and not timestamp:
        return

    try:
        from oauthlib.oauth1 import rfc5849
    except Exception:
        return

    if nonce:
        rfc5849.generate_nonce = lambda: nonce
    if timestamp:
        rfc5849.generate_timestamp = lambda: timestamp


def apply_vapid_fixes():
    jwt_token = os.environ.get("APPRISE_VAPID_TEST_JWT")
    public_key = os.environ.get("APPRISE_VAPID_TEST_PUBLIC_KEY")
    encrypted = os.environ.get("APPRISE_VAPID_TEST_ENCRYPTED")

    if encrypted:
        try:
            encrypted_bytes = base64.b64decode(encrypted)
        except Exception:
            encrypted_bytes = None
        if encrypted_bytes is not None:
            try:
                import apprise.utils.pem as pem

                pem.ApprisePEMController.encrypt_webpush = (
                    lambda *_args, **_kwargs: encrypted_bytes
                )
            except Exception:
                pass

    if jwt_token:
        try:
            import apprise.plugins.vapid as vapid

            vapid.NotifyVapid.jwt_token = property(lambda _self: jwt_token)
        except Exception:
            pass

    if public_key:
        try:
            import apprise.plugins.vapid as vapid

            vapid.NotifyVapid.public_key = property(lambda _self: public_key)
        except Exception:
            pass

    try:
        import apprise.plugins.vapid as vapid

        vapid.NotifyVapid.enabled = True
        if hasattr(vapid, "subscription"):
            vapid.subscription.CRYPTOGRAPHY_SUPPORT = True
        if hasattr(vapid, "_pem"):
            vapid._pem.PEM_SUPPORT = True
    except Exception:
        pass


def apply_simplepush_fixes():
    iv_hex = os.environ.get("APPRISE_SIMPLEPUSH_TEST_IV")
    if not iv_hex:
        return
    try:
        iv_bytes = bytes.fromhex(iv_hex)
    except Exception:
        return
    try:
        import apprise.plugins.simplepush as simplepush

        simplepush.urandom = lambda n, _iv=iv_bytes: _iv[:n]
    except Exception:
        pass


def capture_request(url, body, title, notify_type, body_format=None):
    cached, cache_path = load_cache(url, body, title, notify_type, body_format)
    if cached is not None:
        return cached

    apply_fixed_time()
    apply_store_fix()
    apply_oauth_fixes()
    apply_vapid_fixes()
    apply_simplepush_fixes()

    captured = []

    original_request = requests.sessions.Session.request

    def patched_request(self, method, url, **kwargs):
        provided_headers = kwargs.get("headers") or {}
        explicit_user_agent = any(
            header.lower() == "user-agent" for header in provided_headers.keys()
        )

        req = requests.Request(
            method=method,
            url=url,
            headers=provided_headers,
            data=kwargs.get("data"),
            params=kwargs.get("params"),
            json=kwargs.get("json"),
            auth=kwargs.get("auth"),
        )
        prepared = self.prepare_request(req)
        req_body = prepared.body
        if req_body is None:
            body_text = ""
        elif isinstance(req_body, bytes):
            body_text = req_body.decode("utf-8", "replace")
        else:
            body_text = str(req_body)

        parsed = urlparse(prepared.url)
        if parsed.path.endswith("/xrpc/com.atproto.repo.createRecord"):
            try:
                payload = json.loads(body_text)
                record = payload.get("record")
                if isinstance(record, dict):
                    record["createdAt"] = BLUESKY_CREATED_AT
                    body_text = json.dumps(payload)
            except (TypeError, ValueError):
                pass

        captured.append(
            {
                "method": prepared.method,
                "url": prepared.url,
                "headers": normalize_headers(prepared.headers, explicit_user_agent),
                "body": body_text,
            }
        )

        response = requests.Response()
        response.status_code = 200
        hostname = parsed.hostname or ""
        if hostname in ("sendpulse.com", "api.sendpulse.com") and parsed.path == "/oauth/access_token":
            response._content = b'{"access_token":"token","expires_in":3600}'
        elif hostname in ("reddit.com", "www.reddit.com") and parsed.path == "/api/v1/access_token":
            response._content = b'{"access_token":"token","expires_in":3600}'
        elif parsed.netloc == "oauth2.googleapis.com" and parsed.path == "/token":
            response._content = b'{"access_token":"token","expires_in":3600}'
        elif parsed.netloc == "login.microsoftonline.com" and parsed.path.endswith(
            "/oauth2/v2.0/token"
        ):
            response._content = b'{"access_token":"token","expires_in":3600}'
        elif parsed.netloc == "graph.microsoft.com" and parsed.path.startswith(
            "/v1.0/users/"
        ):
            response._content = (
                b'{"mail":"user@example.com","userPrincipalName":"user@example.com",'
                b'"displayName":"Apprise"}'
            )
        elif parsed.netloc == "public.api.bsky.app" and parsed.path.endswith(
            "/xrpc/com.atproto.identity.resolveHandle"
        ):
            response._content = b'{"did":"did:plc:123"}'
        elif parsed.netloc == "plc.directory" and parsed.path.startswith("/did:plc:"):
            response._content = (
                b'{"service":[{"type":"AtprotoPersonalDataServer",'
                b'"serviceEndpoint":"https://bsky.social"}]}'
            )
        elif parsed.path.endswith("/xrpc/com.atproto.server.createSession"):
            response._content = b'{"accessJwt":"token","refreshJwt":"refresh"}'
        elif parsed.path.endswith("/xrpc/com.atproto.repo.createRecord"):
            response._content = b'{"uri":"at://example/post"}'
        elif parsed.netloc == "api.twist.com" and parsed.path.endswith("/users/login"):
            response._content = b'{"token":"token","default_workspace":12345}'
        elif parsed.netloc == "api.twitter.com" and parsed.path.endswith(
            "/account/verify_credentials.json"
        ):
            response._content = b'{"screen_name":"apprise","id":"123","id_str":"123"}'
        elif parsed.netloc == "api.twitter.com" and parsed.path.endswith(
            "/users/lookup.json"
        ):
            values = parse_qs(body_text)
            names = values.get("screen_name") or []
            if not names:
                names = ["user"]
            response._content = json.dumps(
                [{"screen_name": name, "id": "123", "id_str": "123"} for name in names]
            ).encode("utf-8")
        elif parsed.netloc == "slack.com" and parsed.path == "/api/users.lookupByEmail":
            response._content = b'{"ok": true, "user": {"id": "U123"}}'
        elif parsed.netloc == "slack.com" and parsed.path == "/api/chat.postMessage":
            response._content = b'{"ok": true, "ts": "123.456"}'
        elif parsed.path.endswith("/Users/AuthenticateByName"):
            response._content = (
                b'{"AccessToken":"token","Id":"user-id","User":{"Id":"user-id"}}'
            )
        elif parsed.path == "/Sessions":
            response._content = b'[{"Id":"session-id"}]'
        elif parsed.netloc == "api.twist.com" and parsed.path.endswith("/channels/get"):
            response._content = b'[{"id":123,"name":"general","workspace_id":12345}]'
        elif parsed.netloc.startswith("sns.") and "Action=CreateTopic" in body_text:
            response._content = (
                b"<CreateTopicResponse><CreateTopicResult><TopicArn>"
                b"arn:aws:sns:us-east-1:000000000000:topic"
                b"</TopicArn></CreateTopicResult></CreateTopicResponse>"
            )
        elif parsed.path == "/.well-known/matrix/client":
            base_url = f"{parsed.scheme}://{parsed.netloc}"
            response._content = json.dumps(
                {"m.homeserver": {"base_url": base_url}}
            ).encode("utf-8")
        elif parsed.path.endswith("/_matrix/client/versions"):
            response._content = b'{"versions":["r0"]}'
        elif "/_matrix/client/" in parsed.path and parsed.path.endswith("/login"):
            host = parsed.hostname or parsed.netloc.split(":")[0]
            response._content = json.dumps(
                {
                    "access_token": "token",
                    "home_server": host,
                    "user_id": f"@user:{host}",
                }
            ).encode("utf-8")
        elif "/_matrix/client/" in parsed.path and parsed.path.endswith("/register"):
            host = parsed.hostname or parsed.netloc.split(":")[0]
            response._content = json.dumps(
                {
                    "access_token": "token",
                    "home_server": host,
                    "user_id": f"@user:{host}",
                }
            ).encode("utf-8")
        elif "/_matrix/client/" in parsed.path and "/join/" in parsed.path:
            host = parsed.hostname or parsed.netloc.split(":")[0]
            response._content = json.dumps({"room_id": f"!room:{host}"}).encode("utf-8")
        elif "/_matrix/client/" in parsed.path and parsed.path.endswith("/createRoom"):
            host = parsed.hostname or parsed.netloc.split(":")[0]
            response._content = json.dumps(
                {"room_id": f"!room:{host}", "room_alias": f"#room:{host}"}
            ).encode("utf-8")
        elif "/_matrix/client/" in parsed.path and "/directory/room/" in parsed.path:
            host = parsed.hostname or parsed.netloc.split(":")[0]
            response._content = json.dumps({"room_id": f"!room:{host}"}).encode("utf-8")
        elif "/_matrix/client/" in parsed.path and parsed.path.endswith(
            "/joined_rooms"
        ):
            host = parsed.hostname or parsed.netloc.split(":")[0]
            response._content = json.dumps({"joined_rooms": [f"#room:{host}"]}).encode(
                "utf-8"
            )
        elif (
            "/_matrix/client/" in parsed.path and "/send/m.room.message" in parsed.path
        ):
            response._content = b'{"event_id":"$event"}'
        elif "/_matrix/client/" in parsed.path and parsed.path.endswith("/logout"):
            response._content = b"{}"
        elif "/_matrix/media/" in parsed.path and parsed.path.endswith("/upload"):
            host = parsed.hostname or parsed.netloc.split(":")[0]
            response._content = json.dumps({"content_uri": f"mxc://{host}/abc"}).encode(
                "utf-8"
            )
        elif parsed.path.endswith("/api/v1/login"):
            response._content = json.dumps(
                {
                    "status": "success",
                    "data": {"authToken": "token", "userId": "user-id"},
                }
            ).encode("utf-8")
        elif parsed.path.endswith("/api/v1/logout"):
            response._content = b"{}"
        elif (
            parsed.netloc == "wxpusher.zjiecode.com"
            and parsed.path == "/api/send/message"
        ):
            response._content = b'{"code":1000,"msg":"ok"}'
        elif parsed.netloc.endswith("notificationapi.com"):
            response._content = b'{"ok":true}'
        elif parsed.netloc == "api.sendpulse.com" and parsed.path.endswith(
            "/smtp/emails"
        ):
            response._content = b'{"result":true}'
        elif parsed.netloc == "www.pushsafer.com" and parsed.path == "/api":
            response._content = b'{"status":1,"success":"ok"}'
        elif parsed.netloc == "oauth.reddit.com" and parsed.path == "/api/submit":
            response._content = b'{"json":{"errors":[]}}'
        elif parsed.netloc == "api.simplepush.io" and parsed.path == "/send":
            response._content = b'{"status":"OK","message":"OK"}'
        elif parsed.netloc == "voip.ms" and parsed.path == "/api/v1/rest.php":
            response._content = b'{"status":"success","message":"ok"}'
        elif parsed.path.endswith("/api/message"):
            response._content = b'{"result":true}'
        elif parsed.hostname == "www.hampager.de" and parsed.path == "/calls":
            response.status_code = 201
            response._content = b'{"ok":true}'
        elif parsed.netloc == "api.pushy.me" and parsed.path == "/push":
            response._content = b'{"success":true,"id":"id","info":{"devices":1}}'
        elif parsed.path == "/jsonrpc/sms":
            response._content = b'{"result":{"status":"ok"}}'
        elif (
            parsed.netloc in ("api.opsgenie.com", "api.eu.opsgenie.com")
            and parsed.path == "/v2/alerts"
        ):
            response._content = b'{"requestId":"request"}'
        elif parsed.netloc == "www.dmc.sfr-sh.fr" and parsed.path.endswith(
            "/DmcWS/1.5.8/JsonService/MessagesUnitairesWS/addSingleCall"
        ):
            response._content = b'{"success":true}'
        else:
            response._content = b"ok"
        response.url = prepared.url
        return response

    requests.sessions.Session.request = patched_request
    try:
        asset_kwargs = {}
        if body_format:
            asset_kwargs["body_format"] = NotifyFormat(body_format)
        asset = AppriseAsset(**asset_kwargs)
        service = apprise.Apprise(asset=asset)
        service.add(url)
        success = service.notify(
            body=body,
            title=title,
            notify_type=notify_type,
        )
    finally:
        requests.sessions.Session.request = original_request

    payload = {"requests": captured, "success": bool(success)}
    store_cache(cache_path, payload)
    return payload


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", required=True)
    parser.add_argument("--body", default="")
    parser.add_argument("--title", default="")
    parser.add_argument("--type", default="info")
    parser.add_argument("--body-format", default="")
    args = parser.parse_args()

    notify_type = NotifyType.INFO
    if args.type.lower() == "success":
        notify_type = NotifyType.SUCCESS
    elif args.type.lower() == "warning":
        notify_type = NotifyType.WARNING
    elif args.type.lower() == "failure":
        notify_type = NotifyType.FAILURE

    body_format = args.body_format.strip().lower() or None
    payload = capture_request(args.url, args.body, args.title, notify_type, body_format)
    print(json.dumps(payload))


if __name__ == "__main__":
    main()
