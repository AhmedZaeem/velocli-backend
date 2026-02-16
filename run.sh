#!/usr/bin/env bash
set -euo pipefail

BASE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export VELOCLI_DATA_KEY="$(cat "$BASE_DIR/data/.key")"
export VELOCLI_BACKEND_ADDR="${VELOCLI_BACKEND_ADDR:-0.0.0.0:9999}"

echo "Starting backend at ${VELOCLI_BACKEND_ADDR}..."
go run ./cmd/api
