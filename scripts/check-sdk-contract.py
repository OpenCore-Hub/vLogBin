#!/usr/bin/env python3
"""Verify that every generated SDK operation exists in all three clients."""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT / "sdk/generated/manifest.json"

SOURCES = {
    "go": [
        ROOT / "sdk/go/resources.go",
        ROOT / "sdk/go/client.go",
    ],
    "typescript": [
        ROOT / "sdk/typescript/src/resources.ts",
    ],
    "python": [
        ROOT / "sdk/python/vlogbin/resources.py",
    ],
}

SYMBOL_RE = {
    "go": re.compile(r"func \(c \*Client\) (\w+)\("),
    "typescript": re.compile(r"export function (\w+)\("),
    "python": re.compile(r"^def (\w+)\(", re.MULTILINE),
}

IDEMPOTENCY_MARKERS = {
    "go": "IdempotencyKey",
    "typescript": "idempotencyKey",
    "python": "idempotency_key",
}


def main() -> int:
    manifest = json.loads(MANIFEST.read_text(encoding="utf-8"))
    sources = {
        name: "\n".join(path.read_text(encoding="utf-8") for path in paths)
        for name, paths in SOURCES.items()
    }
    problems: list[str] = []
    for operation in manifest["operations"]:
        for language, symbol in operation["languages"].items():
            if language not in SYMBOL_RE:
                problems.append(f"{operation['id']}: unknown language {language}")
                continue
            if not SYMBOL_RE[language].search(sources[language]):
                problems.append(f"{operation['id']}: missing {symbol} in {language}")
                continue
            if operation["path"] not in sources[language]:
                problems.append(f"{operation['id']}: path {operation['path']} missing in {language}")
        if operation["idempotency"]:
            for language, marker in IDEMPOTENCY_MARKERS.items():
                if marker not in sources[language]:
                    problems.append(f"{operation['id']}: idempotency marker missing in {language}")
        if operation["query_parameters"]:
            for parameter in operation["query_parameters"]:
                if parameter not in sources["typescript"]:
                    problems.append(f"{operation['id']}: query parameter {parameter} missing in typescript")
                if parameter not in sources["python"]:
                    problems.append(f"{operation['id']}: query parameter {parameter} missing in python")
                if parameter not in sources["go"]:
                    problems.append(f"{operation['id']}: query parameter {parameter} missing in go")
    if problems:
        print("SDK contract drift:")
        for problem in problems:
            print(f"  {problem}")
        return 1
    language_count = len(SOURCES)
    operation_count = len(manifest["operations"])
    print(f"SDK contract parity OK ({operation_count} operations x {language_count} languages)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
