# llama-runner

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://go.dev/)
[![Tests](https://img.shields.io/badge/tests-135%2B-brightgreen.svg)](#разработка)
[![Race](https://img.shields.io/badge/race-clean-brightgreen.svg)](#разработка)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](LICENSE)

[![CI](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml/badge.svg)](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml)

**Русский** · [English](README.md)

Локальный лаунчер: запускает `llama-server` с вашей GGUF-моделью и автоматически открывает
**Claude Code**, привязанный к этому серверу. При закрытии Claude Code сервер останавливается.

## Установка

Одной командой — скачивается готовый бинарь (Go не требуется):

```sh
curl -fsSL https://raw.githubusercontent.com/SkalaSkalolaz/llama-runner/master/install.sh | sh
```

С параметрами (агентские файлы, скачивание модели, конкретная версия):

```sh
curl -fsSL https://raw.githubusercontent.com/SkalaSkalolaz/llama-runner/master/install.sh \
  | sh -s -- --version v1.9.0 --with-agents --with-default-model
```

Полный список флагов — `install.sh --help`. Что делает installer:

- помещает двоичный файл в `~/.local/bin/llama-runner` (`llama-runner.exe` в Windows/Git Bash; переопределить с помощью `--dir`);
- создаёт `~/GGUF/`, `~/.cache/llama-runner/`, `~/.config/llama-runner/` (права 0700) и пишет `~/.config/llama-runner/config.json`, если его нет;
- проверяет SHA-256 бинаря по хешу, встроенному в `install.sh` для этой версии; для latest без встроенного хеша — по `sha256sums.txt` релиза, и только с явным `--allow-unverified-hash` (или `-y`);
- опционально ставит `~/.config/agents/AGENTS.md` и `~/.config/agents/workflow-policy.md` (выбор по локали консоли: `.ru.md` для русской, иначе — обычный `.md`) и дописывает соответствующие `@-ссылки` в `~/.claude/CLAUDE.md` (существующее содержимое сохраняется, дубликаты не появляются);
- опционально скачивает маленькую модель Qwen по умолчанию (`Qwen2.5-0.5B-Instruct-Q4_K_M.gguf`, ~400 МБ) — через `--with-default-model`, ответ «y» на вопрос или `--no-default-model`, чтобы пропустить;
- проверяет наличие `llama-server`, `claude` и `mcp-search-server` и печатает предупреждения, если их нет в PATH (автоматически не устанавливает).

Installer никогда не запускает sudo, не трогает пути за пределами `$HOME` и не перезаписывает существующий `~/.config/llama-runner/config.json`.

**Примечание:** `mcp-search-server` (используется с `-webui-mcp-proxy`) — отдельный проект: https://github.com/SkalaSkalolaz/mcp-search-server. Устанавливайте его отдельно, если нужен веб-поиск. Любой другой MCP-сервер (включая удалённый) подключается через `-mcp-config`.

## Быстрый старт (из исходников)

Сборка из исходников требует Go 1.26. Готовые бинари — см. раздел [«Установка»](#установка) выше.

```bash
git clone https://github.com/SkalaSkalolaz/llama-runner.git
cd llama-runner
go build -o llama-runner ./cmd/llama-runner

./llama-runner                  # llama-server + Claude Code (по умолчанию)
./llama-runner -server          # только llama-server (старое поведение)
./llama-runner -webui-mcp-proxy # + поисковый MCP-сервер и прокси CORS
```

## Что умеет

- **Автоматический запуск:** проверяет наличие модели, поднимает сервер, ждёт готовности
  `/health` и открывает Claude Code.
- **Установка одной командой:** `curl … | sh` с готовыми бинарями для linux/darwin/windows × amd64/arm64, проверкой SHA-256, опциональной установкой глобальных агентских файлов и маленькой модели по умолчанию.
- **Авто-загрузка моделей:** флаг `-download` скачивает GGUF-модель
  (single-file или multi-file/sharded с Hugging Face) в локальную
  директорию перед запуском. Шарды `*-00001-of-NNNNN.gguf`
  распознаются и скачиваются все; точка входа — шард 1.
- **Гибридный режим:** встроенный прокси маршрутизирует запросы Claude Code между локальным
  `llama-server` и внешним OpenAI-совместимым API. Локальная модель использует
  нативный Anthropic API по `/v1/messages`, с сохранением Anthropic-специфики
  (thinking, cache_control, tool_use/tool_result) в неизменном виде. Токен-аутентификация — `-proxy-token`.
- **Автоматический режим:** профиль с `"mode": "auto"` — планировщик:
  разбивает задачу на шаги, для каждого шага выбирает модель из `profiles`
  (по `description`), исполняет план с hot-swap между моделями. Запуск:
  `-p auto -task "..."` (headless) или `-p auto` (интерактивный цикл).
  См. раздел «Автоматический режим».
- **MCP-прокси:** `-webui-mcp-proxy` запускает `mcp-search-server`, генерирует временный
  MCP-конфиг и пробрасывает CORS. С `-mcp-strict` — только этот MCP, без сторонних
  из `~/.claude.json`.
- **Обновление llama.cpp:** `-update` выполняет `git pull --ff-only` → `cmake configure/build/install`.
- **Спекулятивное декодирование:** флаги `-spec-type`, `-spec-draft-model`,
  `-spec-draft-n-max`, `-spec-draft-ngl`.
- **Мультимодальность:** `-mmproj` подключает проектор.
- **Алиас модели:** `-alias` подставляет `--alias` в llama-server и
  `ANTHROPIC_DEFAULT_*_MODEL` в Claude Code.
- **Чат-шаблон:** `-chat-template "auto"` (по имени файла, например
  `qwen-fixed-chat-template.jinja`), явный путь или `""`.
- **Управление глубиной размышлений:** `-reasoning` и `-reasoning-budget` (работает только
  для моделей с поддержкой reasoning).
- **Логи с ротацией:** вывод llama-server уходит в файл (по умолчанию
  `~/.cache/llama-runner/llama-server.log`) и ротируется по размеру.
- **Конфиг-файл:** `~/.config/llama-runner/config.json` (или `-config <path>`). Приоритет:
  CLI > файл > встроенные дефолты. `-config ""` — не читать. Секреты (`openaiKey`,
  `proxyToken`) в файле поддерживаются, но рекомендуется env / `credentials` / флаг.
- **Профили:** `-p small` — короткие пресеты из `profiles: {…}` в конфиге.
  Профиль задаёт модель, алиас и прочие настройки единственной локальной
  модели этого запуска (см. «Меню `/model`»).
- **Пересечение по моделям (per-model parallel):** `parallel` задаётся в каждом
  профиле; `-p a3b` использует значение профиля, `-model <чужой.gguf>` сбрасывает
  в `"1"`.
- **`-doctor`:** показать разрешённые пути, версию и итоговую команду без запуска процессов.
- **Корректная остановка:** Ctrl-C/SIGTERM останавливает claude, сервер и MCP с эскалацией
  SIGTERM → SIGKILL; повторный сигнал ускоряет эскалацию.

## Флаги

### Сервер и llama.cpp

| Флаг | По умолчанию | Что |
|------|-------------|-----|
| `-model` | `~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf` | Путь к GGUF |
| `-bin` | `~/.local/bin/llama-server` | Путь к llama-server |
| `-host` / `-port` | `127.0.0.1` / `8080` | Адрес и порт сервера |
| `-alias` | из `-model`, иначе конфиг/профиль, иначе `local` | Алиас модели — `--alias` llama-server и `ANTHROPIC_DEFAULT_*_MODEL` |
| `-c`, `-b`, `-ub` | `32768` / `2048` / `512` | Контекст / батч / микро-батч (значения из конфига; дефолты CLI — `65536` / `2048` / `1024`) |
| `-fa` | `auto` | Flash attention |
| `-parallel` | `1` | Кол-во параллельных слотов; можно задать per-profile в конфиге (дефолт CLI — `1`) |
| `-tools` | `all` | Режим инструментов |
| `-jinja` | `true` | Jinja-шаблоны |
| `-chat-template` | `auto` | `auto` / `""` / путь к `.jinja` |
| `-ngl` | _(не передаётся)_ | Слоёв на GPU |
| `-mmproj` | _(не передаётся)_ | Мультимодальный проектор |
| `-temp`, `-top-p` | _(не передаётся)_ | Сэмплинг; `temp` задаётся per-profile в примере конфига |
| `-cache-type-k`, `-cache-type-v` | _(не передаётся)_ | Типы KV-кэша |
| `-fit` | _(не передаётся)_ | Режим `--fit` (on/off); в примере конфига `"on"` |
| `-lv` | _(не передаётся)_ | Уровень логов llama.cpp |
| `-reasoning` | _(не передаётся)_ | Режим размышлений: on/off/auto; в примере `"on"` per-profile |
| `-reasoning-budget` | _(не передаётся)_ | Бюджет токенов на размышления: -1/0/N; в примере `"2048"` |
| `-spec-type` | _(не передаётся)_ | Тип spec-декодирования |
| `-spec-draft-model` | _(не передаётся)_ | Путь к драфтеру |
| `-spec-draft-n-max` | _(не передаётся)_ | Макс. черновых токенов |
| `-spec-draft-ngl` | _(не передаётся)_ | Слоёв драфтера на GPU |
| `-lm-lazy` | `false` | mmap с ленивым режимом |
| `-dir` | текущая директория | Рабочая директория llama-server |

### Claude Code

| Флаг | По умолчанию | Что |
|------|-------------|-----|
| `-server` | | Только llama-server, без Claude Code |
| `-claude-bin` | `claude` | Путь к бинарю Claude Code |
| `-claude-dir` | текущая директория | Рабочая директория Claude Code |
| `-ready-timeout` | `180s` | Таймаут ожидания `/health` |
| `-log-file` | `~/.cache/llama-runner/llama-server.log` | Файл логов llama-server |
| `-quiet-server` | `true` | Уводить логи llama-server в файл |
| `-log-max-mb` | `50` | Макс. размер лога в МБ перед ротацией; `0` — не ротировать |
| `-log-keep` | `5` | Сколько архивов лога хранить; `0` — без архивов (трункейт) |
| `-webui-mcp-proxy` | `false` | CORS-прокси + `mcp-search-server` |
| `-mcp-addr` | `127.0.0.1:8090` | Адрес `mcp-search-server` |
| `-mcp-config` | временный файл | Куда записать MCP-конфиг |
| `-mcp-strict` | `false` | Передать `--strict-mcp-config` |

### Внешний провайдер (гибридный режим)

| Флаг | По умолчанию | Что |
|------|-------------|-----|
| `-openai` | | URL OpenAI-совместимого API |
| `-key` | | API-ключ внешнего провайдера (см. «Получение ключа»); `none` или `""` — работать **без ключа** |
| `-model-openai` | | Имя модели у внешнего провайдера |
| `-openai-path` | `/chat/completions` | Суффикс пути к `-openai`; `""` — без суффикса (OVH Kepler, VLM.run) |
| `-proxy-addr` | `127.0.0.1:8082` | Адрес локального маршрутизирующего прокси |
| `-proxy-token` | _(пусто — без проверки)_ | Токен для входящих запросов прокси |
| `-proxy-allow-remote` | `false` | Разрешить не-loopback `-proxy-addr` без предупреждения |

### Конфиг, диагностика, обновление

| Флаг | По умолчанию | Что |
|------|-------------|-----|
| `-config` | `~/.config/llama-runner/config.json` | Путь к конфиг-файлу; `""` — не читать |
| `-download` | _(не задан)_ | Скачать модель перед запуском: `<директория><URL>` как один аргумент, например `~/GGUF/https://huggingface.co/org/repo/resolve/main/model.gguf`. Шардированные модели расширяются до полного набора. После скачивания `-model` указывает на скачанный файл; CLI `-model` игнорируется. Несовместим с `-update`. |
| `-p` | _(не задан)_ | Профиль из конфиг-файла; профиль с `"mode": "auto"` включает автоматический режим |
| `-task` | _(не задан)_ | Задача для планировщика: построить план и исполнить его (только профиль с `"mode": "auto"`) |
| `-plan-only` | | Только построить план и выйти, без исполнения (требует `-task`; только `"mode": "auto"`) |
| `-doctor` | | Диагностика без запуска процессов |
| `-v` | | Показать команду перед запуском |
| `-version` | | Показать версию |
| `-update` | | Обновить llama.cpp (`git pull --ff-only` + сборка + установка) и выйти |
| `-llama-src` | `~/src/llama.cpp` | Путь к исходникам llama.cpp |
| `-install-prefix` | `~/.local` | Префикс установки |
| `-cuda` | `true` | Собирать с CUDA |
| `-cuda-arch` | `61` | `-DCMAKE_CUDA_ARCHITECTURES` |

## Гибридный режим: llama-server + внешний OpenAI API

Можно одновременно использовать локальный llama-server и внешний
OpenAI-совместимый сервис (OrcaRouter, OpenRouter, DeepSeek, …).
Для этого поднимается локальный прокси (по умолчанию `127.0.0.1:8082`,
флаг `-proxy-addr`), на который переводится `ANTHROPIC_BASE_URL` Claude Code.

Маршрутизация по имени модели в Claude Code (`/model`):

- алиас локальной модели **или** токен `llama-server` (в том числе выбор
  кастомной записи из меню `/model`) → локальный llama-server
  (запрос идёт verbatim на `/v1/messages`, нативный Anthropic API; без конвертации
  в OpenAI — Anthropic-специфика, такая как thinking, cache_control, tool_use/tool_result,
  сохраняется в неизменном виде);
- **все остальные** (`sonnet`, `haiku`, `opus`, любое другое имя) → внешний провайдер.

Алиас локальной модели также добавлен в интерактивное меню `/model` через
`ANTHROPIC_CUSTOM_MODEL_OPTION`, чтобы его можно было выбрать стрелками.

```bash
./llama-runner \
  -openai https://api.orcarouter.ai/v1 \
  -key $ORCAROUTER_API_KEY \
  -model-openai deepseek/deepseek-v4-flash-free
```

По умолчанию запросы уходят на `<openai>/chat/completions`. Некоторые
провайдеры (например, OVH Kepler `https://oai.endpoints.kepler.ai.cloud.ovh.net/v1`,
VLM.run) обслуживают endpoint по «голому» базовому URL и отвергают лишний
суффикс `/chat/completions`. Передайте `-openai-path ""`, чтобы запросы шли на
`<openai>` как есть, или `-openai-path /custom/path` для другого суффикса.

### Защита прокси

- `-proxy-token <токен>` — прокси проверяет входящие запросы по
  `Authorization: Bearer` или `x-api-key`; тот же токен передаётся Claude Code
  в `ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_API_KEY`.
- Если прокси слушает не-loopback адрес без токена, запускается предупреждение
  (любой в сети сможет использовать ваш платный ключ). Заданный токен или явный
  `-proxy-allow-remote` убирает его.

## Получение ключа внешнего провайдера

Приоритет источников ключа:

1. флаг `-key`;
2. env `LLAMA_RUNNER_OPENAI_KEY`;
3. env `OPENAI_API_KEY`;
4. файл `~/.config/llama-runner/credentials` — простой `KEY=VALUE`
   (ключи `OPENAI_KEY` или `OPENAI_API_KEY`, поддерживаются `#`-комментарии);
   рекомендуется права `0600` (иначе — предупреждение).

Ключ подхватывается только когда внешний провайдер реально запрошен
(задан хотя бы один из `-openai` / `-key` / `-model-openai`).

## Запуск провайдера без ключа

Некоторые OpenAI-совместимые эндпоинты не требуют аутентификации. Укажите это явно:

```bash
./llama-runner \
  -openai https://oai.endpoints.kepler.ai.cloud.ovh.net/v1 \
  -model-openai Qwen3.6-27B \
  -openai-path "" \
  -key none
```

- `-key none` (любой регистр, пробелы игнорируются) и `-key ""` заставляют запрос к
  провайдеру уйти **без заголовка `Authorization` вообще** — ни пустого, ни `Bearer ` —
  точно такой же, какой отправляет голый `curl -H 'Content-Type: application/json'
  …/chat/completions`. Всё остальное (конвертация Anthropic ↔ OpenAI, стриминг,
  маршрутизация по имени модели, локальный прокси и его `-proxy-token`) работает
  без изменений.
- Это решение, а не пропущенные данные: при `-key none` ни
  `LLAMA_RUNNER_OPENAI_KEY`, ни `OPENAI_API_KEY`, ни файл `credentials` не
  консультируются, поэтому машина с экспортированным ключом всё равно вызовет
  провайдер без аутентификации, когда ему скажут.
- `"openaiKey": "none"` в конфиг-файле или в профиле делает то же самое — так
  провайдер без ключа сохраняется как пресет. **Пустой** `openaiKey` в файле всё
  ещё значит «не настроен, ищи в окружении».
- Значение `none`, найденное в окружении или в файле `credentials`, используется
  дословно как ключ: там хранятся ключи, а не намерение конкретного запуска.
  Компромисс в том, что ключ провайдера, который буквально читается как `none`,
  нельзя передать через командную строку.
- Пропуск `-key` сохраняет старое значение (искать в окружении, затем в `credentials`);
  `-doctor` печатает `не требуется (ключ не передаётся)` для keyless-режима,
  поэтому он никогда не спутается с забытым ключом.

## Профили и меню `/model`

По умолчанию читается `~/.config/llama-runner/config.json`. Приоритет значений:
**CLI > конфиг > встроенные дефолты**. Флаг `-config ""` отключает чтение файла.
В файле доступны все настраиваемые параметры (имена близки к флагам:
`model`, `bin`, `host`, `port`, `c`, `b`, `ub`, `fa`, `ngl`, `jinja`,
`tools`, `parallel`, `mmproj`, `temp`, `topP`, `cacheTypeK`,
`cacheTypeV`, `fit`, `logVerbosity`, `reasoning`, `reasoningBudget`,
`specType`, `specDraftModel`, `specDraftNMax`, `specDraftNGL`, `chatTemplate`,
`chatTemplateFallback`, `logFile`, `logMaxMB`, `logKeep`, `quietServer`,
`mcpAddr`, `mcpStrict`, `webuiMcpProxy`, `alias`, `claudeBin`, `claudeDir`,
`serverOnly`, `readyTimeout` (duration-строка, например `"180s"`), `openai`,
`openaiKey`, `modelOpenai`, `openaiPath`, `proxyAddr`, `proxyToken`, `proxyAllowRemote`,
`llamaSrc`, `installPrefix`, `cuda`, `cudaArch`, `dir`, `verbose`, `lmLazy`,
`mcpConfig`).

```json
{
  "host": "127.0.0.1",
  "port": "8080",
  "jinja": true,
  "readyTimeout": "180s",
  "tools": "all",
  "parallel": "1",

  "logFile": "~/.cache/llama-runner/llama-server.log",
  "logMaxMB": 50,
  "logKeep": 5,

  "chatTemplate": "auto",
  "chatTemplateFallback": "~/GGUF/qwen-fixed-chat-template.jinja",

  "proxyAddr": "127.0.0.1:8082",

  "model": "~/GGUF/Qwen2.5-0.5B-Instruct-Q4_K_M.gguf",
  "alias": "qwen2.5-0.5b",
  "c": "32768",
  "b": "2048",
  "ub": "512",
  "fa": "auto",

  "profiles": {
    "small": {
      "model": "~/GGUF/Qwen2.5-0.5B-Instruct-Q4_K_M.gguf",
      "alias": "qwen2.5-0.5b",
      "c": "32768",
      "b": "2048",
      "ub": "512",
      "fa": "auto",
      "parallel": "1",
      "temp": "0.2",
      "fit": "on",
      "reasoning": "on",
      "reasoningBudget": "2048"
    },
    "a3b": {
      "model": "~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf",
      "alias": "qwen3.8-a3b",
      "c": "262144",
      "b": "4196",
      "ub": "1024",
      "fa": "auto",
      "parallel": "1",
      "temp": "0.2",
      "fit": "on",
      "reasoning": "on",
      "reasoningBudget": "2048"
    }
  }
}
```

`install.sh` пишет ровно этот файл при чистой установке (см. `examples/config.example.json`); отредактируйте `model` и `profiles` под свои GGUF-файлы.

### Профили

| Профиль | Алиас | Модель | Контекст | Parallel | Назначение |
|---------|-------|--------|----------|----------|-----------|
| `auto` | `auto` | Qwen3.5-9B-Agentic-Coder | 32768 | 1 | Планировщик (`mode: "auto"`) |
| `small` | `qwen2.5-0.5b` | Qwen2.5-0.5B-Instruct | 32768 | 1 | Маленькая модель для лёгких задач |
| `a3b` | `qwen3.8-a3b` | Qwen3.8-35B-A3B | 262144 | 1 | Большая MoE-модель для сложных задач |

Профили выбираются флагом `-p <имя>` (они применяют модель, алиас и прочие настройки профиля как единственную рабочую локальную модель). Если в конфиг-файле задана секция `profiles`, они также появляются в меню `/model` как записи `switch-to:<alias-профиля>` — см. «Меню `/model`» ниже.

Заметки:

- отсутствие конфиг-файла по умолчанию-пути — нормально; явно заданный
  `-config <path>` без файла — ошибка;
- `-p <имя>` без конфиг-файла или с неизвестным именем — ошибка (не молчит);
- секреты (`openaiKey`, `proxyToken`) в файле технически поддерживаются, но
  рекомендуется env или `~/.config/llama-runner/credentials`;
- `openaiPath` — суффикс пути, добавляемый к `openai` (по умолчанию
  `"/chat/completions"`); `"openaiPath": ""` отправляет запросы на «голый»
  URL `openai` — именно так ждут OVH Kepler и VLM.run.
- `parallel` можно задавать per-profile; если модель переопределена через `-model`
  на CLI, унаследованный parallel сбрасывается в `"1"`, кроме случая явного
  `-parallel N`.
- профиль с `"mode": "auto"` не применяется как единственная модель:
  он становится планировщиком, остальные профили — исполнителями
  (см. «Автоматический режим»).

### Меню `/model`

Если в конфиг-файле есть секция `profiles`, меню `/model` строится динамически (`cli.buildDynamicMenu`):

- Opus (слот «Default») несёт `switch-to:<стартовый-alias>` — алиас, с которым лаунчер запустился (из `-p`, `-model` или корня конфига);
- Sonnet / Fable несут `switch-to:<alias-другого-профиля>` для первых двух «других» профилей;
- Haiku несёт голое имя primary-модели (`cfg.Alias` в локальном режиме, `cfg.OpenAIModel` в гибридном) и никогда не триггерит переключение — фоновый трафик Claude Code (заголовки сессий, компакция, субагенты) идёт на уже запущенный сервер;
- Custom 1 несёт `switch-to:<третий-другой-профиль>` (появляется, только если задано минимум три «других» профиля).

Для примера конфига (`small` с алиасом `qwen2.5-0.5b`, `a3b` с алиасом `qwen3.8-a3b`) меню выглядит так:

    switch-to:qwen2.5-0.5b      (Opus / Default)
    switch-to:qwen3.8-a3b       (Sonnet)
    qwen2.5-0.5b                (Haiku)
    (пусто)                     (Fable)

Перезапуск llama-server происходит только на записях с префиксом `switch-to:`; голое primary (Haiku) всегда уходит на текущий сервер.

**Известный косметический дефект (исправление запланировано в 1.9.1):** стандартные слоты показывают префикс `switch-to:`. Работает, но планируется показывать там голые имена профилей, а `switch-to:` оставить только в value.

Если секции `profiles` нет, используется статическое меню: все пять позиций несут единственную локальную модель (или, в гибридном режиме, четыре стандартные позиции несут внешнюю модель, а кастомная — локальную). Переменные меню, унаследованные из окружения (`ANTHROPIC_DEFAULT_*_MODEL`, `ANTHROPIC_CUSTOM_MODEL_OPTION*`), удаляются до того, как лаунчер опубликует свои, поэтому вложенный запуск не показывает ghost-записей.

## Автоматический режим (Auto Orchestration)

Профиль с `"mode": "auto"` в `~/.config/llama-runner/config.json` объявляет
себя планировщиком. Планировщик разбивает задачу пользователя на шаги,
для каждого шага выбирает модель из `profiles` (по описанию), и
оркестратор исполняет план: группирует шаги по модели, hot-swap'ает
llama-server между моделями, запускает до `parallel` агентов одной модели
одновременно.

В auto-режиме Claude Code и маршрутизирующий прокси не запускаются:
оркестратор сам владеет моделью.

### Headless: `-task "..."`

```bash
llama-runner -p auto -task "отрефакторить пакет config"
```

Планировщик строит план, оркестратор исполняет его, затем
llama-server останавливается. Ctrl-C отменяет текущий шаг и
останавливает сервер.

### Dry-run: `-plan-only`

```bash
llama-runner -p auto -task "..." -plan-only > plan.json
```

В stdout — строка «План (N шагов):» и сам JSON; все диагностические
строки (включая «Планировщик … строит план …») уходят в stderr. Для
машинной обработки:

```bash
llama-runner -p auto -task "..." -plan-only | sed -n '/^{/,$p' | jq '.steps'
```

sed -n '/^{/,$p' печатает всё начиная с первой { — устойчиво к любым будущим строкам диагностики в stdout; tail -n +2 работает, но ломается от одной лишней строки.

### Интерактивный режим: `-p auto` без `-task`

```bash
llama-runner -p auto
```

llama-server поднимается с планировщиком, затем программа входит в
цикл «задача → план → исполнение»:

```
Задача (или 'exit' для выхода): отрефакторить пакет config
Планировщик auto строит план ... [stderr]
План (3 шагов): [stdout]
Шаг 1/3: ...
...
Задача (или 'exit' для выхода): exit
Выход.
```

- Ctrl-C во время шага отменяет шаг и возвращает к приглашению.
- Ctrl-C на приглашении — выход.
- Ctrl-D (EOF) — тоже выход.
- Строка `exit` или `quit` — выход.
- Пустая строка — повторный запрос без действий.

Поток stdin можно перенаправить из пайпа — тогда после первой задачи
цикл завершится по EOF:

```bash
echo "отрефакторить пакет config" | llama-runner -p auto
```

### Профиль `auto` в конфиге

Пример (см. `examples/config.example.json`):

```json
"auto": {
  "mode": "auto",
  "model": "~/GGUF/Qwen3.5-9B-Agentic-Coder-Q5_K_M.gguf",
  "alias": "auto",
  "parallel": "1",
  "description": "Planner: task decomposition and executor selection"
}
```

Ключ `description` попадает в system prompt планировщика — по нему
планировщик решает, какому профилю отдать каждый шаг. Остальные
профили каталога (`big`, `coder`, …) — исполнители; их `parallel`
ограничивает число одновременно работающих агентов на модели.

## Авто-загрузка моделей

Installer может опционально скачать маленькую модель Qwen по умолчанию (см. раздел [«Установка»](#установка)); флаг `-download` ниже — чтобы указать свою модель.

Флаг `-download` скачивает GGUF-модель с HTTP(S)-URL в локальную директорию
и запускает лаунчер как обычно, с загруженным файлом как модель. Значение —
один аргумент формы `<директория><URL>` — точка раздела — первое вхождение
`http://` или `https://`:

```bash
./llama-runner -download ~/GGUF/https://huggingface.co/AtomicChat/DeepSeek-V4.1-Flash-1017-Uncensored-GSQ-RCO-GGUF/resolve/main/DeepSeek-V4.1-Flash-1017-GSQ-RCO-IQ1_M-00001-of-00004.gguf
```

### Шардированные модели

Hugging Face обслуживает большие GGUF как пронумерованные шарды
(`*-00001-of-00004.gguf`). Передача любого шарда такого набора скачивает
**весь** набор (все `*-NNNNN-of-MMMMM.gguf` братья) и указывает
`llama-server` на шард 1 — llama.cpp загружает остальные сам. Передача шарда,
не первого, вызывает предупреждение, но точка входа всё равно будет шард 1.

### Докачка

Файлы записываются в `<имя>.partial` и переименовываются в `<имя>` по
завершении, поэтому прерванная работа никогда не оставляет «полностью
записанный» файл наполовину. Оставшийся `.partial` докачивается с запросом
`Range` на следующем запуске. Доступчик сначала проверяет `HEAD`: файл уже
существует с ожидаемым размером — переиспользуется без передачи.

### Gated-репозитории

Для gated репозиториев Hugging Face установите `HF_TOKEN` или
`HUGGING_FACE_HUB_TOKEN` в окружении. Токен отправляется как заголовок
`Bearer` и никогда не печатается.

### Взаимодействие с другими флагами

- `-model` на CLI игнорируется, когда задан `-download` (скачанный файл
  выигрывает); `-v` печатает предупреждение.
- `-update` вместе с `-download` — ошибка (exit 2): `-update` никогда не
  использует модель.
- Алиас модели выводится из имени скачиванного GGUF-файла, точно так же,
  как если бы `-model` указывал на него.

## Логи

- В режиме Claude Code вывод llama-server (и MCP-сервера) пишется в
  `~/.cache/llama-runner/llama-server.log` (права `0600`), чтобы не засорять TUI.
- Ротация по размеру: при достижении `-log-max-mb` файл архивируется;
  архивы `<файл>.1` (самый свежий) … `<файл>.<log-keep>`.
  `-log-keep 0` — без архивов (просто очистка), `-log-max-mb 0` — ротация выключена.
- Логи маршрутизирующего прокси — `~/.cache/llama-runner/proxy.log`.

## Примеры

```bash
# Сервер + Claude Code в папке проекта
./llama-runner -alias qwen2.5-0.5b -claude-dir ~/my-project

# Только сервер на другом порту
./llama-runner -server -port 9090

# Сервер + Claude Code с поиском
./llama-runner -webui-mcp-proxy

# Без лишних MCP из ~/.claude.json
./llama-runner -webui-mcp-proxy -mcp-strict

# Гибридный режим с токеном на прокси
./llama-runner -openai https://api.orcarouter.ai/v1 \
  -key $ORCAROUTER_API_KEY -model-openai deepseek/deepseek-v4-flash-free \
  -proxy-token $(openssl rand -hex 16)

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
./llama-runner -p a3b

# Скачать модель с HuggingFace и запустить с ней
./llama-runner -download ~/GGUF/https://huggingface.co/AtomicChat/DeepSeek-V4.1-Flash-1017-Uncensored-GSQ-RCO-GGUF/resolve/main/DeepSeek-V4.1-Flash-1017-GSQ-RCO-IQ1_M-00001-of-00004.gguf

# Диагностика без запуска
./llama-runner -doctor
```

## Переменные окружения, передаваемые Claude Code

`ANTHROPIC_BASE_URL` (адрес сервера или прокси),
`ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_API_KEY` — значение `-proxy-token`,
или `local-llama` если токен не задан,
`ANTHROPIC_DEFAULT_{OPUS,SONNET,HAIKU,FABLE}_MODEL` и `ANTHROPIC_CUSTOM_MODEL_OPTION*`
(всё меню `/model` — см. «Меню `/model`»: в локальном режиме все позиции несут
алиас локальной модели, в гибридном стандартные позиции несут внешнюю модель,
а кастомная запись — локальную). Унаследованные из окружения переменные этих
семейств сначала удаляются, так что от внешней сессии llama-runner ничего не
протечёт.
`CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`.

Алиас за этими переменными разрешается один раз за запуск (`cli.ResolveAlias`),
поэтому `--alias` llama-server, его карточка WebUI и запись `/model` Claude Code
никогда не укажут на разные модели. `-doctor` выводит разрешённое значение вместе
с его источником: `CLI`, `из модели`, `профиль`, `конфиг-файл` или `по умолчанию`.

Примечание: если stdout/stderr llama-runner — не TTY (запуск из пайпа,
`… | tee`), stdio Claude Code перенаправляется в `/dev/tty`, чтобы TUI остался
интерактивным.

## Структура репозитория

| Путь | Содержание |
|------|-----------|
| `cmd/llama-runner/` | точка входа исполняемого файла |
| `internal/` | пакеты реализации (см. комментарии в каждом) |
| `templates/` | файлы, устанавливаемые install.sh в `$HOME` пользователя |
| `templates/agents/` | `AGENTS.md`, `workflow-policy.md` (варианты EN и RU) |
| `templates/claude/CLAUDE.md` | фрагмент для справки; `install.sh` напрямую записывает те же две `@`-references непосредственно |
| `templates/chat-templates/` | `qwen-fixed-chat-template.jinja` |
| `packaging/` | файлы для дистрибутивов (install.sh их не ставит) |
| `packaging/systemd/` | `llama-runner.service` |
| `packaging/man/` | `llama-runner.1` |
| `packaging/completions/` | completion'ы для bash и zsh |
| `examples/` | `config.example.json` — эталонный конфиг, который пишет install.sh |
| `scripts/` | `update-embedded-hashes.py` — CI-помощник для таблицы хешей в install.sh |
| `install.sh` | one-shot installer (также раздаётся через raw.githubusercontent.com) |
| `AGENTS.md`, `CLAUDE.md` | инструкции для AI-агентов, работающих над этим репо |

`AGENTS.md` в корне репозитория — для AI-агентов, работающих над самим llama-runner. Файлы из `templates/agents/` устанавливаются `install.sh` в `~/.config/agents/` для **других** проектов; это намеренно разные файлы.

## Разработка

```bash
go build ./...
go vet ./...
gofmt -l .
go test -v ./...
go test -race ./...
```

CI (GitHub Actions): gofmt → build → vet → `go test -race -cover`; версия
бинаря внедряется через `-ldflags "-X main.version=$(git describe --tags --always --dirty)"`.

Файл `install.sh` проверяется с помощью shellcheck в CI в дополнение к конвейеру Go. Вспомогательные скрипты на Python из папки `scripts/` намеренно исключены из этого этапа.
---

[BSD 3-Clause License](LICENSE) · Copyright © 2026 Skala Skalolaz
