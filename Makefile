SHELL := /bin/bash

GO          ?= go
THIRD_PARTY := third_party/llama-go
MODEL_DIR   := models
MODEL_NAME  := Qwen3-0.6B-Q8_0.gguf
MODEL_URL   := https://huggingface.co/Qwen/Qwen3-0.6B-GGUF/resolve/main/$(MODEL_NAME)?download=true
MODEL_PATH  := $(MODEL_DIR)/$(MODEL_NAME)
BINARY      := bin/gguf_llm_runner

export LIBRARY_PATH  := $(abspath $(THIRD_PARTY))
export C_INCLUDE_PATH := $(abspath $(THIRD_PARTY))

.PHONY: all download-model model lib build run chat clean

all: build

## Скачивает модель Qwen3-0.6B-Q8_0.gguf в $(MODEL_DIR)/
download-model: $(MODEL_PATH)

model: download-model

$(MODEL_DIR):
	mkdir -p $(MODEL_DIR)

# Скачивание с прогресс-баром и проверкой, что файл не битый
# (валидный GGUF начинается с сигнатуры "GGUF")
$(MODEL_PATH): | $(MODEL_DIR)
	@echo "Скачивание модели: $(MODEL_URL)"
	curl -L --fail --retry 3 -C - --progress-bar "$(MODEL_URL)" -o "$@.part"
	@head -c 4 "$@.part" | grep -q GGUF || { echo "Ошибка: невалидный GGUF-файл" >&2; rm -f "$@.part"; exit 1; }
	@mv "$@.part" "$@"
	@echo "Модель сохранена: $(MODEL_PATH)"

## Клонирует и собирает библиотеку llama-go (нужна один раз)
lib:
	@if [ ! -f "$(THIRD_PARTY)/libbinding.a" ]; then \
		echo "Сборка llama-go..." && \
		git clone --depth 1 --branch llama.cpp-b10069 \
			--recurse-submodules --shallow-submodules \
			https://github.com/tcpipuk/llama-go "$(THIRD_PARTY)" && \
		$(MAKE) -C "$(THIRD_PARTY)" libbinding.a; \
	fi

## Собирает бинарник в $(BINARY)
build: lib
	$(GO) build -o $(BINARY) ./cmd/gguf_llm_runner
	@echo "Готово: $(BINARY)"

## Запускает интерактивный чат с моделью
run: $(BINARY) $(MODEL_PATH)
	$(BINARY) -model $(MODEL_PATH)

chat: run

clean:
	rm -rf $(BINARY)
	rm -rf $(THIRD_PARTY)
	rm -rf build
