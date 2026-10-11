"""Independent Olm/Megolm receiver for captured request parity.

Uses cryptography primitives, never the sender's Apprise or Go crypto routines.
Only the disposable receiver's keys are fixed. Sender keys, authenticated room
key sharing and message ciphertext must verify and decrypt before comparison.
Wire formats: https://spec.matrix.org/unstable/olm-megolm/
"""
import base64
import copy
import hashlib
import hmac
import json
import sys
from typing import Any
from urllib.parse import urlparse, unquote

from cryptography.hazmat.primitives import hashes, padding
from cryptography.hazmat.primitives.asymmetric import ed25519, x25519
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes
from cryptography.hazmat.primitives.kdf.hkdf import HKDF
from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat

RECEIVER_IDENTITY = bytes(200-i for i in range(32))
RECEIVER_SIGNING = bytes(255-i for i in range(32))
RECEIVER_OTK = bytes(50+i for i in range(32))
RECEIVER_USER = "@target:example.com"
RECEIVER_DEVICE = "TARGETDEVICE"


def b64(value: str) -> bytes:
    return base64.b64decode(value + "=" * (-len(value) % 4), validate=True)


def public(
    seed: bytes,
    kind: type[x25519.X25519PrivateKey] | type[ed25519.Ed25519PrivateKey] = x25519.X25519PrivateKey,
) -> bytes:
    return kind.from_private_bytes(seed).public_key().public_bytes(Encoding.Raw, PublicFormat.Raw)


def canonical(value: Any) -> bytes:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()


def require(condition: object, message: str) -> None:
    if not condition:
        raise ValueError(message)


def signed_object(value: dict[str, Any], user: str, device: str, signing: bytes) -> None:
    require(set(value["signatures"]) == {user} and set(value["signatures"][user]) == {"ed25519:"+device}, "unexpected signatures")
    signature = value["signatures"][user]["ed25519:"+device]
    unsigned = {k:v for k,v in value.items() if k not in ("signatures", "unsigned")}
    ed25519.Ed25519PublicKey.from_public_bytes(signing).verify(b64(signature), canonical(unsigned))


def varint(data: bytes, offset: int) -> tuple[int, int]:
    value = shift = 0
    while offset < len(data) and shift < 64:
        byte = data[offset]
        offset += 1
        value |= (byte & 127) << shift
        if byte < 128:
            return value, offset
        shift += 7
    raise ValueError("invalid protobuf integer")


def fields(data: bytes) -> dict[int, int | bytes]:
    require(data and data[0] == 3, "invalid encrypted message version")
    result, offset = {}, 1
    while offset < len(data):
        tag, offset = varint(data, offset)
        key, wire = tag >> 3, tag & 7
        require(key not in result, "duplicate protobuf field")
        value, offset = varint(data, offset)
        if wire == 2:
            require(offset+value <= len(data), "truncated protobuf field")
            value, offset = data[offset:offset+value], offset+value
        else:
            require(wire == 0, "unsupported protobuf field")
        result[key] = value
    return result


def hkdf(key: bytes, info: bytes, salt: bytes | None = None, length: int = 80) -> bytes:
    return HKDF(algorithm=hashes.SHA256(), length=length, salt=salt, info=info).derive(key)


def decrypt_cbc(ciphertext: bytes, keys: bytes) -> bytes:
    decryptor = Cipher(algorithms.AES(keys[:32]), modes.CBC(keys[64:80])).decryptor()
    plaintext = decryptor.update(ciphertext)+decryptor.finalize()
    unpadder = padding.PKCS7(128).unpadder()
    return unpadder.update(plaintext)+unpadder.finalize()


