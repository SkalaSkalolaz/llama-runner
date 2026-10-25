# STATE.md — состояние проекта

Рабочий документ к [PLAN.md](PLAN.md). Отмечает готовность по вехам.
Обновляется **тем же коммитом**, что и изменения кода: ставим `[x]` в PLAN.md и
переносим статус вехи здесь + добавляем строку в журнал.

**Последнее обновление:** 2026-10-24 (M0 закрыта) ·
**Готовность: 1 из 8 вех завершено (M0 ✅), M1 — следующая**

---

## Сводка по вехам

| Веха | Название | Статус | Готовность | Блокировано |
|---|---|---|---|---|
| M0 | Гигиена и страховка | ✅ завершена | 7/7 | — |
| M1 | Конвертер запроса Anthropic → OpenAI | ⬜ не начата | 0/8 | разблокирована (M0 закрыта) |
| M2 | Стриминг OpenAI → Anthropic | ⬜ не начата | 0/5 | M1 (`convert.go`) |
| M3 | Ключ, лимиты тела, таймауты | ⬜ не начата | 0/6 | — |
| M4 | Аутентификация прокси | ⬜ не начата | 0/5 | M3 |
| M5 | Сигналы, остановка, cleanup | ⬜ не начата | 0/9 | — |
| M6 | Конфиг-файл, профили, doctor | ⬜ не начата | 0/8 | — |
| M7 | Ротация логов | ⬜ не начата | 0/4 | M5 (омываемый shutdown) |

Легенда: ⬜ не начата · 🟨 в работе · ✅ завершена · ⏸ отложена сознательно.

Порядок выполнения: **M0 → M1 → M2 → M3 → M4 → M5 → M6 → M7**.
M3/M5/M6 независимы от M1/M2 и могут идти параллельно, если нужен другой график.

---

## Baseline до начала работ (замерено 2026-10-24)

```
go vet ./...      → чисто
go test ./...     → ok  server  5.631s
gofmt -l .        → 14 из 14 .go-файлов не отформатированы
```

- `gofmt` «грязные»: `build_test.go`, `config.go`, `helpers_test.go`, `main.go`,
  `main_test.go`, `parse.go`, `parse_test.go`, `proxy.go`, `proxy_test.go`, `run.go`,
  `run_test.go`, `runner.go`, `update.go`, `update_test.go`.
  Причина: часть файлов набрана пробелами вместо табов (`config.go`, `parse.go`,
  `runner.go` целиком; в `run.go` и `proxy.go` — вперемешку).
- `go.mod`: `module server` — расходится с именем бинаря `llama-runner`.
- Версия захардкожена: `main.go:15` = `1.5.2`.
- Тесты проходят, но стриминг (`convertOpenAIStreamToAnthropic`, ~150 строк конечного
  автомата) не покрыт ни одним тестом.

## Состояние после M0 (2026-10-24, ветка `chore/m0-hygiene`)

```
gofmt -l .        → пусто (было 14/14)
go vet ./...      → чисто
go test ./...     → ok  github.com/SkalaSkalolaz/llama-runner
```

Сделано (коммит `07533c4` + незакоммиченные правки M0):

- `gofmt -w .` — все 14 файлов, отдельным коммитом `style:`. `git diff -w` подтверждает:
    только переносы строк, сортировка импортов, пустые строки у комментариев. Логики нет.
- `go.mod`: `module github.com/SkalaSkalolaz/llama-runner`.
- Справка: `llama-runner [флаги]` + 4 примера вызова.
- CI: шаг `gofmt` (падает первым с списком файлов), `fetch-depth: 0` и сборка с
    `-ldflags "-X main.version=$(git describe --tags --always --dirty)"`.
    Дефолт `version = "1.5.2"` в `main.go:15` сохранён.
- `golden_test.go` + `testdata/anthropic/` — каркас golden-тестов (`-update`, `t.Skip()`
    при отсутствии golden), первый fixture `plain_text`, таблица обязательных кейсов M1.
- `scripts/smoke.sh` — написан и **проган: 11/11 автоматически** (формат, vet,
    `test -race`, сборка с ldflags, `-version` → `v1.5.2-4-g07533c4-dirty`, коды возврата
    `-h`=0 / неизвестного флага=2 / `-openai` без `-key`=2 / `-reasoning maybe`=2 /
    отсутствующей модели=1, имя бинаря в справке). Реальные запуски — за `--launch`,
    ручные проверки — чек-листом в конце вывода.

Открытые хвосты M0: push ветки и первый прогон CI (там проверятся шаги `gofmt` и
`build with version`). Код в дереве, коммит — см. журнал.

### Что уже хорошо (не трогать без нужды)

- `CommandRunner` (`runner.go`) — инъекция `exec.Command`/`LookPath` для тестов.
- `trackedProcess` (`main.go:21-39`) — ровно один `Wait()` на дочерний процесс.
- Чистые функции `buildServerArgs` / `buildClaudeEnv` / `validateConfig` — тестируемы.
- CI (`ci.yml`): build + vet + `test -race` + coverage.
- Двойная документация README.md / README_RU.md.

