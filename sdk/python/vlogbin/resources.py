from __future__ import annotations

from typing import Any, Dict, Optional

from .client import VLogBinClient


def create_customer(
    client: VLogBinClient,
    input_data: Dict[str, Any],
    *,
    idempotency_key: Optional[str] = None,
) -> Dict[str, Any]:
    out = client.request(
        "POST",
        "/customers",
        idempotency_key=idempotency_key,
        body=input_data,
    )
    return out["customer"]


def ingest_usage(
    client: VLogBinClient,
    input_data: Dict[str, Any],
    *,
    idempotency_key: Optional[str] = None,
) -> Dict[str, Any]:
    return client.request(
        "POST",
        "/usage/ingest",
        idempotency_key=idempotency_key,
        body=input_data,
    )


def stream_events(
    client: VLogBinClient,
    *,
    cursor: Optional[str] = None,
    limit: int = 100,
    event_type: Optional[str] = None,
    aggregate_type: Optional[str] = None,
) -> Dict[str, Any]:
    params: Dict[str, str] = {"limit": str(limit)}
    if cursor:
        params["cursor"] = cursor
    if event_type:
        params["type"] = event_type
    if aggregate_type:
        params["aggregate_type"] = aggregate_type
    return client.request("GET", "/events", params=params)

