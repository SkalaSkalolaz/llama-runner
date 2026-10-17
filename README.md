# llama-runner

[![Go](https://img.shields.io/badge/Go-1.24-00ADD8.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](LICENSE)

Локальный лаунчер для **llama-server** и **Claude Code**.

По умолчанию запускает локальный `llama-server` с вашей GGUF-моделью, дождавшись его готовности автоматически поднимает **Claude Code**, привязанный к этому серверу. Когда Claude Code закрывается — `llama-server` останавливается.

С флагом `-server` запускается **только** `llama-server` (прежнее поведение).

---

## Установка

```bash
go build -o server .
```

---

## Быстрый старт

```bash
# Сервер + Claude Code с дефолтными параметрами
./server

# Только llama-server (старое поведение)
./server -server

# Сервер + Claude Code + автопоиск через mcp-search-server
./server -webui-mcp-proxy
```

---

## Флаги

### Основные

| Флаг | Значение по умолчанию | Описание |
|------|----------------------|----------|
| `-model string` | `~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf` | Путь к GGUF-модели (поддерживает `~`) |
| `-bin string` | `~/.local/bin/llama-server` | Путь к исполняемому файлу llama-server |
| `-host string` | `127.0.0.1` | Адрес сервера |
| `-port string` | `8080` | Порт сервера |
| `-dir string` | _(текущая)_ | Рабочая директория для llama-server |
| `-alias string` | _(auto: `"local"`)_ | Алиас модели: `--alias` для llama-server и `ANTHROPIC_DEFAULT_*_MODEL` для Claude Code |

### Параметры llama.cpp

| Флаг | Значение по умолчанию | Описание |
|------|----------------------|----------|
| `-c, -b, -ub string` | `65536` / `2048` / `1024` | Размер контекста / батча / микро-батча |
| `-fa string` | `auto` | Flash attention (`-fa`) |
| `-parallel string` | `1` | Число параллельных слотов |
| `-tools string` | `all` | Режим инструментов (`--tools`) |
| `-jinja` | `true` | Включить Jinja-шаблоны чата (`--jinja`) |
| `-lm-lazy` | `false` | Mmap с ленивым режимом (`-lm mmap --lazy-mode on`) |
| `-webui-mcp-proxy` | `false` | Включить CORS-прокси и запустить mcp-search-server (`--webui-mcp-proxy`) |
| `-ngl string` | _(не передаётся)_ | Число слоёв модели на GPU (`-ngl`), напр. `999` |
| `-mmproj string` | _(не передаётся)_ | Путь к мультимодальному проектору (`--mmproj`, можно с `~`) |
| `-temp string` | _(не передаётся)_ | Температура сэмплинга (`--temp`) |
| `-top-p string` | _(не передаётся)_ | Top-p сэмплинг (`--top-p`) |
| `-cache-type-k string` | _(не передаётся)_ | Тип K-кэша (`--cache-type-k`), напр. `q8_0` |
| `-cache-type-v string` | _(не передаётся)_ | Тип V-кэша (`--cache-type-v`), напр. `q8_0` |

### Спекулятивное декодирование

| Флаг | Значение по умолчанию | Описание |
|------|----------------------|----------|
| `-spec-type string` | _(не передаётся)_ | Тип спекулятивного декодирования (`--spec-type`), напр. `draft-dflash` |
| `-spec-draft-model string` | _(не передаётся)_ | Путь к драфтер-модели (`--spec-draft-model`, можно с `~`) |
| `-spec-draft-n-max string` | _(не передаётся)_ | Макс. число черновых токенов драфтера (`--spec-draft-n-max`) |
| `-spec-draft-ngl string` | _(не передаётся)_ | Число слоёв драфтера на GPU (`--spec-draft-ngl`) |

### Обновление llama.cpp

| Флаг | Значение по умолчанию | Описание |
|------|----------------------|----------|
| `-update` | `false` | Обновить llama.cpp: `git pull --ff-only`, `cmake configure/build/install`, затем выйти (без запуска сервера) |
| `-llama-src string` | `~/src/llama.cpp` | Путь к исходникам llama.cpp (для `-update`, можно с `~`) |
| `-install-prefix string` | `~/.local` | Префикс для `-DCMAKE_INSTALL_PREFIX` (для `-update`, можно с `~`) |
| `-cuda` | `true` | Собирать с CUDA (`-DGGML_CUDA=ON`) (для `-update`) |
| `-cuda-arch string` | `61` | Архитектура CUDA (`-DCMAKE_CUDA_ARCHITECTURES`, для `-update`). `""` — не передавать флаг (CMake выберет default) |

### Флаги Claude Code

| Флаг | Описание |
|------|----------|
| `-server` | Запустить **только** llama-server, без Claude Code |
| `-claude-bin string` | Путь к исполняемому файлу Claude Code (по умолчанию `claude`, ищется в PATH) |
| `-claude-dir string` | Рабочая директория для Claude Code |
| `-ready-timeout duration` | Таймаут ожидания `/health` перед запуском Claude Code (по умолчанию `180s`) |
| `-log-file string` | Файл для логов llama-server и mcp-search-server (по умолчанию `~/.cache/llama-runner/llama-server.log`) |
| `-quiet-server` | Уводить логи llama-server в файл (по умолчанию `true`) |
| `-mcp-addr string` | Адрес mcp-search-server `host:port` (по умолчанию `127.0.0.1:8090`) |
| `-mcp-config string` | Путь к временному MCP-конфигу для Claude Code (по умолчанию — во временный каталог ОС) |
| `-mcp-strict` | Передать Claude Code `--strict-mcp-config`: использовать только MCP из лаунчера |
| `-chat-template string` | Шаблон чата (по умолчанию `"auto"`). `"auto"` — если имя файла модели содержит "qwen" (без учёта регистра), берёт `~/GGUF/qwen-fixed-chat-template.jinja`; иначе не передаёт шаблон. Явный путь — использовать его. Явный `""` — без шаблона. |

### Прочее

| Флаг | Описание |
|------|----------|
| `-v` | Печатать полную команду и рабочую директорию перед запуском |
| `-version` | Показать версию (`1.1`) и выйти |
| `-h, --help` | Показать справку и выйти |

---

## Примеры

```bash
# Сервер + Claude Code с дефолтными параметрами
./server

# Сервер + Claude Code, алиас модели и рабочая папка проекта
./server -alias qwen3-coder -claude-dir ~/my-project

# Только сервер (как раньше)
./server -server

# Только сервер на другом порту
./server -server -port 9090

# Посмотреть, что будет выполнено
./server -v -claude-dir ~/work

# Запуск с прокси для MCP-серверов и поисковым MCP-сервером
./server -webui-mcp-proxy -server

# Сервер + Claude Code с автоматически подключённым web_search
./server -webui-mcp-proxy

# То же, но не примешивать другие MCP-серверы из ~/.claude.json
./server -webui-mcp-proxy -mcp-strict
```

### Обновление llama.cpp

```bash
# Обновить llama.cpp (архитектура по умолчанию — sm_61, Pascal) и выйти
./server -update

# Обновить для Turing (sm_75)
./server -update -cuda-arch 75

# Обновить без явной архитектуры (CMake выберет default)
./server -update -cuda-arch ""

# Обновить и сразу запустить сервер
./server -update && ./server -server -model ~/GGUF/model.gguf
```

### Спекулятивное декодирование (LiLiCorr)

```bash
# Сервер + Claude Code с LiLiCorr-драфтером
./server -server \
  -model  ~/models/nemotron-bolt/NVIDIA-Nemotron-3.5-Bolt-74B-A9B-24GB.gguf \
  -mmproj ~/models/nemotron-bolt/mmproj-NVIDIA-Nemotron-3.5-Bolt-74B-A9B-24GB.gguf \
  -spec-type draft-dflash \
  -spec-draft-model ~/models/nemotron-bolt/NVIDIA-Nemotron-3.5-Bolt-74B-A9B-24GB-LiLiCorr.gguf \
  -spec-draft-n-max 6 -spec-draft-ngl 999 -ngl 999 \
  -c 98304 -b 2048 -ub 512 \
  -cache-type-k q8_0 -cache-type-v q8_0 \
  -temp 1.0 -top-p 0.95 -fa on -jinja -chat-template ""
```

### Немного о мультимодальности

```bash
# Сервер + Claude Code с мультимодальной моделью (vision)
./server -server \
  -model  ~/models/llama-3.2-11B-Vision-instruct-fp8.gguf \
  -mmproj  ~/models/llama-3.2-11B-Vision-instruct-mmproj-f16.gguf
```

---

## Режимы работы

### Сервер + Claude Code (по умолчанию)

1. Проверяет, что `llama-server` и модель существуют на указанных путях
2. Запускает `llama-server` с указанными параметрами
3. Ожидает готовности по `/health` (таймаут по умолчанию 180 с)
4. Если задан `-webui-mcp-proxy` — поднимает `mcp-search-server` и генерирует временный MCP-конфиг
5. Запускает Claude Code с переменными `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_API_KEY`, `ANTHROPIC_DEFAULT_*_MODEL`
6. При выходе Claude Code или сигнале SIGINT/SIGTERM останавливает все дочерние процессы (Claude Code → llama-server → mcp-search-server)

### Обновление llama.cpp

```bash
./server -update
```

Выполняет: `git pull --ff-only` → `cmake configure` (с `-DGGML_CUDA=ON` и заданной архитектурой) → `cmake build` (на все ядра) → `cmake install`. Сервер **не** запускается.

### Только llama-server

```bash
./server -server
```

Работает как раньше: запускает только `llama-server`, при `Ctrl+C` останавливает его.

---

## Переменные окружения

Программа передаёт Claude Code:

| Переменная | Значение |
|------------|----------|
| `ANTHROPIC_BASE_URL` | `http://<host>:<port>` |
| `ANTHROPIC_API_KEY` | `local-llama` |
| `ANTHROPIC_AUTH_TOKEN` | `local-llama` |
| `ANTHROPIC_DEFAULT_*_MODEL` | алиас модели (если задан `-alias`) |
| `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` | `1` |

---

## Совет

Если сервер должен быть доступен из локальной сети, укажите `-host 0.0.0.0`. Значение `127.0.0.1` принимает соединения только с этой же машины.

---

## Лицензия

[BSD 3-Clause License](LICENSE)

Copyright (c) [2026] [Skala Skalolaz]
