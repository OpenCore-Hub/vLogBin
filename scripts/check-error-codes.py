#!/usr/bin/env python3
"""Check docs/ERROR_CODES.md covers every HTTP error code in the API."""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
HTTPAPI = ROOT / "apps/api/internal/httpapi"
CATALOG = ROOT / "docs/ERROR_CODES.md"

CODE_RE = re.compile(r'writeError\([^)]*"([a-z0-9_]+)"')


def extract_codes() -> set[str]:
    codes: set[str] = set()
    for path in HTTPAPI.rglob("*.go"):
        if path.name.endswith("_test.go"):
            continue
        for line in path.read_text(encoding="utf-8").splitlines():
            if "writeError(" not in line:
                continue
            codes.update(CODE_RE.findall(line))
    return codes


def catalog_codes() -> set[str]:
    text = CATALOG.read_text(encoding="utf-8")
    return set(re.findall(r"^\| `([a-z0-9_]+)`", text, re.MULTILINE))


def main() -> int:
    codes = extract_codes()
    documented = catalog_codes()
    missing = sorted(codes - documented)
    stale = sorted(documented - codes)
    if missing:
        print("Error codes missing from ERROR_CODES.md:")
        for code in missing:
            print(f"  {code}")
    if stale:
        print("Error codes documented but not found in code:")
        for code in stale:
            print(f"  {code}")
    if missing or stale:
        print(f"Coverage: {len(codes - set(missing))}/{len(codes)}")
        return 1
    print(f"Error code catalog OK ({len(codes)} codes)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