def decrypt_olm(message: str, identity: bytes) -> dict[str, Any]:
    outer = fields(b64(message))
    require(set(outer) == {1,2,3,4}, "unexpected pre-key fields")
    require(outer[1] == public(RECEIVER_OTK), "wrong recipient one-time key")
    require(outer[3] == identity, "wrong sender identity")
    ephemeral = x25519.X25519PublicKey.from_public_bytes(outer[2])
    sender = x25519.X25519PublicKey.from_public_bytes(identity)
    recipient_otk = x25519.X25519PrivateKey.from_private_bytes(RECEIVER_OTK)
    recipient_identity = x25519.X25519PrivateKey.from_private_bytes(RECEIVER_IDENTITY)
    secret = recipient_otk.exchange(sender)+recipient_identity.exchange(ephemeral)+recipient_otk.exchange(ephemeral)
    chain = hkdf(secret, b"OLM_ROOT", length=64)[32:]
    key = hmac.digest(chain, b"\x01", "sha256")
    keys = hkdf(key, b"OLM_KEYS", bytes(32))
    inner = outer[4]
    require(hmac.compare_digest(hmac.digest(keys[32:64], inner[:-8], "sha256")[:8], inner[-8:]), "invalid Olm MAC")
    parsed = fields(inner[:-8])
    require(set(parsed) == {1,2,4} and parsed[1] == outer[2] and parsed[2] == 0, "invalid Olm ratchet")
    return json.loads(decrypt_cbc(parsed[4], keys))


def decrypt_megolm(message: str, session_key: str, session_id: str) -> dict[str, Any]:
    key = b64(session_key)
    require(len(key) == 229 and key[0] == 2, "invalid Megolm session key")
    signing = key[133:165]
    require(signing == b64(session_id), "wrong Megolm session ID")
    ed25519.Ed25519PublicKey.from_public_bytes(signing).verify(key[165:], key[:165])
    counter = int.from_bytes(key[1:5], "big")
    ratchet = [key[5+32*i:37+32*i] for i in range(4)]
    raw = b64(message)
    require(len(raw) > 73, "truncated Megolm message")
    ed25519.Ed25519PublicKey.from_public_bytes(signing).verify(raw[-64:], raw[:-64])
    parsed = fields(raw[:-72])
    require(set(parsed) == {1,2}, "unexpected Megolm fields")
    index = parsed[1]
    require(counter <= index < counter+4096, "invalid Megolm message index")
    while counter < index:
        next_counter = counter+1
        high = 0 if next_counter % (1<<24) == 0 else 1 if next_counter % (1<<16) == 0 else 2 if next_counter % (1<<8) == 0 else 3
        source = ratchet[high]
        for i in range(high, 4):
            ratchet[i] = hmac.digest(source, bytes([i]), "sha256")
        counter = next_counter
    keys = hkdf(b"".join(ratchet), b"MEGOLM_KEYS")
    require(hmac.compare_digest(hmac.digest(keys[32:64], raw[:-72], "sha256")[:8], raw[-72:-64]), "invalid Megolm MAC")
    return json.loads(decrypt_cbc(parsed[2], keys))


