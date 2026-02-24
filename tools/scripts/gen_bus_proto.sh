#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PROTO_ROOT="$ROOT_DIR/proto"
OUT_DIR="$ROOT_DIR/libs/go/domain"

# Ensure Go bin directory is on PATH so protoc can find protoc-gen-go.
if command -v go >/dev/null 2>&1; then
  GO_BIN_DIR="$(go env GOPATH 2>/dev/null)/bin"
  if [ -n "$GO_BIN_DIR" ] && [[ ":$PATH:" != *":$GO_BIN_DIR:"* ]]; then
    export PATH="$GO_BIN_DIR:$PATH"
  fi
fi

echo "ROOT_DIR: $ROOT_DIR"
echo "PROTO_ROOT: $PROTO_ROOT"
echo "OUT_DIR: $OUT_DIR"

if ! command -v protoc >/dev/null 2>&1; then
  echo "protoc not found on PATH" >&2
  exit 1
fi

if ! command -v protoc-gen-go >/dev/null 2>&1; then
  echo "protoc-gen-go not found on PATH" >&2
  echo "Install it with: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest" >&2
  exit 1
fi

mkdir -p "$OUT_DIR"

# Remove previously generated bus proto files to avoid stale artifacts.
rm -rf "$OUT_DIR"/bus

PROTO_FILES=$(find "$PROTO_ROOT" -name '*.proto' | sort)

if [ -z "$PROTO_FILES" ]; then
  echo "No proto files found under $PROTO_ROOT" >&2
  exit 1
fi

protoc \
  -I "$PROTO_ROOT" \
  --go_out "$OUT_DIR" \
  --go_opt paths=source_relative \
  $PROTO_FILES

echo "Generated Go bus protos in $OUT_DIR"
