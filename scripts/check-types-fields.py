#!/usr/bin/env python3
"""Compare TS interface property names with OpenAPI object schema properties."""

from __future__ import annotations

import re
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
TYPES = ROOT / "apps/web/src/lib/api/types.ts"
OPENAPI = ROOT / "docs/openapi.yaml"

INTERFACE_RE = re.compile(
    r"^export interface (\w+) \{(?P<body>.*?)^\}",
    re.MULTILINE | re.DOTALL,
)
PROPERTY_RE = re.compile(r"^\s+(\w+)\??:", re.MULTILINE)

INTENTIONAL = {
    "UsageEvent": "UsageEventRecord",
}


def main() -> int:
    source = TYPES.read_text(encoding="utf-8")
    with OPENAPI.open(encoding="utf-8") as handle:
        spec = yaml.safe_load(handle)
    schemas = spec.get("components", {}).get("schemas", {})
    problems: list[str] = []
    for match in INTERFACE_RE.finditer(source):
        name = match.group(1)
        ts_fields = set(PROPERTY_RE.findall(match.group("body")))
        schema = schemas.get(INTENTIONAL.get(name, name))
        if schema is None:
            continue
        if schema.get("type") != "object" or not isinstance(schema.get("properties"), dict):
            continue
        openapi_fields = set(schema["properties"])
        if ts_fields != openapi_fields:
            missing = sorted(ts_fields - openapi_fields)
            extra = sorted(openapi_fields - ts_fields)
            problems.append(f"{name}: missing={missing} extra={extra}")
    if problems:
        print("Field drift between types.ts and OpenAPI:")
        for problem in problems:
            print(f"  {problem}")
        return 1
    print("Type/schema field sync OK")
    return 0


if __name__ == "__main__":
    sys.exit(main())
