package cli

import (
	"fmt"
	"io"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// PrintUsageForLocale writes the command-line help to w, selecting the
// language from the console locale: Russian when i18n.IsRussianLocale reports
// true, English otherwise. The parser installs it as flag.FlagSet.Usage, so
// -h and --help automatically print the help in the user's console language.
func PrintUsageForLocale(w io.Writer) {
	if i18n.IsRussianLocale() {
		PrintUsageRU(w)
	} else {
		PrintUsageEN(w)
	}
}

// PrintUsageRU writes the full command-line help in Russian: program purpose,
// all flag groups (core, llama.cpp tuning, speculative decoding, Claude Code,
// config file, llama.cpp update) with defaults, usage examples and a
// networking tip. It is selected by PrintUsageForLocale when the console
// locale is Russian.
func PrintUsageRU(w io.Writer) {
	fmt.Fprintf(w, `llama-runner — лаунчер для llama-server и Claude Code

По умолчанию запускает локальный llama-server и, дождавшись его готовности,
поднимает Claude Code с нужными переменными окружения (ANTHROPIC_BASE_URL и др.).
Когда Claude Code завершается, llama-server автоматически останавливается.

С флагом -server запускается ТОЛЬКО llama-server (прежнее поведение).
С флагом -update обновляется llama.cpp (git pull + сборка + установка), затем выход.

При использовании -webui-mcp-proxy дополнительно запускается
SEARCH_ENABLED=true mcp-search-server --addr :8090.

Использование:
  llama-runner [флаги]

Основные флаги:
  -bin string
        путь к llama-server (по умолчанию "~/.local/bin/llama-server")
  -model string
        путь к GGUF-модели
  -host string
        адрес сервера (по умолчанию "127.0.0.1")
  -port string
        порт сервера (по умолчанию "8080")
  -alias string
        алиас модели (--alias для llama-server и ANTHROPIC_DEFAULT_*_MODEL)
  -v    печатать команду перед запуском
  -version
        показать версию и выйти
  -h, --help
        показать эту справку и выйти

Параметры llama.cpp:
  -c, -b, -ub string   контекст / батч / микро-батч
  -fa string           flash attention (-fa)
  -parallel string     число параллельных слотов
  -tools string        режим инструментов
  -jinja               Jinja-шаблоны
  -ngl string          слоёв на GPU
  -mmproj string       путь к мультимодальному проектору
  -temp, -top-p string сэмплинг
  -cache-type-k/v string типы KV-кэша
  -fit string          режим --fit (on|off)
  -lv string           уровень логов llama.cpp
  -lm-lazy             mmap с ленивым режимом
  -webui-mcp-proxy     CORS-прокси + mcp-search-server
  -reasoning string    режим размышлений: on|off|auto
  -reasoning-budget N  бюджет токенов на размышления: -1/0/N

Спекулятивное декодирование:
  -spec-type string        тип (draft-mtp, draft-dflash)
  -spec-draft-model string путь к драфтеру
  -spec-draft-n-max string макс. число черновых токенов
  -spec-draft-ngl string   слоёв драфтера на GPU

Claude Code:
  -server              только llama-server, без Claude Code
  -claude-bin string   путь к claude (по умолчанию "claude")
  -claude-dir string   рабочая директория claude
  -ready-timeout dur   таймаут готовности /health (по умолчанию 180s)
  -log-file string     файл логов llama-server (по умолчанию "~/.cache/llama-runner/llama-server.log")
  -quiet-server        уводить логи llama-server в файл (по умолчанию true)
  -log-max-mb int      макс. размер лога llama-server в МБ перед ротацией
                       (по умолчанию 50; 0 — не ротировать)
  -log-keep int        сколько архивов лога хранить (по умолчанию 5; 0 — без архивов)
  -mcp-addr string     адрес mcp-search-server (по умолчанию 127.0.0.1:8090)
  -mcp-config string   путь к MCP-конфигу (по умолчанию временный)
  -mcp-strict          передать --strict-mcp-config
  -chat-template str   шаблон чата ("auto" | "" | путь)
  -openai string       URL OpenAI-совместимого API
  -key string          API-ключ внешнего провайдера
  -model-openai str    имя модели у внешнего провайдера
  -proxy-addr string   адрес локального прокси (по умолчанию 127.0.0.1:8082)
  -proxy-token string  токен для проверки входящих запросов прокси (пусто — без проверки)
  -proxy-allow-remote  разрешить не-loopback -proxy-addr без предупреждения

Конфиг:
  -config string       путь к конфиг-файлу (по умолчанию
                       "~/.config/llama-runner/config.json"; "" — не читать)
  -p string            профиль из конфиг-файла (profiles: {имя: {...}})
  -doctor              показать разрешённые пути и версию, не запуская процессы

Обновление llama.cpp:
  -update
        обновить llama.cpp: git pull --ff-only, cmake configure/build/install,
        затем выйти
  -llama-src string
        путь к исходникам llama.cpp (по умолчанию "~/src/llama.cpp")
  -install-prefix string
        префикс для -DCMAKE_INSTALL_PREFIX (по умолчанию "~/.local")
  -cuda
        собирать с CUDA (по умолчанию true)
  -cuda-arch string
        значение для -DCMAKE_CUDA_ARCHITECTURES (по умолчанию "61")

Примеры:
  # Сервер + Claude Code
  llama-runner

  # Только сервер
  llama-runner -server -port 9090

  # MCP-поиск + Claude Code
  llama-runner -webui-mcp-proxy

  # Профиль из конфиг-файла
  llama-runner -p coder

  # Диагностика без запуска процессов
  llama-runner -doctor

  # Обновить llama.cpp
  llama-runner -update -cuda-arch 61

Совет:
  Для доступа из локальной сети используйте -host 0.0.0.0.
`)
}

// PrintUsageEN writes the full command-line help in English — the mirror of
// PrintUsageRU: program purpose, every flag group with its defaults, usage
// examples and a networking tip. It is the default help language and is used
// whenever the console locale is not Russian.
func PrintUsageEN(w io.Writer) {
	fmt.Fprintf(w, `llama-runner — launcher for llama-server and Claude Code

By default it starts a local llama-server and, after waiting for it to be ready,
launches Claude Code with the required environment variables (ANTHROPIC_BASE_URL, etc.).
When Claude Code finishes, llama-server stops automatically.

With -server, ONLY llama-server starts (legacy behavior).
With -update, llama.cpp is updated (git pull + build + install), then exits.

With -webui-mcp-proxy, SEARCH_ENABLED=true mcp-search-server --addr :8090 is also started.

Usage:
  llama-runner [flags]

Main flags:
  -bin string
        path to llama-server (default "~/.local/bin/llama-server")
  -model string
        path to GGUF model
  -host string
        server address (default "127.0.0.1")
  -port string
        server port (default "8080")
  -alias string
        model alias (--alias for llama-server and ANTHROPIC_DEFAULT_*_MODEL)
  -v    print the command before launch
  -version
        show version and exit
  -h, --help
        show this help and exit

llama.cpp parameters:
  -c, -b, -ub string   context / batch / micro-batch
  -fa string           flash attention (-fa)
  -parallel string     number of parallel slots
  -tools string        tool mode
  -jinja               Jinja templates
  -ngl string          layers on GPU
  -mmproj string       path to multimodal projector
  -temp, -top-p string sampling
  -cache-type-k/v string KV-cache types
  -fit string          --fit mode (on|off)
  -lv string           llama.cpp log verbosity
  -lm-lazy             mmap with lazy mode
  -webui-mcp-proxy     CORS-proxy + mcp-search-server
  -reasoning string    reasoning mode: on|off|auto
  -reasoning-budget N  reasoning token budget: -1 (unlimited), 0 (disabled), N (positive)

Speculative decoding:
  -spec-type string        type (draft-mtp, draft-dflash)
  -spec-draft-model string path to draft model
  -spec-draft-n-max string max draft tokens
  -spec-draft-ngl string   draft layers on GPU

Claude Code:
  -server              only llama-server, no Claude Code
  -claude-bin string   path to claude (default "claude")
  -claude-dir string   working directory for claude
  -ready-timeout dur   readiness timeout for /health (default 180s)
  -log-file string     llama-server log file (default "~/.cache/llama-runner/llama-server.log")
  -quiet-server        redirect llama-server logs to file (default true)
  -log-max-mb int      max log size in MB before rotation
                       (default 50; 0 = no rotation)
  -log-keep int        how many log archives to keep (default 5; 0 = no archives)
  -mcp-addr string     mcp-search-server address (default 127.0.0.1:8090)
  -mcp-config string   path to MCP config (default: temp file)
  -mcp-strict          pass --strict-mcp-config
  -chat-template str   chat template ("auto" | "" | path)
  -openai string       OpenAI-compatible API URL
  -key string          external provider API key
  -model-openai str    model name at external provider
  -proxy-addr string   local proxy address (default 127.0.0.1:8082)
  -proxy-token string  proxy auth token for incoming requests (empty = no check)
  -proxy-allow-remote  allow non-loopback -proxy-addr without warning

Config:
  -config string       config file path (default
                       "~/.config/llama-runner/config.json"; "" = don't read)
  -p string            profile from config file (profiles: {name: {...}})
  -doctor              show resolved paths and version without starting processes

llama.cpp update:
  -update
        update llama.cpp: git pull --ff-only, cmake configure/build/install,
        then exit
  -llama-src string
        path to llama.cpp sources (default "~/src/llama.cpp")
  -install-prefix string
        -DCMAKE_INSTALL_PREFIX prefix (default "~/.local")
  -cuda
        build with CUDA (default true)
  -cuda-arch string
        value for -DCMAKE_CUDA_ARCHITECTURES (default "61")

Examples:
  # Server + Claude Code
  llama-runner

  # Server only
  llama-runner -server -port 9090

  # MCP search + Claude Code
  llama-runner -webui-mcp-proxy

  # Profile from config file
  llama-runner -p coder

  # Diagnose without starting processes
  llama-runner -doctor

  # Update llama.cpp
  llama-runner -update -cuda-arch 61

Tip:
  For local network access, use -host 0.0.0.0.
`)
}
