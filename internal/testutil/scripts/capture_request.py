import argparse
import base64
import datetime
import inspect
import json
import os
import sys
import types
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

from apprise.common import NotifyType

import apprise
from apprise import AppriseAsset

DROP_HEADERS = {"x-apprise-id", "x-apprise-recursion-count"}
KEEP_HEADERS = {"content-type", "accept", "accepts", "authorization"}

BLUESKY_CREATED_AT = "2024-01-01T00:00:00Z"


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


def capture_request(url, body, title, notify_type):
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

        if "/xrpc/com.atproto.repo.createRecord" in prepared.url:
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
        parsed = urlparse(prepared.url)
        if "sendpulse.com/oauth/access_token" in prepared.url:
            response._content = b'{"access_token":"token","expires_in":3600}'
        elif "reddit.com/api/v1/access_token" in prepared.url:
            response._content = b'{"access_token":"token","expires_in":3600}'
        elif "login.microsoftonline.com" in prepared.url and parsed.path.endswith(
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
        else:
            response._content = b"ok"
        response.url = prepared.url
        return response

    requests.sessions.Session.request = patched_request
    try:
        asset = AppriseAsset()
        service = apprise.Apprise(asset=asset)
        service.add(url)
        service.notify(
            body=body,
            title=title,
            notify_type=notify_type,
        )
    finally:
        requests.sessions.Session.request = original_request

    return captured


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", required=True)
    parser.add_argument("--body", default="")
    parser.add_argument("--title", default="")
    parser.add_argument("--type", default="info")
    args = parser.parse_args()

    notify_type = NotifyType.INFO
    if args.type.lower() == "success":
        notify_type = NotifyType.SUCCESS
    elif args.type.lower() == "warning":
        notify_type = NotifyType.WARNING
    elif args.type.lower() == "failure":
        notify_type = NotifyType.FAILURE

    specs = capture_request(args.url, args.body, args.title, notify_type)
    print(json.dumps(specs))


if __name__ == "__main__":
    main()
