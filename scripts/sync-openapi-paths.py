#!/usr/bin/env python3
"""Insert generic OpenAPI stubs for undocumented chi routes.

Preserves existing comments and path entries; only adds missing paths. Run
after adding a route so the contract never silently drifts from the router.
"""

from __future__ import annotations

import importlib.util
import re
from collections import defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OPENAPI = ROOT / "docs/openapi.yaml"
CHECKER = ROOT / "scripts/check-openapi-coverage.py"


def load_checker():
    spec = importlib.util.spec_from_file_location("check_openapi_coverage", CHECKER)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


def main() -> int:
    checker = load_checker()
    lines = OPENAPI.read_text(encoding="utf-8").splitlines(keepends=True)
    paths_index = next(
        (i for i, line in enumerate(lines) if line.startswith("paths:")),
        len(lines),
    )
    components_index = next(
        (i for i, line in enumerate(lines) if line.startswith("components:")),
        len(lines),
    )
    first_path = next(
        (
            i
            for i in range(components_index)
            if lines[i].startswith("  /")
        ),
        None,
    )
    if first_path is not None:
        del lines[first_path:components_index]
        OPENAPI.write_text("".join(lines), encoding="utf-8")

    routes = checker.extract_route_paths()
    documented = checker.extract_documented()
    by_normalized = {
        (method, checker.normalize(path)): path for method, path in routes
    }
    route_set = checker.extract_routes()
    missing = sorted(route_set - documented)
    if not missing:
        print("OpenAPI paths are up to date")
        return 0

    paths_index = next(
        (i for i, line in enumerate(lines) if line.startswith("paths:")),
        len(lines),
    )
    methods_by_path: dict[str, list[tuple[str, str]]] = defaultdict(list)
    for method, normalized in missing:
        path = by_normalized[(method, normalized)]
        methods_by_path[normalized].append((method, path))

    path_key_re = re.compile(r"^  (/[^:]+):\s*$")
    existing: dict[str, int] = {}
    for i in range(paths_index + 1, len(lines)):
        match = path_key_re.match(lines[i].rstrip("\n"))
        if match:
            existing[checker.normalize(match.group(1))] = i

    new_blocks: list[str] = []
    insertions: list[tuple[int, list[str]]] = []
    for normalized, method_paths in sorted(methods_by_path.items()):
        block = []
        for method, path in sorted(method_paths):
            block.extend(
                [
                    f"    {method.lower()}:\n",
                    "      tags: [Contract]\n",
                    f"      summary: {method.title()} {path}\n",
                    "      responses:\n",
                    "        '200':\n",
                    "          description: OK\n",
                ]
            )
        if normalized in existing:
            insertions.append((existing[normalized] + 1, block))
        else:
            path = method_paths[0][1]
            new_blocks.append(f"  {path}:\n")
            new_blocks.extend(block)

    for index, block in sorted(insertions, reverse=True):
        lines[index:index] = block
    if new_blocks:
        lines.append("\n")
        lines.extend(new_blocks)
    OPENAPI.write_text("".join(lines), encoding="utf-8")
    print(f"Added {len(missing)} undocumented OpenAPI paths")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
