#!/bin/bash

set -euo pipefail

# Supply your own key in the environment; never store it in this script.
if [[ -z "${YANDEX_RASP_API_KEY:-${YANDEX_API_KEY:-}}" ]]; then
    echo "Set YANDEX_RASP_API_KEY (or the legacy YANDEX_API_KEY) before running live tests." >&2
    exit 1
fi

# Run the integration tests
echo "Running Yandex integration tests..."
go test ./internal/providers/transport -v -run TestYandexIntegration -timeout 90s

# Run all tests
echo ""
echo "Running all Yandex provider tests..."
go test ./internal/providers/transport -v -run TestYandex -skip TestYandexIntegration -timeout 30s
