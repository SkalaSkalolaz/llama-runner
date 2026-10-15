# llama-runner

Локальный лаунчер для **llama-server** и **Claude Code**.

По умолчанию запускает локальный `llama-server` с вашей GGUF-моделью, дождавшись его готовности автоматически поднимает **Claude Code**, привязанный к этому серверу. Когда Claude Code закрывается — `llama-server` останавливается.

С флагом `-server` запускается **только** `llama-server` (прежнее поведение).

---

## Установка

```bash
go build -o server .
```

Переопределить версию при сборке (по умолчанию `"1.0"`):

```bash
go build -ldflags="-X main.version=1.2.0" -o server .
```

Проверить версию:

```bash
./server --version
# 1.0
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

| Флаг | Описание |
|------|----------|
| `-c, -b, -ub string` | Размер контекста / батча / микро-батча |
| `-fa string` | Flash attention (`-fa`, по умолчанию `auto`) |
| `-parallel string` | Число параллельных слотов |
| `-tools string` | Режим инструментов (по умолчанию `all`) |
| `-jinja` | Включить Jinja-шаблоны чата |
| `-lm-lazy` | Mmap с ленивым режимом |
| `-webui-mcp-proxy` | Включить CORS-прокси и запустить mcp-search-server |

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
| `-chat-template string` | Путь к файлу Jinja-шаблона чата (по умолчанию `~/GGUF/qwen-fixed-chat-template.jinja`) |

### Прочее

| Флаг | Описание |
|------|----------|
| `-v` | Печатать полную команду перед запуском |
| `-h, --help` | Показать справку |
| `--version` | Показать версию и выйти |

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

---

## Режимы работы

### Сервер + Claude Code (по умолчанию)

1. Запускает `llama-server` с указанными параметрами
2. Ожидает готовности по `/health` (таймаут по умолчанию 180 с)
3. Запускает Claude Code с переменными `ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY`, `ANTHROPIC_DEFAULT_*_MODEL`
4. Если задан `-webui-mcp-proxy` — дополнительно поднимает `mcp-search-server` и генерирует временный MCP-конфиг
5. При выходе Claude Code или сигнале SIGINT/SIGTERM останавливает все дочерние процессы

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

## Лицензия

MIT
