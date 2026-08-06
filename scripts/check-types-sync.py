#!/usr/bin/env python3
"""Check that every exported TS API interface has an OpenAPI schema."""

from __future__ import annotations

import re
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
TYPES = ROOT / "apps/web/src/lib/api/types.ts"
OPENAPI = ROOT / "docs/openapi.yaml"

INTERFACE_RE = re.compile(r"^export interface (\w+)", re.MULTILINE)


def main() -> int:
    source = TYPES.read_text(encoding="utf-8")
    interfaces = set(INTERFACE_RE.findall(source))
    with OPENAPI.open(encoding="utf-8") as handle:
        spec = yaml.safe_load(handle)
    schemas = set(spec.get("components", {}).get("schemas", {}))
    missing = sorted(interfaces - schemas)
    if missing:
        print("TS interfaces missing OpenAPI schemas:")
        for name in missing:
            print(f"  {name}")
        return 1
    print(f"Type/schema name sync OK ({len(interfaces)} interfaces)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
