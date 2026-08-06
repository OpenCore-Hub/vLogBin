from __future__ import annotations

import json
import threading
import unittest
from contextlib import contextmanager
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any, Callable, Dict, Iterator
from urllib import parse

from vlogbin import (
    ApiError,
    VLogBinClient,
    create_customer,
    ingest_usage,
    list_subscriptions,
    stream_events,
)


Handler = Callable[[BaseHTTPRequestHandler], None]


@contextmanager
def serving(handler_fn: Handler) -> Iterator[str]:
    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:
            handler_fn(self)

        def do_POST(self) -> None:
            handler_fn(self)

        def log_message(self, format: str, *args: Any) -> None:
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    server.daemon_threads = True
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}"
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=1)


def write_json(handler: BaseHTTPRequestHandler, status: int, body: Dict[str, Any]) -> None:
    raw = json.dumps(body).encode("utf-8")
    handler.send_response(status)
    handler.send_header("Content-Type", "application/json")
    handler.send_header("Content-Length", str(len(raw)))
    handler.end_headers()
    handler.wfile.write(raw)


class ClientTest(unittest.TestCase):
    def test_client_lists_subscriptions(self) -> None:
        def handler(req: BaseHTTPRequestHandler) -> None:
            write_json(
                req,
                200,
                {
                    "subscriptions": [
                        {
                            "id": "sub-1",
                            "external_id": "sub-1",
                            "customer_account_id": "cust-1",
                            "catalog_version_id": "cv-1",
                            "plan_id": "plan-1",
                            "status": "active",
                            "started_at": "2026-01-01T00:00:00Z",
                            "terminated_at": None,
                        }
                    ]
                },
            )

        with serving(handler) as base_url:
            client = VLogBinClient(base_url, "key")
            subscriptions = list_subscriptions(client)
        self.assertEqual(len(subscriptions), 1)
        self.assertEqual(subscriptions[0]["status"], "active")

    def test_client_sends_auth_and_idempotency_headers(self) -> None:
        seen: Dict[str, str] = {}

        def handler(req: BaseHTTPRequestHandler) -> None:
            seen["auth"] = req.headers.get("Authorization", "")
            seen["idempotency"] = req.headers.get("Idempotency-Key", "")
            write_json(req, 200, {"status": "accepted"})

        with serving(handler) as base_url:
            client = VLogBinClient(base_url, "key-1")
            result = ingest_usage(
                client,
                {
                    "transaction_id": "tx-1",
                    "customer_external_id": "cust-1",
                    "metric_code": "api_calls",
                    "timestamp": "2026-01-01T00:00:00Z",
                    "properties": {"count": 1},
                },
                idempotency_key="tx-1",
            )
            self.assertEqual(result["status"], "accepted")
        self.assertEqual(seen["auth"], "Bearer key-1")
        self.assertEqual(seen["idempotency"], "tx-1")

    def test_client_decodes_error_envelope(self) -> None:
        def handler(req: BaseHTTPRequestHandler) -> None:
            write_json(
                req,
                429,
                {
                    "error": {
                        "code": "rate_limited",
                        "message": "slow down",
                        "request_id": "req-1",
                        "retry_after": "5",
                    }
                },
            )

        with serving(handler) as base_url:
            client = VLogBinClient(base_url, "key")
            with self.assertRaises(ApiError) as raised:
                client.request("GET", "/customers")
        err = raised.exception
        self.assertEqual(err.status, 429)
        self.assertEqual(err.code, "rate_limited")
        self.assertEqual(err.request_id, "req-1")
        self.assertEqual(err.retry_after, "5")

    def test_client_creates_customer(self) -> None:
        def handler(req: BaseHTTPRequestHandler) -> None:
            write_json(
                req,
                201,
                {
                    "customer": {
                        "id": "cust-1",
                        "external_id": "acct-123",
                        "account_type": "business",
                        "display_name": "Example Corp",
                    }
                },
            )

        with serving(handler) as base_url:
            client = VLogBinClient(base_url, "key")
            customer = create_customer(
                client,
                {
                    "external_id": "acct-123",
                    "account_type": "business",
                    "display_name": "Example Corp",
                },
            )
        self.assertEqual(customer["id"], "cust-1")
        self.assertEqual(customer["display_name"], "Example Corp")

    def test_client_streams_events_with_filters(self) -> None:
        seen: Dict[str, Dict[str, str]] = {}

        def handler(req: BaseHTTPRequestHandler) -> None:
            seen["query"] = {
                key: values[0] for key, values in parse.parse_qs(parse.urlsplit(req.path).query).items()
            }
            write_json(
                req,
                200,
                {
                    "events": [
                        {
                            "id": "evt-1",
                            "event_type": "usage.accepted",
                            "aggregate_id": "agg-1",
                            "transaction_id": "tx-1",
                            "status": "accepted",
                            "created_at": "2026-01-01T00:00:00Z",
                        }
                    ],
                    "next_cursor": None,
                    "has_more": False,
                },
            )

        with serving(handler) as base_url:
            client = VLogBinClient(base_url, "key")
            page = stream_events(
                client,
                cursor="c1",
                limit=25,
                event_type="usage.accepted",
                aggregate_type="usage",
            )
        self.assertEqual(page["has_more"], False)
        self.assertEqual(seen["query"]["cursor"], "c1")
        self.assertEqual(seen["query"]["limit"], "25")
        self.assertEqual(seen["query"]["type"], "usage.accepted")
        self.assertEqual(seen["query"]["aggregate_type"], "usage")


if __name__ == "__main__":
    unittest.main()
