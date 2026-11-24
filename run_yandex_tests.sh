#!/bin/bash

# Set the Yandex API key
export YANDEX_API_KEY=c7e69f5a-84d0-496f-b1ec-38c184f1b26e

# Run the integration tests
echo "Running Yandex integration tests..."
go test ./internal/providers/transport -v -run TestYandexIntegration -timeout 20s

# Run all tests
echo ""
echo "Running all Yandex provider tests..."
go test ./internal/providers/transport -v -run TestYandex -timeout 20s
