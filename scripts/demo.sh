#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ENV_FILE="${ENV_FILE:-.env}"
SMOKE_EXCHANGES="${SMOKE_EXCHANGES:-okx,bithumb,bitget,kucoin,gate,htx,coinex,whitebit,bitmart}"
RUN_INGEST="${RUN_INGEST:-1}"
START_SERVICES="${START_SERVICES:-1}"
START_BOT="${START_BOT:-auto}"

load_env_file() {
  local file="$1"
  local overwrite="$2"
  [[ -f "$file" ]] || return 0

  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line#"${line%%[![:space:]]*}"}"
    line="${line%"${line##*[![:space:]]}"}"
    [[ -z "$line" || "${line:0:1}" == "#" ]] && continue
    [[ "$line" == export\ * ]] && line="${line#export }"
    [[ "$line" == *=* ]] || continue

    local key="${line%%=*}"
    local value="${line#*=}"
    key="${key//[[:space:]]/}"
    value="${value#"${value%%[![:space:]]*}"}"
    value="${value%"${value##*[![:space:]]}"}"
    if [[ ${#value} -ge 2 ]]; then
      local first="${value:0:1}"
      local last="${value: -1}"
      if [[ "$first" == "$last" && ( "$first" == "'" || "$first" == '"' ) ]]; then
        value="${value:1:${#value}-2}"
      fi
    fi

    if [[ "$overwrite" == "1" || -z "${!key+x}" ]]; then
      export "$key=$value"
    fi
  done < "$file"
}

run() {
  printf '\n==> %s\n' "$*"
  "$@"
}

cleanup() {
  if [[ -n "${BOT_PID:-}" ]]; then
    kill "$BOT_PID" 2>/dev/null || true
  fi
  if [[ -n "${API_PID:-}" ]]; then
    kill "$API_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

load_env_file ".env.example" 0
load_env_file "$ENV_FILE" 1

: "${LISTEN_ADDR:=:8080}"
export LISTEN_ADDR

if [[ -z "${DATABASE_URL:-}" ]]; then
  printf 'DATABASE_URL is required. Set it in %s or export it before running.\n' "$ENV_FILE" >&2
  exit 1
fi

run go run ./cmd/migrate up

if [[ "$RUN_INGEST" == "1" ]]; then
  run go run ./cmd/smoke-adapters -env "$ENV_FILE" -exchanges "$SMOKE_EXCHANGES"
else
  printf '\n==> skipping live ingestion because RUN_INGEST=%s\n' "$RUN_INGEST"
fi

audit_flags=()
if [[ -n "${CMC_API_KEY:-}" ]]; then
  audit_flags+=("-require-cmc=true")
fi
run go run ./cmd/normalization-audit -env "$ENV_FILE" "${audit_flags[@]}"
run go run ./cmd/e2e-smoke -env "$ENV_FILE"

http_addr="${LISTEN_ADDR}"
if [[ "$http_addr" == :* ]]; then
  http_addr="localhost${http_addr}"
fi
base_url="http://${http_addr}"

printf '\nDemo routes:\n'
printf '  UI: %s/\n' "$base_url"
printf '  USDT live route: curl "%s/v1/routes?coin=usdt&from_chain=ethereum&to_chain=bsc&amount=1000"\n' "$base_url"
printf '  BTC equivalent route: curl "%s/v1/routes?coin=btc&from_chain=bitcoin&to_chain=ethereum&amount=0.1&equivalent_assets=true"\n' "$base_url"
printf '  Events: curl "%s/v1/events?limit=5"\n' "$base_url"

if [[ "$START_SERVICES" != "1" ]]; then
  printf '\nChecks complete. START_SERVICES=%s, so API/bot were not started.\n' "$START_SERVICES"
  exit 0
fi

printf '\n==> starting API on %s\n' "$LISTEN_ADDR"
ENV_FILE="$ENV_FILE" go run ./cmd/api &
API_PID=$!

if [[ "$START_BOT" == "auto" ]]; then
  if [[ -n "${TELEGRAM_BOT_TOKEN:-}" ]]; then
    START_BOT=1
  else
    START_BOT=0
  fi
fi

if [[ "$START_BOT" == "1" ]]; then
  printf '==> starting Telegram bot\n'
  ENV_FILE="$ENV_FILE" go run ./cmd/bot &
  BOT_PID=$!
else
  printf '==> skipping Telegram bot because START_BOT=%s or TELEGRAM_BOT_TOKEN is unset\n' "$START_BOT"
fi

printf '\nDemo services are running. Press Ctrl-C to stop.\n'
wait "$API_PID"
