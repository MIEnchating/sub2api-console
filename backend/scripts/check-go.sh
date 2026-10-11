#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

check_kind="${1:-}"
case "$check_kind" in
  vet|test) shift ;;
  *) printf 'Usage: bash scripts/check-go.sh {vet|test} [package ...]\n' >&2; exit 2 ;;
esac

packages=("$@")
if [[ ${#packages[@]} -eq 0 ]]; then
  # Go's ./... skips directories starting with an underscore. Discover the
  # module-specific regression packages too, including nested browser packages.
  package_list="$(find ./internal -type f -name '*_test.go' -path '*/__tests__/*' -printf '%h\n' | LC_ALL=C sort -u)"
  packages=(./...)
  if [[ -n "$package_list" ]]; then
    mapfile -t regression_packages <<< "$package_list"
    packages+=("${regression_packages[@]}")
  fi
else
  # Scoped checks must name their affected __tests__ packages explicitly.
  printf 'Scoped %s check; only explicitly listed packages are checked.\n' "$check_kind" >&2
fi

case "$check_kind" in
  vet) exec go vet "${packages[@]}" ;;
  # Bound both package and in-package concurrency to reduce race-test contention.
  test) exec go test -p=4 -parallel=4 -race -timeout=20m "${packages[@]}" ;;
esac