def normalize_requests(requests: list[dict[str, Any]]) -> list[dict[str, Any]]:
    normalized = copy.deepcopy(requests)
    identity = signing = user = device = None
    sessions = {}
    for request in normalized:
        path = unquote(urlparse(request["url"]).path)
        if path.endswith("/keys/upload"):
            payload = json.loads(request["body"])
            require(set(payload) == {"device_keys", "one_time_keys", "fallback_keys"}, "incomplete key upload")
            keys = payload["device_keys"]
            user, device = keys["user_id"], keys["device_id"]
            identity, signing = b64(keys["keys"]["curve25519:"+device]), b64(keys["keys"]["ed25519:"+device])
            require(set(keys["keys"]) == {"curve25519:"+device, "ed25519:"+device}, "unexpected device keys")
            require(len(identity) == len(signing) == 32, "invalid device key length")
            require(keys["algorithms"] == ["m.olm.v1.curve25519-aes-sha2", "m.megolm.v1.aes-sha2"], "wrong device algorithms")
            signed_object(keys, user, device, signing)
            for name, count in (("one_time_keys", 10), ("fallback_keys", 1)):
                objects = payload[name]
                require(len(objects) == count, "wrong "+name+" count")
                public_keys = set()
                for key_id, value in objects.items():
                    require(set(value) in ({"key", "signatures"}, {"key", "signatures", "fallback"}), "unexpected curve key fields")
                    require(key_id.startswith("signed_curve25519:"), "unsigned curve key")
                    signed_object(value, user, device, signing)
                    decoded = b64(value["key"])
                    require(len(decoded) == 32 and decoded not in public_keys, "duplicate or invalid curve key")
                    # Apprise 1.13.1 omits the optional marker; the containing
                    # fallback_keys map still identifies the signed fallback.
                    if name == "fallback_keys":
                        require("fallback" not in value or value["fallback"] is True, "invalid fallback marker")
                    else:
                        require("fallback" not in value, "one-time key marked as fallback")
                    public_keys.add(decoded)
                payload[name] = {"verified_keys":count}
            keys["keys"] = {"curve25519:"+device:"verified", "ed25519:"+device:"verified"}
            keys["signatures"] = {user:{"ed25519:"+device:"verified"}}
            request["body"] = json.dumps(payload)
        elif "/sendToDevice/m.room.encrypted/" in path:
            require(identity is not None, "room key shared before device keys")
            payload = json.loads(request["body"])
            require(set(payload) == {"messages"}, "unexpected room-key payload")
            require(set(payload["messages"]) == {RECEIVER_USER}, "wrong room-key recipient")
            messages = payload["messages"][RECEIVER_USER]
            require(set(messages) == {RECEIVER_DEVICE}, "wrong room-key device")
            envelope = messages[RECEIVER_DEVICE]
            require(set(envelope) == {"algorithm", "ciphertext", "sender_key"}, "unexpected room-key envelope")
            require(envelope["algorithm"] == "m.olm.v1.curve25519-aes-sha2" and b64(envelope["sender_key"]) == identity, "invalid room-key envelope")
            require(len(envelope["ciphertext"]) == 1, "wrong ciphertext recipient count")
            recipient_key, ciphertext = next(iter(envelope["ciphertext"].items()))
            require(set(ciphertext) == {"body", "type"}, "unexpected Olm ciphertext fields")
            require(b64(recipient_key) == public(RECEIVER_IDENTITY) and ciphertext["type"] == 0, "wrong ciphertext recipient")
            event = decrypt_olm(ciphertext["body"], identity)
            require(set(event) == {"type", "content", "sender", "recipient", "recipient_keys", "keys"}, "unexpected room-key event fields")
            require(event["type"] == "m.room_key" and event["sender"] == user and event["recipient"] == RECEIVER_USER, "invalid room-key event")
            require(b64(event["keys"]["ed25519"]) == signing and b64(event["recipient_keys"]["ed25519"]) == public(RECEIVER_SIGNING, ed25519.Ed25519PrivateKey), "wrong signing-key binding")
            room_key = event["content"]
            require(set(room_key) == {"algorithm", "room_id", "session_id", "session_key"}, "unexpected session-key fields")
            require(room_key["algorithm"] == "m.megolm.v1.aes-sha2", "wrong room-key algorithm")
            sessions[room_key["session_id"]] = (room_key["session_key"], room_key["room_id"])
            event["keys"] = {"ed25519":"verified"}
            event["recipient_keys"] = {"ed25519":"verified"}
            room_key["session_id"] = room_key["session_key"] = "verified"
            request["body"] = json.dumps({"messages":{RECEIVER_USER:{RECEIVER_DEVICE:event}}})
        elif "/send/m.room.encrypted/" in path:
            require(identity is not None, "encrypted message before device keys")
            payload = json.loads(request["body"])
            require(set(payload) == {"algorithm", "ciphertext", "sender_key", "session_id", "device_id"}, "unexpected encrypted fields")
            require(payload["algorithm"] == "m.megolm.v1.aes-sha2" and payload["device_id"] == device and b64(payload["sender_key"]) == identity, "wrong encrypted-message binding")
            session_key, room = sessions[payload["session_id"]]
            event = decrypt_megolm(payload["ciphertext"], session_key, payload["session_id"])
            require(event["room_id"] == room and "/rooms/"+room+"/send/" in path and event["type"] == "m.room.message", "wrong encrypted room")
            request["body"] = json.dumps({"algorithm":payload["algorithm"], "device_id":device, "event":event})
    return normalized


if __name__ == "__main__":
    print(json.dumps(normalize_requests(json.loads(sys.argv[1]))))
