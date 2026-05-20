#!/usr/bin/env sh
set -eu

if [ ! -d .git ]; then
  echo "No .git directory found; skipping hook installation."
  exit 0
fi

mkdir -p .git/hooks
cp .githooks/pre-commit .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
echo "Installed pre-commit hook."
