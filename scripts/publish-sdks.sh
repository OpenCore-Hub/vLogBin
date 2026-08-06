#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

MODE="${1:-dry-run}"
case "$MODE" in
  dry-run|publish)
    ;;
  *)
    echo "usage: $0 [dry-run|publish]" >&2
    exit 2
    ;;
esac

make sdk

echo "==> TypeScript package"
if [ "$MODE" = "publish" ]; then
  : "${NPM_TOKEN:?NPM_TOKEN is required for publish}"
  TMP_NPMRC=$(mktemp)
  trap 'rm -f "$TMP_NPMRC"' EXIT
  printf '//registry.npmjs.org/:_authToken=%s\n' "$NPM_TOKEN" > "$TMP_NPMRC"
  npm --userconfig="$TMP_NPMRC" publish ./sdk/typescript --access public
else
  (cd sdk/typescript && npm pack --dry-run)
fi

echo "==> Python package"
if [ "$MODE" = "publish" ]; then
  : "${PYPI_TOKEN:?PYPI_TOKEN is required for publish}"
  python3 -m build sdk/python
  python3 -m twine upload sdk/python/dist/* --username __token__ --password "$PYPI_TOKEN"
else
  echo "Python publish dry-run: run 'python3 -m build sdk/python' locally"
fi

echo "SDK release ${MODE} finished"
