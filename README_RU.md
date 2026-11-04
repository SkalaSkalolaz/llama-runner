# llama-runner

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://go.dev/)
[![Tests](https://img.shields.io/badge/tests-135%2B-brightgreen.svg)](#разработка)
[![Race](https://img.shields.io/badge/race-clean-brightgreen.svg)](#разработка)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](LICENSE)

[![CI](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml/badge.svg)](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml)

**Русский** · [English](README.md)

Локальный лаунчер: запускает `llama-server` с вашей GGUF-моделью и автоматически открывает
**Claude Code**, привязанный к этому серверу. При закрытии Claude Code сервер останавливается.

## Быстрый старт

```bash
go build -o llama-runner ./cmd/llama-runner

./llama-runner                  # llama-server + Claude Code (по умолчанию)
./llama-runner -server          # только llama-server (старое поведение)
./llama-runner -webui-mcp-proxy # + поисковый MCP-сервер и прокси CORS
```

## Что умеет

- **Автоматический запуск:** проверяет наличие модели, поднимает сервер, ждёт готовности
  `/health` и открывает Claude Code.
- **Гибридный режим:** встроенный прокси маршрутизирует запросы Claude Code между локальным
  `llama-server` и внешним OpenAI-совместимым API (с конвертацией Anthropic → OpenAI
  и стримингом обратно). Токен-аутентификация — `-proxy-token`.
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
- **Профили:** `-p coder` — короткие пресеты из `profiles: {…}` в конфиге.
  Профиль задаёт модель, алиас и прочие настройки единственной локальной
  модели этого запуска (см. «Меню `/model`»).
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
| `-c`, `-b`, `-ub` | `65536` / `2048` / `1024` | Контекст / батч / микро-батч |
| `-fa` | `auto` | Flash attention |
| `-parallel` | `1` | Кол-во параллельных слотов |
| `-tools` | `all` | Режим инструментов |
| `-jinja` | `true` | Jinja-шаблоны |
| `-chat-template` | `auto` | `auto` / `""` / путь к `.jinja` |
| `-ngl` | _(не передаётся)_ | Слоёв на GPU |
| `-mmproj` | _(не передаётся)_ | Мультимодальный проектор |
| `-temp`, `-top-p` | _(не передаются)_ | Сэмплинг |
| `-cache-type-k`, `-cache-type-v` | _(не передаются)_ | Типы KV-кэша |
| `-fit` | _(не передаётся)_ | Режим `--fit` (`on`/`off`) |
| `-lv` | _(не передаётся)_ | Уровень логов llama.cpp |
| `-lm-lazy` | `false` | mmap с ленивым режимом |
| `-reasoning` | _(не передаётся)_ | Режим размышлений: `on`/`off`/`auto` |
| `-reasoning-budget` | _(не передаётся)_ | Бюджет токенов на размышления: `-1`/`0`/`N` |
| `-spec-type` | _(не передаётся)_ | Тип spec-декодирования (`draft-mtp`, `draft-dflash`) |
| `-spec-draft-model` | _(не передаётся)_ | Путь к драфтеру |
| `-spec-draft-n-max`, `-spec-draft-ngl` | _(не передаются)_ | Параметры драфтера |
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
| `-p` | _(не задан)_ | Профиль из конфиг-файла |
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
  (запрос конвертируется Anthropic → OpenAI, стрим возвращается обратно);
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
`tools`, `parallel`, `temp`, `topP`, `cacheTypeK/V`, `fit`, `logVerbosity`,
`reasoning`, `reasoningBudget`, `specType`, `specDraftModel`,
`specDraftNMax`, `specDraftNGL`, `mmproj`, `chatTemplate`,
`chatTemplateFallback`, `logFile`, `logMaxMB`, `logKeep`, `quietServer`,
`mcpAddr`, `mcpStrict`, `webuiMcpProxy`, `alias`, `claudeBin`, `claudeDir`,
`serverOnly`, `readyTimeout` (duration-строка, например `"180s"`), `openai`,
`openaiKey`, `modelOpenai`, `openaiPath`, `proxyAddr`, `proxyToken`, `proxyAllowRemote`,
`llamaSrc`, `installPrefix`, `cuda`, `cudaArch`, `dir`, `verbose`, `lmLazy`,
`mcpConfig`).

```json
{
  "model": "~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf",
  "host": "127.0.0.1",
  "port": "8080",
  "jinja": true,
  "readyTimeout": "180s",
  "chatTemplate": "auto",
  "chatTemplateFallback": "~/GGUF/qwen-fixed-chat-template.jinja",
  "openaiPath": "/chat/completions",
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

Заметки:

- отсутствие конфиг-файла по умолчанию-пути — нормально; явно заданный
  `-config <path>` без файла — ошибка;
- `-p <имя>` без конфиг-файла или с неизвестным именем — ошибка (не молчит);
- секреты (`openaiKey`, `proxyToken`) в файле технически поддерживаются, но
  рекомендуется env или `~/.config/llama-runner/credentials`;
- `openaiPath` — суффикс пути, добавляемый к `openai` (по умолчанию
  `"/chat/completions"`); `"openaiPath": ""` отправляет запросы на «голый»
  URL `openai` — именно так ждут OVH Kepler и VLM.run.

### Меню `/model`

В локальном режиме все пять позиций (Opus, Sonnet, Haiku, Fable и кастомная
запись) несут единственную локальную модель — выбранную через `-model` / `-p`
или дефолтную из конфиг-файла.

В гибридном режиме (`-openai` + `-model-openai` + ключ или `-key none`)
четыре стандартных позиции несут внешнюю модель, а кастомная запись — локальную.
Выбор кастомной записи направляет запрос в llama-server через прокси.

Профили из конфиг-файла по-прежнему выбираются флагом `-p <имя>` (они
применяют модель, алиас и прочие настройки профиля как единственную рабочую
локальную модель), но в меню отдельными записями больше не показываются.

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
./llama-runner -alias qwen3-coder -claude-dir ~/my-project

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
./llama-runner -p coder

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

---

[BSD 3-Clause License](LICENSE) · Copyright © 2026 Skala Skalolaz
