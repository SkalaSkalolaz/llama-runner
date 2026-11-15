# README_RU.md

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://go.dev/)
[![Tests](https://img.shields.io/badge/tests-135%2B-brightgreen.svg)](#разработка)
[![Race](https://img.shields.io/badge/race-clean-brightgreen.svg)](#разработка)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](LICENSE)
[![CI](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml/badge.svg)](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml)

**Русский** · [English](README.md)

Локальный лаунчер: запускает `llama-server` с GGUF-моделью и автоматически открывает
**Claude Code**, привязанный к этому серверу. При закрытии Claude Code сервер останавливается.

## Установка

Одной командой (Go не требуется):
```sh
curl -fsSL https://raw.githubusercontent.com/SkalaSkalolaz/llama-runner/master/install.sh | sh
```

С параметрами:
```sh
curl -fsSL .../install.sh | sh -s -- --version v1.9.0 --with-agents --with-default-model
```

Installer помещает бинарь в `~/.local/bin/`, создаёт конфиг-директории,
проверяет SHA-256, опционально устанавливает агентские файлы и скачивает
маленькую модель Qwen (~400 МБ). Никогда не запускает sudo и не трогает
пути за пределами `$HOME`.

**Примечание:** `mcp-search-server` (для `-webui-mcp-proxy`) — отдельный проект:
https://github.com/SkalaSkalolaz/mcp-search-server

## Быстрый старт

```bash
git clone https://github.com/SkalaSkalolaz/llama-runner.git
cd llama-runner
go build -o llama-runner ./cmd/llama-runner
./llama-runner                  # llama-server + Claude Code (по умолчанию)
./llama-runner -server          # только llama-server
./llama-runner -webui-mcp-proxy # + поисковый MCP-сервер
```

## Возможности

- **Автоматический режим:** профиль с `"mode": "auto"` — планировщик: разбивает задачу
  на шаги, выбирает модели из `profiles`, оркестратор исполняет с hot-swap
- **Git-страховка:** `-git-guard` создаёт ветку прогона, чекпоинт на волну, откат при провале
- **Повторы шагов:** `-max-retries N` (0..5) повторяет провалившиеся шаги с контекстом ошибки
- **Карта проекта:** `-repo-map` добавляет контекст рабочей директории в промпт планировщика
- **Preflight плана:** `-preflight` проверяет verify-команды и артефакты до исполнения
- **Гибридный режим:** прокси маршрутизирует между локальным llama-server (нативный Anthropic API)
  и внешним OpenAI-совместимым API
- **Автозагрузка:** `-download` скачивает GGUF-модели (single или sharded) перед запуском
- **MCP-прокси:** `-webui-mcp-proxy` запускает поисковый сервер с CORS-прокси
- **Спекулятивное декодирование:** `-spec-type`, `-spec-draft-model` и др.
- **Мультимодальность:** `-mmproj` для vision-проектора
- **Алиас модели:** `-alias` устанавливает `--alias` llama-server и запись `/model` Claude Code
- **Чат-шаблон:** `-chat-template "auto"` или явный путь
- **Управление размышлениями:** `-reasoning` и `-reasoning-budget`
- **Логи с ротацией:** ротация по размеру в `~/.cache/llama-runner/`
- **Конфиг-файл:** `~/.config/llama-runner/config.json` с профилями
- **Per-model parallel:** задаётся per-profile, CLI `-model` сбрасывает в `"1"`
- **`-doctor`:** диагностика без запуска процессов
- **Корректная остановка:** Ctrl-C/SIGTERM с эскалацией SIGTERM → SIGKILL

## Флаги

### Сервер и llama.cpp
| Флаг | По умолчанию | Описание |
|------|--------------|----------|
| `-model` | `~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf` | Путь к GGUF |
| `-bin` | `~/.local/bin/llama-server` | Путь к llama-server |
| `-host` / `-port` | `127.0.0.1` / `8080` | Адрес сервера |
| `-alias` | из `-model` или конфига | Алиас модели |
| `-c` / `-b` / `-ub` | `65536` / `2048` / `1024` | Контекст / батч / микро-батч |
| `-parallel` | `1` | Параллельные слоты (per-profile) |
| `-ngl` | — | Слои на GPU |
| `-mmproj` | — | Мультимодальный проектор |
| `-reasoning` / `-reasoning-budget` | — | Режим / бюджет размышлений |
| `-spec-*` | — | Спекулятивное декодирование |
| `-chat-template` | `auto` | Шаблон чата (`auto` / путь / `""`) |
| `-fa` / `-fit` / `-lv` | — | Flash attention / fit-режим / уровень логов |

### Claude Code
| Флаг | По умолчанию | Описание |
|------|--------------|----------|
| `-server` | | Только llama-server |
| `-claude-dir` | текущая директория | Рабочая директория Claude |
| `-ready-timeout` | `180s` | Таймаут `/health` |
| `-log-file` | `~/.cache/llama-runner/llama-server.log` | Файл логов |
| `-log-max-mb` / `-log-keep` | `50` / `5` | Ротация логов |
| `-git-guard` | `false` | Git-страховка (auto-режим) |
| `-max-retries` | `0` | Повторы шагов (0..5) |
| `-stream-output` | `false` | Живой вывод шагов в терминал |
| `-resume` | `false` | Продолжить последний прогон из журнала |

### Внешний провайдер (гибридный режим)
| Флаг | По умолчанию | Описание |
|------|--------------|----------|
| `-openai` | | URL OpenAI-совместимого API |
| `-key` | | API-ключ (`none` или `""` = без авторизации) |
| `-model-openai` | | Имя внешней модели |
| `-openai-path` | `/chat/completions` | Суффикс пути (`""` для OVH/VLM.run) |
| `-proxy-addr` | `127.0.0.1:8082` | Адрес локального прокси |
| `-proxy-token` | | Токен авторизации прокси |

### Конфиг и диагностика
| Флаг | По умолчанию | Описание |
|------|--------------|----------|
| `-config` | `~/.config/llama-runner/config.json` | Конфиг-файл (`""` = не читать) |
| `-download` | | Скачать модель: `<директория><URL>` |
| `-p` | | Имя профиля |
| `-task` | | Задача для планировщика (auto-режим) |
| `-plan-only` | | Только построить план, без исполнения |
| `-repo-map` | `false` | Карта проекта в промпт планировщика |
| `-preflight` | `false` | Проверка плана до исполнения |
| `-doctor` | | Только диагностика |
| `-update` | | Обновить llama.cpp и выйти |

## Гибридный режим

Маршрутизирует запросы Claude Code между локальным llama-server и внешним
OpenAI-совместимым API:
- Алиас локальной модели или `llama-server` → локальный llama-server (нативный Anthropic API, verbatim)
- Всё остальное → внешний провайдер

```bash
./llama-runner \
  -openai https://api.orcarouter.ai/v1 \
  -key $ORCAROUTER_API_KEY \
  -model-openai deepseek/deepseek-v4-flash-free
```

## Ключ провайдера

Приоритет: флаг `-key` → `LLAMA_RUNNER_OPENAI_KEY` → `OPENAI_API_KEY` →
файл `~/.config/llama-runner/credentials`.

**Режим без ключа:** `-key none` или `-key ""` не отправляет заголовок
`Authorization` (для провайдеров типа OVH Kepler).

## Конфиг-файл

`~/.config/llama-runner/config.json` с приоритетом: CLI > конфиг > дефолты.

```json
{
  "host": "127.0.0.1",
  "port": "8080",
  "model": "~/GGUF/Qwen2.5-0.5B-Instruct-Q4_K_M.gguf",
  "alias": "qwen2.5-0.5b",
  "profiles": {
    "auto": {
      "mode": "auto",
      "model": "~/GGUF/Qwen3.5-9B-Agentic-Coder-Q5_K_M.gguf",
      "alias": "auto",
      "parallel": "1",
      "description": "Planner: task decomposition"
    },
    "small": {
      "model": "~/GGUF/Qwen2.5-0.5B-Instruct-Q4_K_M.gguf",
      "alias": "qwen2.5-0.5b",
      "parallel": "1"
    }
  }
}
```

### Профили

| Профиль | Алиас | Модель | Контекст | Parallel | Назначение |
|---------|-------|--------|----------|----------|------------|
| `auto` | `auto` | Qwen3.5-9B-Agentic-Coder | 32768 | 1 | Планировщик |
| `small` | `qwen2.5-0.5b` | Qwen2.5-0.5B-Instruct | 32768 | 1 | Лёгкие задачи |
| `a3b` | `qwen3.8-a3b` | Qwen3.8-35B-A3B | 262144 | 1 | Сложные задачи |

### Меню `/model`

При наличии секции `profiles` меню динамическое:
- **Opus** (Default): `switch-to:<стартовый-алиас>`
- **Sonnet / Fable**: `switch-to:<другой-профиль>`
- **Haiku**: голое имя primary-модели (никогда не триггерит переключение)
- **Custom 1**: `switch-to:<третий-профиль>`

Только записи с `switch-to:` перезапускают llama-server.

## Автоматический режим (Auto Orchestration)

Профиль с `"mode": "auto"` становится планировщиком. Оркестратор исполняет
план с hot-swap между моделями.

**Headless:** `llama-runner -p auto -task "отрефакторить config"`

**Dry-run:** `llama-runner -p auto -task "..." -plan-only > plan.json`

**Интерактивный:** `llama-runner -p auto` (цикл задача → план → исполнение)

**Страховки:**
- `-git-guard` — ветка + чекпоинт на волну + откат
- `-max-retries N` — повторы провалившихся шагов с контекстом ошибки
- `-repo-map` — контекст проекта в промпт планировщика
- `-preflight` — проверка verify-команд и артефактов до исполнения
- `-stream-output` — живой вывод шагов в терминал
- `-resume` — продолжение последнего прогона из журнала

## Автозагрузка моделей

```bash
./llama-runner -download ~/GGUF/https://huggingface.co/org/repo/resolve/main/model.gguf
```

Шардированные модели (`*-00001-of-00004.gguf`) скачивают весь набор.
Докачка через `.partial` файлы. Gated-репозитории: установите `HF_TOKEN`
или `HUGGING_FACE_HUB_TOKEN`.

## Примеры

```bash
# Сервер + Claude Code
./llama-runner -alias qwen2.5-0.5b -claude-dir ~/my-project

# Гибридный режим с токеном прокси
./llama-runner -openai https://api.orcarouter.ai/v1 \
  -key $ORCAROUTER_API_KEY -model-openai deepseek/deepseek-v4-flash-free \
  -proxy-token $(openssl rand -hex 16)

# Auto-режим со всеми страховками
./llama-runner -p auto -git-guard -max-retries 2 -repo-map -preflight \
  -task "отрефакторить пакет config"

# Скачать модель и запустить
./llama-runner -download ~/GGUF/https://huggingface.co/org/repo/resolve/main/model.gguf

# Диагностика
./llama-runner -doctor
```

## Переменные окружения

Передаются Claude Code:
- `ANTHROPIC_BASE_URL` — адрес сервера/прокси
- `ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_API_KEY` — токен прокси или `local-llama`
- `ANTHROPIC_DEFAULT_{OPUS,SONNET,HAIKU,FABLE}_MODEL` — меню `/model`
- `ANTHROPIC_CUSTOM_MODEL_OPTION*` — кастомные записи меню
- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`
- `CLAUDE_CODE_MAX_CONTEXT_TOKENS` — контекст на слот

Алиас разрешается один раз за запуск (`cli.ResolveAlias`), поэтому `--alias`
llama-server, карточка WebUI и запись `/model` Claude Code всегда совпадают.

## Структура репозитория

| Путь | Содержание |
|------|------------|
| `cmd/llama-runner/` | Точка входа исполняемого файла |
| `internal/` | Пакеты реализации |
| `templates/` | Файлы, устанавливаемые `install.sh` |
| `templates/agents/` | `AGENTS.md`, `workflow-policy.md` (EN/RU) |
| `examples/` | `config.example.json` |
| `install.sh` | One-shot installer |
| `AGENTS.md`, `CLAUDE.md` | Инструкции для AI-агентов |

## Разработка

```bash
go build ./...
go vet ./...
gofmt -l .
go test -race -cover ./...
```

CI: gofmt → build → vet → `go test -race -cover`. Версия бинаря внедряется через `-ldflags`.

---

[BSD 3-Clause License](LICENSE) · Copyright © 2026 Skala Skalaz