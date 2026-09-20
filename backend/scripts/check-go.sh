#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

# Go's ./... skips directories starting with an underscore. Discover the
# module-specific regression packages too, including nested browser packages.
package_list="$(find ./internal -type f -name '*_test.go' -path '*/__tests__/*' -printf '%h\n' | LC_ALL=C sort -u)"
packages=(./...)
if [[ -n "$package_list" ]]; then
  mapfile -t regression_packages <<< "$package_list"
  packages+=("${regression_packages[@]}")
fi

case "${1:-}" in
  vet) exec go vet "${packages[@]}" ;;
  test) exec go test -race -timeout=20m "${packages[@]}" ;;
  *) printf 'Usage: bash scripts/check-go.sh {vet|test}\n' >&2; exit 2 ;;
esac
