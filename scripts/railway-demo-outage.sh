#!/usr/bin/env bash
set -euo pipefail

POSTGRES_SERVICE="${RAILWAY_POSTGRES_SERVICE:-Postgres}"

DATABASE_URL="$(
  railway variable list --service "$POSTGRES_SERVICE" --json | node -e '
    let input = "";
    process.stdin.on("data", (chunk) => input += chunk);
    process.stdin.on("end", () => {
      const parsed = JSON.parse(input);
      const value = Array.isArray(parsed)
        ? (parsed.find((item) => item.name === "DATABASE_PUBLIC_URL" || item.key === "DATABASE_PUBLIC_URL") || {}).value
        : parsed.DATABASE_PUBLIC_URL;
      if (!value) {
        console.error("DATABASE_PUBLIC_URL missing on Postgres service");
        process.exit(1);
      }
      process.stdout.write(value);
    });
  '
)"

DATABASE_URL="$DATABASE_URL" go run ./cmd/demo-outage -env /tmp/cex-router-no-env "$@"
