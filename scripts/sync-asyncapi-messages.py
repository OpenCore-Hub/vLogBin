#!/usr/bin/env python3
"""Insert AsyncAPI message stubs for emitted outbox event types."""

from __future__ import annotations

import importlib.util
import re
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
ASYNCAPI = ROOT / "docs/asyncapi.yaml"
CHECKER = ROOT / "scripts/check-asyncapi-coverage.py"


def load_checker():
    spec = importlib.util.spec_from_file_location("check_asyncapi_coverage", CHECKER)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


def title(name: str) -> str:
    return " ".join(part.replace("_", " ").title() for part in name.split("."))


def main() -> int:
    checker = load_checker()
    events = checker.extract_event_types()
    with ASYNCAPI.open(encoding="utf-8") as handle:
        spec = yaml.safe_load(handle)
    existing = set(spec.get("messages", {}))
    missing = sorted(events - existing)
    missing_versions = sorted(
        name for name in events if not spec.get("messages", {}).get(name, {}).get("x-schema-version")
    )
    missing_examples = sorted(
        name for name in events if "examples" not in spec.get("messages", {}).get(name, {})
    )

    blocks: list[str] = []
    for name in missing:
        blocks.extend(
            [
                f"  {name}:\n",
                f"    name: {name}\n",
                f"    title: {title(name)}\n",
                f"    summary: Outbox event emitted by vLogBin\n",
                "    contentType: application/json\n",
                '    x-schema-version: "1.0"\n',
                "    payload:\n",
                "      type: object\n",
                "      required: [id, event_type, timestamp]\n",
                "      properties:\n",
                "        id:\n",
                "          type: string\n",
                "          format: uuid\n",
                "        provider_id:\n",
                "          type: string\n",
                "          format: uuid\n",
                "          nullable: true\n",
                "        environment_id:\n",
                "          type: string\n",
                "          format: uuid\n",
                "          nullable: true\n",
                "        event_type:\n",
                "          type: string\n",
                "        timestamp:\n",
                "          type: string\n",
                "          format: date-time\n",
                "    examples:\n",
                "      - value:\n",
                '          id: "00000000-0000-0000-0000-000000000000"\n',
                f'          event_type: "{name}"\n',
                '          timestamp: "2026-01-01T00:00:00Z"\n',
            ]
        )
    text = ASYNCAPI.read_text(encoding="utf-8").rstrip() + "\n\n" + "".join(blocks)
    lines = text.splitlines(keepends=True)
    for name in missing_versions:
        start = next(
            i for i, line in enumerate(lines) if line.strip() == f"{name}:"
        )
        for j in range(start, len(lines)):
            if lines[j].lstrip().startswith("contentType:"):
                lines.insert(j + 1, '    x-schema-version: "1.0"\n')
                break
    for name in missing_examples:
        start = next(
            i for i, line in enumerate(lines) if line.strip() == f"{name}:"
        )
        end = next(
            (
                i
                for i in range(start + 1, len(lines))
                if re.match(r"^  [^\s]+:", lines[i])
            ),
            len(lines),
        )
        example = [
            "    examples:\n",
            "      - value:\n",
            '          id: "00000000-0000-0000-0000-000000000000"\n',
            f'          event_type: "{name}"\n',
            '          timestamp: "2026-01-01T00:00:00Z"\n',
        ]
        lines[end:end] = example
    text = "".join(lines)
    ASYNCAPI.write_text(text, encoding="utf-8")
    print(
        f"Added {len(missing)} stubs, {len(missing_versions)} schema versions, "
        f"{len(missing_examples)} examples"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
