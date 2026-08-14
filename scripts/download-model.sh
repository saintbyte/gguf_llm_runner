#!/usr/bin/env bash
# Скачивает тестовую модель Qwen3-0.6B-Q8_0.gguf в models/
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$ROOT_DIR"
make download-model
