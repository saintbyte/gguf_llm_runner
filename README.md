# gguf_llm_runner

CLI-чат с локальной LLM (GGUF) на базе [llama-go](https://github.com/tcpipuk/llama-go) — Go-биндингов к llama.cpp. Стриминг ответа, поддержка reasoning-моделей, разовая генерация и интерактивный чат с TUI ([bubbletea](https://github.com/charmbracelet/bubbletea)).

## Требования

- Linux/macOS, C++-компилятор (g++/clang) и CMake
- Go 1.26+ (версия ниже — `go` сам скачает нужный toolchain при сборке)
- ~1.5 ГБ свободного места (библиотека + модель 639 МБ)

## Установка и запуск

```bash
make download-model   # скачивает тестовую модель Qwen3-0.6B-Q8_0.gguf в models/
make build            # собирает llama-go (один раз) и бинарник в bin/
make run              # запускает интерактивный чат
```

## Использование

```bash
# Интерактивный чат (стриминг, по умолчанию)
./bin/gguf_llm_runner

# Разовая генерация
./bin/gguf_llm_runner -message "What is the capital of France?"

# Другая модель
./bin/gguf_llm_runner -model models/mymodel.gguf

# Без показа рассуждений (<think>...</think> скрываются)
./bin/gguf_llm_runner -reasoning=false

# Классический построчный интерактив без TUI (для pipe/не-TTY)
./bin/gguf_llm_runner -tui=false

# С LoRA-адаптером (применяется к контексту)
./bin/gguf_llm_runner -lora models/my-adapter.gguf -lora-scale 0.8

# Полный список опций
./bin/gguf_llm_runner -h
```

### TUI-режим

В интерактивном режиме в терминале открывается TUI (bubbletea). Вверху — шапка с моделью и параметрами генерации (контекст, max_tokens, температура, top-p/k, seed, потоки, GPU, LoRA), под каждым ответом — статистика генерации (токены, скорость `ток/с`, время, размер контекста). Рассуждения модели (`<think>...</think>`) показываются приглушённым курсивом и не попадают в историю диалога. Горячие клавиши:

- `Enter` — отправить сообщение
- `Tab` — переключить фокус между полем ввода и прокруткой диалога
- Колесо мыши / `PgUp` / `PgDn` — прокрутка диалога (работает и при фокусе на поле ввода)
- `Ctrl-C` — прервать генерацию / выйти из чата
- Команды в поле ввода: `/clear` — сбросить диалог, `/exit` (или `/quit`) — выйти, `/help` — справка

Во время генерации чат автоматически прокручивается к свежим токенам. Если отлистать вверх (например, чтобы прочитать начало длинных рассуждений `<think>`), автопрокрутка приостанавливается и в футере появляется подсказка `↑/↓ прокрутка`; вернуться в конец — `PgDn` или колесо вниз.

При перенаправлении stdout (не TTY) TUI автоматически отключается и включается построчный режим.

### Основные флаги

| Флаг | По умолчанию | Описание |
| --- | --- | --- |
| `-model` | `models/Qwen3-0.6B-Q8_0.gguf` | Путь к GGUF-модели |
| `-message` | — | Разовая генерация (пусто = интерактив) |
| `-system` | `You are a helpful assistant.` | Системный промпт |
| `-context` | `4096` | Размер контекста (0 = нативный максимум) |
| `-max-tokens` | `1024` | Максимум токенов в ответе |
| `-temperature` | `0.7` | Температура сэмплинга |
| `-top-p` | `0.9` | Nucleus sampling |
| `-top-k` | `40` | Top-K sampling |
| `-seed` | `-1` | Сид (фикс. результат; -1 = случайный) |
| `-ngl` | `-1` | Слоёв на GPU (-1 = все, работает при GPU-сборке) |
| `-threads` | число CPU | Потоков CPU |
| `-reasoning` | `true` | Подсвечивать рассуждения `<think>...</think>` |
| `-timeout` | `120` | Таймаут генерации, сек. (0 = без) |
| `-stream` | `true` | Стриминг ответа |
| `-tui` | `true` | TUI в интерактивном режиме (автофолбэк на построчный вне TTY) |
| `-lora` | — | Путь к LoRA-адаптеру (`.gguf`), применяется к контексту |
| `-lora-scale` | `1.0` | Масштаб LoRA-адаптера |
| `-debug` | `false` | Отладка (лог llama.cpp, промпт перед каждым ходом) |

## Сборка из исходников llama-go

Скрипты сборки требуют клона llama-go с submodule llama.cpp (делается автоматически через `make build`):

```bash
# Вручную
git clone --depth 1 --branch llama.cpp-b10069 \
  --recurse-submodules --shallow-submodules \
  https://github.com/tcpipuk/llama-go third_party/llama-go
make -C third_party/llama-go libbinding.a

export LIBRARY_PATH=$PWD/third_party/llama-go
export C_INCLUDE_PATH=$PWD/third_party/llama-go
go build -o bin/gguf_llm_runner ./cmd/gguf_llm_runner
```

GPU-сборки: см. [building.md](https://github.com/tcpipuk/llama-go/blob/main/docs/building.md) (CUDA, Metal, Vulkan, OpenCL — через `BUILD_TYPE` и Go-теги).

## Структура

```
cmd/gguf_llm_runner/   точка входа CLI
internal/llmadapter/   загрузка и применение LoRA (cgo-шим llama.cpp)
internal/llmcli/       стриминг-рендерер (обработка <think>) и построчный режим
internal/tui/          TUI-чат на bubbletea
scripts/               обёртки над Makefile
models/                скачанные модели (в git не коммитятся)
third_party/llama-go/  локальный клон llama-go для сборки (gitignored)
```
