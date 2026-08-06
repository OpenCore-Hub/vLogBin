from __future__ import annotations

import json
from typing import Any, Dict, Optional
from urllib import error, parse, request


class ApiError(Exception):
    """Standardized vLogBin public error envelope."""

    def __init__(
        self,
        status: int,
        code: str,
        message: str,
        request_id: Optional[str] = None,
        retry_after: Optional[str] = None,
        details: Any = None,
    ) -> None:
        super().__init__(message)
        self.status = status
        self.code = code
        self.message = message
        self.request_id = request_id
        self.retry_after = retry_after
        self.details = details

    def __str__(self) -> str:
        if self.request_id:
            return f"{self.code}: {self.message} (request_id={self.request_id})"
        return f"{self.code}: {self.message}"


class VLogBinClient:
    """Minimal, dependency-free client for the vLogBin v1 API."""

    def __init__(
        self,
        base_url: str,
        api_key: str,
        timeout: float = 30.0,
    ) -> None:
        self._base_url = base_url.rstrip("/")
        self._api_key = api_key
        self._timeout = timeout

    def request(
        self,
        method: str,
        path: str,
        *,
        params: Optional[Dict[str, str]] = None,
        idempotency_key: Optional[str] = None,
        body: Any = None,
    ) -> Any:
        url = self._base_url + path
        if params:
            url = f"{url}?{parse.urlencode(params)}"

        headers = {
            "Authorization": f"Bearer {self._api_key}",
            "Content-Type": "application/json",
        }
        if idempotency_key:
            headers["Idempotency-Key"] = idempotency_key

        data = None
        if body is not None:
            data = json.dumps(body, separators=(",", ":")).encode("utf-8")

        req = request.Request(url, data=data, headers=headers, method=method)
        try:
            with request.urlopen(req, timeout=self._timeout) as resp:
                raw = resp.read()
        except error.HTTPError as exc:
            raw = exc.read()
            raise self._decode_error(exc.code, raw) from None

        if not raw:
            return None
        return json.loads(raw.decode("utf-8"))

    def _decode_error(self, status: int, raw: bytes) -> ApiError:
        code = "api_error"
        message = f"request failed with status {status}"
        request_id: Optional[str] = None
        retry_after: Optional[str] = None
        details: Any = None
        try:
            body = json.loads(raw.decode("utf-8"))
            envelope = body.get("error", {})
            code = envelope.get("code") or code
            message = envelope.get("message") or message
            request_id = envelope.get("request_id")
            retry_after = envelope.get("retry_after")
            details = envelope.get("details")
        except (json.JSONDecodeError, UnicodeDecodeError, AttributeError):
            pass
        return ApiError(
            status,
            code,
            message,
            request_id=request_id,
            retry_after=retry_after,
            details=details,
        )

