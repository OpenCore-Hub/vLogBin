#!/usr/bin/env python3
"""Pin and compare deterministic SDK build artifacts."""

from __future__ import annotations

import hashlib
import json
import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
EXPECTED = ROOT / "sdk/generated/artifacts.json"

TS_SOURCE_FILES = [
    ROOT / "sdk/typescript/package.json",
    ROOT / "sdk/typescript/README.md",
    ROOT / "sdk/typescript/tsconfig.json",
]
TS_DIRS = [
    ROOT / "sdk/typescript/src",
    ROOT / "sdk/typescript/test",
    ROOT / "sdk/typescript/dist",
]
PY_SOURCE_FILES = [
    ROOT / "sdk/python/pyproject.toml",
    ROOT / "sdk/python/README.md",
]
PY_DIRS = [ROOT / "sdk/python/vlogbin"]


def file_entries(paths: list[Path], dirs: list[Path]) -> list[dict]:
    entries: list[dict] = []
    candidates: list[Path] = []
    for path in paths:
        if path.exists():
            candidates.append(path)
    for directory in dirs:
        if directory.exists():
            candidates.extend(
                path
                for path in sorted(directory.rglob("*"))
                if path.is_file()
                and "__pycache__" not in path.parts
                and path.suffix not in {".pyc", ".pyo"}
            )
    for path in candidates:
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        relative = path.relative_to(ROOT).as_posix()
        entries.append({"path": relative, "sha256": digest})
    return sorted(entries, key=lambda item: item["path"])


def build_typescript() -> None:
    subprocess.run(
        ["npx", "tsc", "--project", "../../sdk/typescript/tsconfig.json"],
        cwd=ROOT / "apps/web",
        check=True,
    )


def build_artifacts() -> dict:
    build_typescript()
    return {
        "version": 1,
        "typescript": file_entries(TS_SOURCE_FILES, TS_DIRS),
        "python": file_entries(PY_SOURCE_FILES, PY_DIRS),
    }


def diff(current: dict, expected: dict) -> list[str]:
    problems: list[str] = []
    for language in ("typescript", "python"):
        current_map = {item["path"]: item["sha256"] for item in current[language]}
        expected_map = {item["path"]: item["sha256"] for item in expected[language]}
        for path in sorted(expected_map.keys() - current_map.keys()):
            problems.append(f"{language}: missing {path}")
        for path in sorted(current_map.keys() - expected_map.keys()):
            problems.append(f"{language}: unexpected {path}")
        for path in sorted(expected_map.keys() & current_map.keys()):
            if current_map[path] != expected_map[path]:
                problems.append(f"{language}: changed {path}")
    return problems


def main() -> int:
    current = build_artifacts()
    if "--update" in sys.argv:
        EXPECTED.parent.mkdir(parents=True, exist_ok=True)
        EXPECTED.write_text(
            json.dumps(current, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )
        print(f"Wrote {EXPECTED.relative_to(ROOT)}")
        return 0
    if not EXPECTED.exists():
        raise SystemExit(f"{EXPECTED} missing; run with --update to generate")
    expected = json.loads(EXPECTED.read_text(encoding="utf-8"))
    problems = diff(current, expected)
    if problems:
        print("SDK artifact drift:")
        for problem in problems:
            print(f"  {problem}")
        return 1
    print(
        "SDK artifact parity OK "
        f"({len(expected['typescript'])} typescript + {len(expected['python'])} python files)"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
