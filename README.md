# llama-runner

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://go.dev/)
[![Tests](https://img.shields.io/badge/tests-40%2B-brightgreen.svg)](#разработка)
[![Race](https://img.shields.io/badge/race-clean-brightgreen.svg)](#разработка)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](LICENSE)

[![CI](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml/badge.svg)](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml)

Локальный лаунчер: запускает `llama-server` с вашей GGUF-моделью и автоматически открывает **Claude Code**, привязанный к этому серверу. При закрытии Claude Code сервер останавливается.

## Быстрый старт

```bash
go build -o server .

./server                  # сервер + Claude Code (по умолчанию)
./server -server          # только llama-server (старое поведение)
./server -webui-mcp-proxy # + поисковый MCP-сервер и прокси CORS
```

## Разработка

```bash
go build ./...
go vet ./...
go test -v ./...
go test -race ./...
```

## Что умеет

- **Автоматический запуск:** проверяет наличие модели, поднимает сервер, ждёт готовности `/health` и открывает Claude Code.
- **MCP-прокси:** `-webui-mcp-proxy` запускает `mcp-search-server`, генерирует временный MCP-конфиг и пробрасывает CORS. С `-mcp-strict` — только этот MCP, без сторонних из `~/.claude.json`.
- **Обновление llama.cpp:** `-update` выполняет `git pull` → `cmake configure/build/install`.
- **Спекулятивное декодирование:** флаги `-spec-type`, `-spec-draft-model`, `-spec-draft-n-max`, `-spec-draft-ngl`.
- **Мультимодальность:** `-mmproj` подключает проектор.
- **Алиас модели:** `-alias` подставляет `--alias` в llama-server и `ANTHROPIC_DEFAULT_*_MODEL` в Claude Code.
- **Чат-шаблон:** `-chat-template "auto"` (по имени файла, например `qwen-fixed-chat-template.jinja`), явный путь или `""`.

## Основные флаги

| Флаг | По умолчанию | Что |
|------|-------------|-----|
| `-model` | `~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf` | Путь к GGUF |
| `-bin` | `~/.local/bin/llama-server` | Путь к llama-server |
| `-host:port` | `127.0.0.1:8080` | Адрес и порт сервера |
| `-alias` | `local` | Алиас модели |
| `-c, -b, -ub` | `65536/2048/1024` | Контекст / батч / микро-батч |
| `-jinja` | `true` | Jinja-шаблоны |
| `-ngl` | _(не передаётся)_ | Слоёв на GPU |
| `-fa` | `auto` | Flash attention |
| `-parallel` | `1` | Кол-во параллельных слотов |
| `-ready-timeout` | `180s` | Таймаут ожидания готовности |
| `-v` | | Показать команду перед запуском |
| `-version` | | Показать версию |

## Гибридный режим: llama-server + внешний OpenAI API

Можно одновременно использовать локальный llama-server и внешний
OpenAI-совместимый сервис (OrcaRouter, OpenRouter, DeepSeek, …).
В Claude Code переключение моделей — через `/model`:

- `llama-server` (или выбор из меню `/model`) → локальный llama-server
- **Все остальные** (`sonnet`, `haiku`, `opus`, любое другое имя) → внешний провайдер

`llama-server` также добавлен в интерактивное меню `/model` через
`ANTHROPIC_CUSTOM_MODEL_OPTION`, чтобы его можно было выбрать стрелками.

```bash
./server \
  -openai https://api.orcarouter.ai/v1 \
  -key $ORCAROUTER_API_KEY \
  -model-openai deepseek/deepseek-v4-flash-free
```

## Примеры

```bash
# Сервер + Claude Code в папке проекта
./server -alias qwen3-coder -claude-dir ~/my-project

# Только сервер на другом порту
./server -server -port 9090

# Сервер + Claude Code с поиском
./server -webui-mcp-proxy

# Без лишних MCP из ~/.claude.json
./server -webui-mcp-proxy -mcp-strict

# С LiLiCorr (спекулятивное декодирование)
./server -server -model ~/models/nemotron-bolt/NVIDIA-Nemotron-3.5-Bolt-74B-A9B-24GB.gguf \
  -mmproj ~/models/nemotron-bolt/mmproj-NVIDIA-Nemotron-3.5-Bolt-74B-A9B-24GB.gguf \
  -spec-type draft-dflash -spec-draft-model ~/models/...-LiLiCorr.gguf \
  -spec-draft-n-max 6 -spec-draft-ngl 999 -ngl 999 \
  -c 98304 -b 2048 -ub 512 -cache-type-k q8_0 -cache-type-v q8_0 \
  -temp 1.0 -top-p 0.95 -fa on -jinja -chat-template ""

# Обновить llama.cpp (архитектура Pascal) и запустить
./server -update -cuda-arch 61 && ./server -server -model ~/GGUF/model.gguf
```

## Переменные окружения, передаваемые Claude Code

`ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY=local-llama`, `ANTHROPIC_AUTH_TOKEN=local-llama`,
`ANTHROPIC_DEFAULT_*_MODEL` (при `-alias`), `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`.

---

[BSD 3-Clause License](LICENSE) · Copyright © 2026 Skala Skalolaz
