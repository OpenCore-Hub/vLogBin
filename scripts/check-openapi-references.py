#!/usr/bin/env python3
"""Validate $ref references inside docs/openapi.yaml."""

from __future__ import annotations

import sys
from pathlib import Path
from typing import Any

import yaml

ROOT = Path(__file__).resolve().parents[1]
OPENAPI = ROOT / "docs/openapi.yaml"


def walk(value: Any):
    if isinstance(value, dict):
        yield from value.items()
        for child in value.values():
            yield from walk(child)
    elif isinstance(value, list):
        for child in value:
            yield from walk(child)


def main() -> int:
    with OPENAPI.open(encoding="utf-8") as handle:
        spec = yaml.safe_load(handle)
    components = spec.get("components", {})
    missing: list[str] = []
    for key, value in walk(spec):
        if key == "$ref" and isinstance(value, str):
            if value.startswith("#/components/"):
                parts = value.removeprefix("#/components/").split("/")
                node: Any = components
                for part in parts:
                    if not isinstance(node, dict) or part not in node:
                        missing.append(value)
                        break
                    node = node[part]
    if missing:
        print("Missing OpenAPI references:")
        for ref in sorted(set(missing)):
            print(f"  {ref}")
        return 1
    print("OpenAPI reference integrity OK")
    return 0


if __name__ == "__main__":
    sys.exit(main())
