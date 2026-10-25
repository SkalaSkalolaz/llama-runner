# llama-runner

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://go.dev/)
[![Tests](https://img.shields.io/badge/tests-40%2B-brightgreen.svg)](#разработка)
[![Race](https://img.shields.io/badge/race-clean-brightgreen.svg)](#разработка)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](LICENSE)

[![CI](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml/badge.svg)](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml)

**Русский** · [English](README.md)

Локальный лаунчер: запускает `llama-server` с вашей GGUF-моделью и автоматически открывает
**Claude Code**, привязанный к этому серверу. При закрытии Claude Code сервер останавливается.

## Быстрый старт

```bash
go build -o llama-runner .

./llama-runner                  # llama-server + Claude Code (по умолчанию)
./llama-runner -server          # только llama-server (старое поведение)
./llama-runner -webui-mcp-proxy # + поисковый MCP-сервер и прокси CORS
```

## Разработка

```bash
go build ./...
go vet ./...
go test -v ./...
go test -race ./...
```

## Что умеет

- **Автоматический запуск:** проверяет наличие модели, поднимает сервер, ждёт готовности
  `/health` и открывает Claude Code.
- **MCP-прокси:** `-webui-mcp-proxy` запускает `mcp-search-server`, генерирует временный
  MCP-конфиг и пробрасывает CORS. С `-mcp-strict` — только этот MCP, без сторонних
  из `~/.claude.json`.
- **Обновление llama.cpp:** `-update` выполняет `git pull` → `cmake configure/build/install`.
- **Спекулятивное декодирование:** флаги `-spec-type`, `-spec-draft-model`,
  `-spec-draft-n-max`, `-spec-draft-ngl`.
- **Мультимодальность:** `-mmproj` подключает проектор.
- **Алиас модели:** `-alias` подставляет `--alias` в llama-server и
  `ANTHROPIC_DEFAULT_*_MODEL` в Claude Code.
- **Чат-шаблон:** `-chat-template "auto"` (по имени файла, например
  `qwen-fixed-chat-template.jinja`), явный путь или `""`.
- **Управление глубиной размышлений:** `-reasoning` и `-reasoning-budget` (работает только
  для моделей с поддержкой reasoning).
- **Конфиг-файл:** `~/.config/llama-runner/config.json` (или `-config <path>`). Приоритет:
  CLI > файл > встроенные дефолты. `-config ""` — не читать. Секреты (`openaiKey`,
  `proxyToken`) в файле поддерживаются, но рекомендуется env / `credentials` / флаг.
- **Профили:** `-p coder` — короткие пресеты из `profiles: {…}` в конфиге.
- **`-doctor`:** показать разрешённые пути, версию и итоговую команду без запуска процессов.

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
| `-reasoning` | _(не передаётся)_ | Режим размышлений: `on`/`off`/`auto` |
| `-reasoning-budget` | _(не передаётся)_ | Бюджет токенов на размышления: `-1`/`0`/`N` |
| `-v` | | Показать команду перед запуском |
| `-version` | | Показать версию |
| `-config` | `~/.config/llama-runner/config.json` | Путь к конфиг-файлу; `""` — не читать |
| `-p` | _(не задан)_ | Профиль из конфиг-файла |
| `-doctor` | | Диагностика без запуска |

## Гибридный режим: llama-server + внешний OpenAI API

Можно одновременно использовать локальный llama-server и внешний
OpenAI-совместимый сервис (OrcaRouter, OpenRouter, DeepSeek, …).
В Claude Code переключение моделей — через `/model`:

- `llama-server` (или выбор из меню `/model`) → локальный llama-server
- **Все остальные** (`sonnet`, `haiku`, `opus`, любое другое имя) → внешний провайдер

`llama-server` также добавлен в интерактивное меню `/model` через
`ANTHROPIC_CUSTOM_MODEL_OPTION`, чтобы его можно было выбрать стрелками.

```bash
./llama-runner \
  -openai https://api.orcarouter.ai/v1 \
  -key $ORCAROUTER_API_KEY \
  -model-openai deepseek/deepseek-v4-flash-free
```

## Примеры

```bash
# Сервер + Claude Code в папке проекта
./llama-runner -alias qwen3-coder -claude-dir ~/my-project

# Только сервер на другом порту
./llama-runner -server -port 9090

# Сервер + Claude Code с поиском
./llama-runner -webui-mcp-proxy

# Без лишних MCP из ~/.claude.json
./llama-runner -webui-mcp-proxy -mcp-strict

# С LiLiCorr (спекулятивное декодирование)
./llama-runner -server -model ~/models/nemotron-bolt/NVIDIA-Nemotron-3.5-Bolt-74B-A9B-24GB.gguf \
  -mmproj ~/models/nemotron-bolt/mmproj-NVIDIA-Nemotron-3.5-Bolt-74B-A9B-24GB.gguf \
  -spec-type draft-dflash -spec-draft-model ~/models/...-LiLiCorr.gguf \
  -spec-draft-n-max 6 -spec-draft-ngl 999 -ngl 999 \
  -c 98304 -b 2048 -ub 512 -cache-type-k q8_0 -cache-type-v q8_0 \
  -temp 1.0 -top-p 0.95 -fa on -jinja -chat-template ""

# Обновить llama.cpp (архитектура Pascal) и запустить
./llama-runner -update -cuda-arch 61 && ./llama-runner -server -model ~/GGUF/model.gguf

# Профиль из конфиг-файла
./llama-runner -p coder

# Диагностика без запуска
./llama-runner -doctor
```

## Конфиг-файл

По умолчанию читается `~/.config/llama-runner/config.json`. Приоритет значений:
**CLI > конфиг > встроенные дефолты**. Флаг `-config ""` отключает чтение файла
(поведение как до M6).

```json
{
  "model": "~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf",
  "host": "127.0.0.1",
  "port": "8080",
  "jinja": true,
  "readyTimeout": "180s",
  "chatTemplate": "auto",
  "chatTemplateFallback": "~/GGUF/qwen-fixed-chat-template.jinja",
  "profiles": {
    "coder": {
      "model": "~/GGUF/qwen3-coder.gguf",
      "alias": "qwen3-coder",
      "c": "98304"
    },
    "fast": {
      "model": "~/GGUF/llama3.2-3b.gguf",
      "c": "16384",
      "b": "512"
    }
  }
}
```

## Переменные окружения, передаваемые Claude Code

`ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY=local-llama`, `ANTHROPIC_AUTH_TOKEN=local-llama`,
`ANTHROPIC_DEFAULT_*_MODEL` (при `-alias`), `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`.

---

[BSD 3-Clause License](LICENSE) · Copyright © 2026 Skala Skalolaz