### Подтверждённые дефекты (открыты на 2026-10-24)

| # | Дефект | Место | Веха |
|---|---|---|---|
| 1 | `tool_result` с массивом `content` теряется → tool use на локальной модели ломается | `proxy.go:261-270` | M1 |
| 2 | `system` как массив блоков игнорируется → теряется системный промпт | `proxy.go:220` | M1 |
| 3 | `is_error` в `tool_result` не обрабатывается | `proxy.go:261-270` | M1 |
| 4 | `thinking`/`image` отбрасываются молча (в т.ч. при `-mmproj`) | `proxy.go:246-271` | M1 |
| 5 | Ошибки прокси в `text/plain`, не в формате Anthropic | `proxy.go:85-170` | M1 |
| 6 | Обрыв апстрима до `[DONE]` финализируется как успешный `message_stop` (`scanner.Err()` не проверен) | `proxy.go:451-556` | M2 |
| 7 | `usage` всегда 0 в стриме → контекст-учёт в Claude Code врёт | `proxy.go:431,554` | M2 |
| 8 | `Client.Timeout: 5m` режет длинную генерацию посреди стрима | `proxy.go:29` | M3 |
| 9 | Ключ из `-key` виден в `ps` / `/proc/<pid>/cmdline` | `parse.go:77` | M3 |
| 10 | Нет `MaxBytesReader` на теле запроса | `proxy.go:83,185` | M3 |
| 11 | Прокси без аутентификации; при `-proxy-addr 0.0.0.0` — платный API всем в сети | `proxy.go:25-48` | M4 |
| 12 | Второй Ctrl-C теряется в буфере `sigCh` (cap=1) → Ctrl-C перестаёт работать | `run.go:275-281` | M5 |
| 13 | 8 копий cleanup-блока в `run()` | `run.go:315-460` | M5 |
| 14 | `waitForServer` / `waitForServerProcess` — дубль; `sleep(1s)` после последней попытки | `main.go:42-107` | M5 |
| 15 | Лог `llama-server` растёт бесконечно (`O_APPEND`, без ротации) | `run.go:181-194` | M7 |

Косметика: зашитые личные пути `~/GGUF/...` (`parse.go:26,160`, M6).
Учтено в M0: `module server` → переименован, версия получила ldflags-путь в CI,
справка переименована в `llama-runner`.

---

## Чек-листы готовности по вехам

### M0 — Гигиена и страховка · 7/7 ✅

- [x] `gofmt -w .` одним коммитом, без изменений логики (`07533c4`)
- [x] `gofmt`-проверка в CI (первый прогон — после push)
- [x] `module github.com/SkalaSkalolaz/llama-runner`
- [x] Справка: `llama-runner [флаги]` вместо `server [флаги]` + 4 примера
- [x] Версия через `-ldflags -X main.version` (дефолт `1.5.2` в коде сохранён)
- [x] `scripts/smoke.sh` — проган, 11/11 автоматических проверок пройдены
- [x] Каркас golden-тестов в `testdata/anthropic/` (`plain_text` + `-update` + таблица M1)

Заметки: `gofmt` заодно отсортировал импорты в `parse.go`/`run.go` и раскрыл однострочные
`if` в `buildServerArgs` — это видно в `git diff -w`, но поведение не меняет (тесты зелёные).
`-h` = код 0 и версия из ldflags подтверждены прогоном smoke.

Дальше по плану — **M1 (конвертер запроса)**: первыми идут падающие golden-тесты на
реальных payload'ах Claude Code (`tool_result_blocks`, `system_blocks`, `tool_result_error`,
`thinking`, `image`), и только потом правки `proxy.go`.

### M1 — Конвертер запроса · 0/8

- [ ] Реальные payload'ы собраны в `testdata/` (≥5, включая tool_result-массив и system-массив)
- [ ] `tool_result` с массивом `content` обрабатывается
- [ ] `is_error` → префикс `Error: `
- [ ] `system` массивом блоков склеивается в `role=system`
- [ ] `thinking`/`image`: маппинг либо явный warning (решение задокументировано)
- [ ] Двухпроходная сборка `messages`, нормальный поток байт-в-байт как раньше
- [ ] Ошибки в Anthropic-формате (до начала стрима)
- [ ] `convert.go`, `arguments` остаётся строкой JSON, без `DisallowUnknownFields`

Заметки: ___

### M2 — Стриминг · 0/5

- [ ] ≥5 тестов на `convertOpenAIStreamToAnthropic` (включая обрыв без `[DONE]`)
- [ ] `scanner.Err()` → `event: error`, не фиктивный `message_stop`
- [ ] `stream_options: {"include_usage": true}` + реальный usage в `message_start`/`message_delta`
- [ ] Перенос в `convert.go`
- [ ] Покрытие стриминга в coverage > 0

