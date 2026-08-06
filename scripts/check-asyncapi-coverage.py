#!/usr/bin/env python3
"""Check AsyncAPI message coverage against emitted outbox event types."""

from __future__ import annotations

import re
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
ASYNCAPI = ROOT / "docs/asyncapi.yaml"
SERVICE_DIR = ROOT / "apps/api/internal/service"

EVENT_RE = re.compile(r'"(?P<name>[a-z0-9_]+\.[a-z0-9_]+)"')

# Event types built dynamically in catalog.go from domain state names.
DYNAMIC_EVENTS = {
    "catalog.version_validated",
    "catalog.version_published",
    "catalog.version_retired",
}


def extract_event_types() -> set[str]:
    events: set[str] = set()
    for path in SERVICE_DIR.glob("*.go"):
        for line in path.read_text(encoding="utf-8").splitlines():
            if "emitOutbox" not in line:
                continue
            events.update(EVENT_RE.findall(line))
    events.update(DYNAMIC_EVENTS)
    return events


def main() -> int:
    events = extract_event_types()
    with ASYNCAPI.open(encoding="utf-8") as handle:
        spec = yaml.safe_load(handle)
    messages = spec.get("messages", {})
    missing = sorted(events - set(messages))
    no_version = sorted(
        name for name in events if not messages.get(name, {}).get("x-schema-version")
    )
    no_payload = sorted(
        name for name in events if "payload" not in messages.get(name, {})
    )
    no_example = sorted(
        name for name in events if "examples" not in messages.get(name, {})
    )
    problems = False
    if missing:
        print("Outbox event types missing AsyncAPI messages:")
        for name in missing:
            print(f"  {name}")
        problems = True
    if no_version:
        print("Messages missing x-schema-version:")
        for name in no_version:
            print(f"  {name}")
        problems = True
    if no_payload:
        print("Messages missing payload:")
        for name in no_payload:
            print(f"  {name}")
        problems = True
    if no_example:
        print("Messages missing examples:")
        for name in no_example:
            print(f"  {name}")
        problems = True
    if problems:
        print(f"Coverage: {len(events - set(missing))}/{len(events)}")
        return 1
    print(f"AsyncAPI event coverage OK ({len(events)} events)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
