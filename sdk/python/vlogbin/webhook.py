from __future__ import annotations

import hashlib
import hmac
import time
from typing import Union


def verify_webhook_signature(
    secret: str,
    timestamp: str,
    payload: Union[str, bytes],
    signature: str,
) -> bool:
    if isinstance(payload, str):
        payload = payload.encode("utf-8")
    expected = hmac.new(
        secret.encode("utf-8"),
        timestamp.encode("ascii") + payload,
        hashlib.sha256,
    ).hexdigest()
    return hmac.compare_digest(expected, signature.lower())


def verify_webhook_signature_within(
    secret: str,
    timestamp: str,
    payload: Union[str, bytes],
    signature: str,
    max_age_ms: int = 5 * 60 * 1000,
) -> bool:
    try:
        seconds = int(timestamp, 10)
    except ValueError:
        return False
    age_ms = (time.time() - seconds) * 1000
    if age_ms > max_age_ms or age_ms < -max_age_ms:
        return False
    return verify_webhook_signature(secret, timestamp, payload, signature)