Заметки: ___

### M3 — Ключ, лимиты, таймауты · 0/6

- [ ] `LLAMA_RUNNER_OPENAI_KEY` / файл `credentials` `0600`
- [ ] Приоритет CLI > env > файл > пусто (с тестом)
- [ ] Ключ не попадает в stdout/stderr/лог (проверено на `-v`)
- [ ] `http.MaxBytesReader` 100 МБ в двух местах
- [ ] `Client.Timeout = 0` + `Transport{ResponseHeaderTimeout: 30s}` + `r.Context()`
- [ ] Длинная генерация не режется на 5-й минуте (проверено вручную)

Заметки: ___

### M4 — Аутентификация прокси · 0/5

- [ ] Проверка токена: `Authorization: Bearer` **и** `x-api-key`
- [ ] Тот же токен в `buildClaudeEnv` (одним коммитом с проверкой)
- [ ] Не-loopback: предупреждение + `-proxy-allow-remote` (не hard-fail)
- [ ] 401 в Anthropic-формате
- [ ] Обычный запуск без новых флагов — поведение прежнее

Заметки: ___

### M5 — Сигналы, остановка, cleanup · 0/9

- [ ] Цикл `for sig := range sigCh`, `signal.Stop` только в `defer` перед `return`
- [ ] Эскалация SIGINT → SIGTERM → SIGKILL (порог 10–15 с)
- [ ] `Setpgid` только для `llama-server` и `mcp-search-server`; `claude` — прямому потомку
- [ ] Единый shutdown-хелпер вместо 8 копий
- [ ] Порядок остановки/ожидания: `claude` → `llama-server` → `mcp` → `proxy`
- [ ] Сохранён единственный `return 2` (`run.go:371`)
- [ ] `run()` разбит на `resolvePaths`/`startServer`/`startProxy`/`startClaude`
- [ ] Один context-aware health-check вместо двух
- [ ] Двойной/тройной Ctrl-C: сирот нет (`pgrep -f llama-server` пуст), интерактив не виснет

Заметки: ___

### M6 — Конфиг, профили, doctor · 0/8

- [ ] `-config` со всеми опциями, приоритет CLI > env > файл > дефолты
- [ ] Профили `-p <name>`
- [ ] Личные пути вынесены в `modelDefault` / `chatTemplateFallback`
- [ ] Автодетект `qwen` **сохранён как дефолт**
- [ ] `doctor` без запуска дочерних процессов
- [ ] Тест: флаг переопределяет конфиг
- [ ] README.md / README_RU.md обновлены
- [ ] `printUsage` обновлён

Заметки: ___

### M7 — Ротация логов · 0/4

- [ ] `io.Pipe` + горутину-писатель (не переименование файла в родителе)
- [ ] `-log-max-mb` + N архивов, права `0600`
- [ ] Документированный `logrotate`-вариант с `copytruncate`
- [ ] Проверено на живом процессе: строки не теряются на границе ротации

Заметки: ___

---

## Зафиксированные решения (что мы НЕ делаем)

Записано, чтобы не вернуть при следующем обходе кода. Подробно — «Красная зона» в PLAN.md.

1. Никакого `Setpgid` для `claude` — интерактивный stdin + не-foreground pgid = `SIGTTIN`
   и зависший UI. `tcsetpgrp` не реализуем.
2. Никакого `signal.Stop` внутри обработчика-цикла — снимет регистрацию, и второй Ctrl-C
   уйдёт в дефолтную обработку с сиротами.
3. Никакого hard-fail на не-loopback `-proxy-addr` — только warning + явный флаг.
4. Не удаляем автоопределение чат-шаблона по подстроке `qwen` — только выносим путь в конфиг.
5. Не ротируем лог переименованием открытого файла — `llama-server` останется писать в
   old inode. Только pipe-писатель или внешний `copytruncate`.
6. Не переходим на `DisallowUnknownFields` в типизированном конвертере — потеряем поля,
   которые сейчас проходят «сквозь».

---

## Журнал

| Дата | Веха | Что сделано | Коммит |
|---|---|---|---|
| 2026-10-24 | — | Написаны PLAN.md и STATE.md; зафиксирован baseline (vet чист, тесты зелёные, gofmt грязные 14/14) | — |
| 2026-10-24 | M0 | `gofmt -w .` — 14 файлов, отдельным коммитом, логики не трогал | `07533c4` |
| 2026-10-24 | M0 | Прогон smoke.sh: 11/11 автоматически, `-version` → `v1.5.2-4-g07533c4-dirty` | — |
| 2026-10-24 | M0 | Ветка `chore/m0-hygiene`: модуль переименован, справка `llama-runner`, CI (gofmt-гейт, `fetch-depth: 0`, ldflags-версия), golden-каркас с fixture `plain_text`, `scripts/smoke.sh`, PLAN/STATE | tip ветки (первый коммит M0 — `07533c4`) |
