#!/usr/bin/env bash
set -euo pipefail

# Simple project bootstrapper for local tooling required by the proto/Kafka build.
# Currently supports macOS environments that use Homebrew.

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

if ! command_exists brew; then
  echo "Homebrew is required but not found on PATH. Install it from https://brew.sh/" >&2
  exit 1
fi

if ! command_exists buf; then
  echo "Installing buf CLI via Homebrew..."
  brew install bufbuild/buf/buf
else
  echo "buf already installed"
fi

if ! command_exists protoc; then
  echo "Installing protobuf compiler via Homebrew..."
  brew install protobuf
else
  echo "protoc already installed"
fi

if ! command_exists go; then
  echo "Go toolchain is required but not found on PATH." >&2
  exit 1
fi

if ! command_exists protoc-gen-go; then
  echo "Installing protoc-gen-go via 'go install'..."
  GO111MODULE=on go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
  echo "Ensure \"$(go env GOPATH)/bin\" is on your PATH so protoc can find protoc-gen-go."
else
  echo "protoc-gen-go already installed"
fi

echo "Tooling install complete. You can now run dev scripts in ./tools/scripts"
