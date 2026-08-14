#!/usr/bin/env bash
# Собирает llama-go (libbinding.a) и бинарник gguf_llm_runner в bin/
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$ROOT_DIR"
make build
