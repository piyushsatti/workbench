#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
bash -n "$ROOT/bootstrap.sh" "$ROOT/verify.sh"
shellcheck "$ROOT/bootstrap.sh" "$ROOT/scripts/packages.sh" "$ROOT/verify.sh"
python3 -m unittest discover -s "$ROOT/tests" -p 'test_*.py'
bash "$ROOT/bootstrap.sh" --help
if bash "$ROOT/bootstrap.sh" --apply; then
  printf 'Obsolete flags must fail rather than silently select a partial installation.\n' >&2
  exit 1
fi
if bash "$ROOT/bootstrap.sh" --profile no-such-profile; then
  printf 'An unknown profile must fail before anything is installed.\n' >&2
  exit 1
fi
python3 "$ROOT/scripts/profile.py" check
printf 'Static and configuration recovery checks passed.\n'
