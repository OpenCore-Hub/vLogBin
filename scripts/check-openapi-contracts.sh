#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

python3 scripts/check-openapi-coverage.py
python3 scripts/check-openapi-references.py
python3 scripts/check-types-sync.py
python3 scripts/check-types-fields.py
python3 scripts/check-error-codes.py
