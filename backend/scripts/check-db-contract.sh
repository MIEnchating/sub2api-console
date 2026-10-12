#!/usr/bin/env bash
# Isolated database dependencies only; Go tests execute on the host.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
for port in 25432 23306; do
  if ss -H -ltn "sport = :$port" | read -r _; then
    printf 'Contract-test port %s is occupied; refusing to reuse an unknown service.\n' "$port" >&2
    exit 1
  fi
done
export CONSOLE_DB_CONTRACT_PASSWORD
CONSOLE_DB_CONTRACT_PASSWORD="$(openssl rand -hex 24)"
export CONSOLE_DB_CONTRACT_TEST=1
pg_container="console-db-contract-pg-$$"
mysql_container="console-db-contract-mysql-$$"
created=()
cleanup() {
  if (( ${#created[@]} )); then docker rm -fv "${created[@]}" >/dev/null; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
export POSTGRES_PASSWORD="$CONSOLE_DB_CONTRACT_PASSWORD"
docker run --detach --name "$pg_container" --label console.purpose=db-contract \
  -p 127.0.0.1:25432:5432 -e POSTGRES_PASSWORD \
  -e POSTGRES_USER=console_contract -e POSTGRES_DB=console_contract_test \
  postgres:18-alpine >/dev/null
created+=("$pg_container")
export MYSQL_PASSWORD="$CONSOLE_DB_CONTRACT_PASSWORD" MYSQL_ROOT_PASSWORD="$CONSOLE_DB_CONTRACT_PASSWORD"
docker run --detach --name "$mysql_container" --label console.purpose=db-contract \
  -p 127.0.0.1:23306:3306 -e MYSQL_PASSWORD -e MYSQL_ROOT_PASSWORD \
  -e MYSQL_USER=console_contract -e MYSQL_DATABASE=console_contract_test \
  mysql:8.4 --log-bin-trust-function-creators=1 >/dev/null
created+=("$mysql_container")
ready=false
for (( attempt=0; attempt<60; attempt++ )); do
  if docker exec "$pg_container" pg_isready -U console_contract -d console_contract_test >/dev/null 2>&1 \
    && docker exec "$mysql_container" sh -c 'MYSQL_PWD="$MYSQL_PASSWORD" mysql -h 127.0.0.1 -u console_contract -D console_contract_test -e "SELECT 1"' >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 1
done
if [[ "$ready" != true ]]; then
  printf '%s\n' 'Isolated databases did not become ready within 60 seconds.' >&2
  exit 1
fi
# The endpoints are ephemeral; cached test results cannot validate a new server.
contract_scope="${1:-all}"
case "$contract_scope" in all|connection|config|tasks|business|transfer) ;; *) printf 'Unknown contract scope: %s\n' "$contract_scope" >&2; exit 1 ;; esac
if [[ "$contract_scope" == all || "$contract_scope" == connection ]]; then
  GOFLAGS="${GOFLAGS:-} -count=1" bash scripts/check-go.sh test ./internal/dbconn ./internal/dbconn/__tests__
fi
if [[ "$contract_scope" == all || "$contract_scope" == config ]]; then
  GOFLAGS="${GOFLAGS:-} -count=1" go test -race -p=4 -parallel=4 -timeout=20m ./internal/configstore/__tests__ -run TestDatabaseContract
fi
if [[ "$contract_scope" == all || "$contract_scope" == tasks ]]; then
  GOFLAGS="${GOFLAGS:-} -count=1" go test -race -p=4 -parallel=4 -timeout=20m ./internal/taskstore/__tests__ -run TestDatabaseContract
fi
if [[ "$contract_scope" == all || "$contract_scope" == business ]]; then
  GOFLAGS="${GOFLAGS:-} -count=1" go test -race -p=4 -parallel=4 -timeout=20m ./internal/business/__tests__ -run TestDatabaseContract
fi
if [[ "$contract_scope" == all || "$contract_scope" == transfer ]]; then
  GOFLAGS="${GOFLAGS:-} -count=1" bash scripts/check-go.sh test ./internal/dbtransfer ./internal/dbtransfer/__tests__
fi
