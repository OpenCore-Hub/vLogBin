from __future__ import annotations

import hashlib
import hmac
import time
import unittest

from vlogbin import verify_webhook_signature, verify_webhook_signature_within


def sign(secret: str, timestamp: str, payload: bytes) -> str:
    return hmac.new(
        secret.encode("utf-8"),
        timestamp.encode("ascii") + payload,
        hashlib.sha256,
    ).hexdigest()


class WebhookTest(unittest.TestCase):
    def test_verify_webhook_signature(self) -> None:
        secret = "secret"
        timestamp = str(int(time.time()))
        payload = b'{"event_type":"usage.accepted"}'
        signature = sign(secret, timestamp, payload)
        self.assertTrue(verify_webhook_signature(secret, timestamp, payload, signature))
        self.assertFalse(verify_webhook_signature(secret, timestamp, payload, "deadbeef"))

    def test_verify_webhook_signature_within(self) -> None:
        secret = "secret"
        fresh = str(int(time.time()))
        stale = str(int(time.time()) - 600)
        payload = b"{}"
        self.assertTrue(
            verify_webhook_signature_within(
                secret,
                fresh,
                payload,
                sign(secret, fresh, payload),
            )
        )
        self.assertFalse(
            verify_webhook_signature_within(
                secret,
                stale,
                payload,
                sign(secret, stale, payload),
            )
        )
        self.assertFalse(
            verify_webhook_signature_within(secret, "not-a-number", payload, "0")
        )


if __name__ == "__main__":
    unittest.main()

