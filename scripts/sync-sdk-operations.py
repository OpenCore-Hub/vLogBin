#!/usr/bin/env python3
"""Generate the official SDK operation manifest from OpenAPI + operations.yaml."""

from __future__ import annotations

import json
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
OPERATIONS = ROOT / "sdk/operations.yaml"
OPENAPI = ROOT / "docs/openapi.yaml"
OUTPUT = ROOT / "sdk/generated/manifest.json"


def load() -> tuple[dict, dict]:
    operations = yaml.safe_load(OPERATIONS.read_text(encoding="utf-8"))
    openapi = yaml.safe_load(OPENAPI.read_text(encoding="utf-8"))
    return operations, openapi


def build_manifest(operations: dict, openapi: dict) -> dict:
    schemas = openapi.get("components", {}).get("schemas", {})
    entries: list[dict] = []
    raw = operations["operations"]
    for name in sorted(raw):
        op = raw[name]
        method = op["method"].upper()
        path = op["path"]
        operation = openapi.get("paths", {}).get(path, {}).get(method.lower())
        if operation is None:
            raise SystemExit(f"SDK operation {name}: {method} {path} missing from OpenAPI")
        if op.get("request_schema") and op["request_schema"] not in schemas:
            raise SystemExit(f"SDK operation {name}: request schema {op['request_schema']} missing")
        if op.get("response_schema") and op["response_schema"] not in schemas:
            raise SystemExit(f"SDK operation {name}: response schema {op['response_schema']} missing")
        if op.get("idempotency") and operation.get("x-idempotency-key") is not True:
            raise SystemExit(f"SDK operation {name}: OpenAPI must declare x-idempotency-key: true")
        query_parameters = [
            p.get("name")
            for p in operation.get("parameters", [])
            if p.get("in") == "query"
        ]
        for expected in op.get("query_parameters", []):
            if expected not in query_parameters:
                raise SystemExit(f"SDK operation {name}: query parameter {expected} missing")
        entries.append(
            {
                "id": name,
                "method": method,
                "path": path,
                "request_schema": op.get("request_schema"),
                "response_schema": op.get("response_schema"),
                "idempotency": bool(op.get("idempotency", False)),
                "query_parameters": list(op.get("query_parameters", [])),
                "languages": dict(sorted(op.get("languages", {}).items())),
            }
        )
    return {
        "version": int(operations.get("version", 1)),
        "openapi_path": "docs/openapi.yaml",
        "operations": entries,
    }


def main() -> int:
    operations, openapi = load()
    manifest = build_manifest(operations, openapi)
    if "--check" in sys.argv:
        if not OUTPUT.exists():
            raise SystemExit(f"{OUTPUT} missing; run without --check to generate")
        current = json.loads(OUTPUT.read_text(encoding="utf-8"))
        if current != manifest:
            raise SystemExit(f"{OUTPUT} is stale; run without --check to regenerate")
        print(f"SDK operation manifest OK ({len(manifest['operations'])} operations)")
        return 0
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    OUTPUT.write_text(
        json.dumps(manifest, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    print(f"Wrote {OUTPUT.relative_to(ROOT)} ({len(manifest['operations'])} operations)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
