#!/usr/bin/env python3
"""Compare chi route registrations with docs/openapi.yaml.

Exits non-zero when a public HTTP route is undocumented or a documented route
no longer exists. Infra endpoints (/metrics, /health, /ready, /startup) are
excluded; every /v1 public contract route must be documented.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
OPENAPI = ROOT / "docs/openapi.yaml"
ROUTER = ROOT / "apps/api/internal/httpapi/httpapi.go"

EXCLUDE = {"/metrics", "/health", "/ready", "/startup"}

METHOD_RE = re.compile(r'\.(Get|Post|Put|Patch|Delete)\("([^"]+)"')
ROUTE_RE = re.compile(r'r\.Route\("([^"]+)", func\(r chi\.Router\) \{')


def normalize(path: str) -> str:
    normalized = re.sub(r"\{[^}]+\}", "{}", path)
    return normalized.rstrip("/") or "/"


def extract_route_paths() -> list[tuple[str, str]]:
    routes: list[tuple[str, str]] = []
    stack: list[tuple[str, int]] = []
    for line in ROUTER.read_text(encoding="utf-8").splitlines():
        indent = len(line) - len(line.lstrip(" \t"))
        is_close = line.strip() == "})"
        while stack and (indent < stack[-1][1] or (is_close and indent <= stack[-1][1])):
            stack.pop()
        group = ROUTE_RE.search(line)
        if group:
            stack.append((group.group(1), indent))
            continue
        for match in METHOD_RE.finditer(line):
            method = match.group(1).upper()
            path = match.group(2)
            full = "".join(prefix for prefix, _ in stack) + path
            if full in EXCLUDE or not (full.startswith("/v1") or full.startswith("/scim/v2")):
                continue
            routes.append((method, full.removeprefix("/v1")))
    return routes


def extract_routes() -> set[tuple[str, str]]:
    return {(method, normalize(path)) for method, path in extract_route_paths()}


def extract_documented() -> set[tuple[str, str]]:
    with OPENAPI.open(encoding="utf-8") as handle:
        spec = yaml.safe_load(handle)
    documented: set[tuple[str, str]] = set()
    for raw_path, operations in spec.get("paths", {}).items():
        if not isinstance(operations, dict):
            continue
        for method in ("get", "post", "put", "patch", "delete"):
            if method in operations:
                documented.add((method.upper(), normalize(raw_path)))
    return documented


def main() -> int:
    routes = extract_routes()
    documented = extract_documented()
    missing = sorted(routes - documented)
    stale = sorted(documented - routes)
    if missing:
        print("Undocumented routes:")
        for method, path in missing:
            print(f"  {method} {path}")
    if stale:
        print("Documented routes missing from implementation:")
        for method, path in stale:
            print(f"  {method} {path}")
    if missing or stale:
        print(f"Coverage: {len(routes) - len(missing)}/{len(routes)}")
        return 1
    print(f"OpenAPI route coverage OK ({len(routes)} routes)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
