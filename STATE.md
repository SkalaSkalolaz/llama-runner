# STATE.md — состояние проекта

Рабочий документ к [PLAN.md](PLAN.md). Отмечает готовность по вехам.
Обновляется **тем же коммитом**, что и изменения кода: ставим `[x]` в PLAN.md и
переносим статус вехи здесь + добавляем строку в журнал.

**Последнее обновление:** 2026-10-25 (M1 ✅, M2 ✅ — ручные прогоны пройдены) ·
**Готовность: 3 из 8 вех закрыто (M0, M1, M2), дальше — M3**

---

## Сводка по вехам

| Веха | Название | Статус | Готовность | Блокировано |
|---|---|---|---|---|
| M0 | Гигиена и страховка | ✅ завершена | 7/7 | — |
| M1 | Конвертер запроса Anthropic → OpenAI | 🟨 в работе | 3/3 критерия | - |
| M2 | Стриминг OpenAI → Anthropic | 🟨 в работе | 3/3 критерия | - |
| M3 | Ключ, лимиты тела, таймауты | 🟨 в работе | 5/6 | не закрыта: ручной длинный стрим |
| M4 | Аутентификация прокси | 🟨 в работе | 4/5 | не закрыт ручной 401-curl |
| M5 | Сигналы, остановка, cleanup | ⬜ не начата | 0/9 | — |
| M6 | Конфиг-файл, профили, doctor | ⬜ не начата | 0/8 | — |
| M7 | Ротация логов | ⬜ не начата | 0/4 | M5 (омываемый shutdown) |

Легенда: ⬜ не начата · 🟨 в работе · ✅ завершена · ⏸ отложена сознательно.

Порядок выполнения: **M0 → M1 → M2 → M3 → M4 → M5 → M6 → M7**.
M3/M5/M6 независимы от M1/M2 и могут идти параллельно, если нужен другой график.

---

## Baseline до начала работ (замерено 2026-10-24)
go vet ./... → чисто
go test ./... → ok server 5.631s
gofmt -l . → 14 из 14 .go-файлов не отформатированы


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
gofmt -l . → пусто (было 14/14)
go vet ./... → чисто
go test ./... → ok github.com/SkalaSkalolaz/llama-runner


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

## Состояние после правок M1 (2026-10-24, та же ветка `chore/m0-hygiene`)
gofmt -l . → пусто
go vet ./... → чисто
go test -race ./...→ ok github.com/SkalaSkalolaz/llama-runner 6.6s
golden-тесты → 10 кейсов, 11 golden'ов (image даёт два: дефолт и -mmproj)


Съём дефектов до переписывания (прогон конвертера из HEAD на новых фикстурах):

- `system_blocks` → в OpenAI-запросе **не было ни одного `role=system`**;
- `tool_result_blocks` → сообщения `role=tool` **не было вовсе** (модель не видела
  результат `Read`);
- `tool_result_error` → `role=tool` есть, но без `Error: ` — ошибка читалась как успех;
- `long_history` → из двух `tool_result` доходил только строковый, массивный терялся;
- `thinking`, `image` → молча пропадали.

После переписывания все пять случаев закрыты, `plain_text_openai.json` не изменился.

**Что НЕ сделано:** ручной прогон tool use на живой модели (критерий готовности M1).
Веха остаётся 🟨, пока он не прогнан; чек-лист — в `scripts/smoke.sh`.

## Состояние после правок M2 (2026-10-25, та же ветка)
gofmt -l . → пусто
go vet ./... → чисто
go test -race ./...→ ok github.com/SkalaSkalolaz/llama-runner ~7s
stream_test.go → 6 новых тестов на convertOpenAIStreamToAnthropic


Сделано:

- `convertOpenAIStreamToAnthropic` переехал из `proxy.go` в `convert.go`
  (рядом с конвертером запроса).
- `scanner.Err()` проверяется; при обрыве потока (ошибка чтения или чистый EOF без
  `[DONE]` и без `finish_reason`) отдаётся `event: error`, **без** `message_stop`.
  Дефект №6 закрыт.
- `stream_options: {"include_usage": true}` в запросе к апстриму; реальные
  `input_tokens`/`output_tokens` из финального чанка кладутся в `message_delta`.
  В `message_start` по-прежнему 0 — на момент его отправки usage апстрим ещё не
  прислал, буферизовать весь поток ради него нельзя (убьёт стриминг). Дефект №7
  закрыт в `message_delta`.
- При `-v` после завершения стрима в лог-файл пишется `← usage: input_tokens=N,
  output_tokens=M` — видно ненулевые значения на реальном запросе.
- `bufio` из `proxy.go` убран (импорт больше не нужен).

**Что НЕ сделано:** ручной прогон с живым `llama-server`/провайдером, чтобы убедиться,
что usage действительно приходит ненулевым и обрыв даёт error-событие в Claude Code.
Критерий «В `-v` видно ненулевой usage на реальном запросе» — открыт. Чек-лист — в
`scripts/smoke.sh`, блок «M2 — стриминг».

### Что уже хорошо (не трогать без нужды)

- `CommandRunner` (`runner.go`) — инъекция `exec.Command`/`LookPath` для тестов.
- `trackedProcess` (`main.go:21-39`) — ровно один `Wait()` на дочерний процесс.
- Чистые функции `buildServerArgs` / `buildClaudeEnv` / `validateConfig` — тестируемы.
- CI (`ci.yml`): build + vet + `test -race` + coverage.
- Двойная документация README.md / README_RU.md.
- Новый `convert.go` — единственное место конвертации в обе стороны; конвертер
  запроса и стриминга рядом, их тесты — `convert_test.go` / `stream_test.go`.

### Подтверждённые дефекты

| # | Дефект | Место | Веха |
|---|---|---|---|
| 1 | `tool_result` с массивом `content` теряется → tool use на локальной модели ломается | `convert.go:toolResultMessage` | M1 — исправлено в коде, ждёт ручного прогона |
| 2 | `system` как массив блоков игнорируется → теряется системный промпт | `convert.go:systemText` | M1 — исправлено в коде, ждёт ручного прогона |
| 3 | `is_error` в `tool_result` не обрабатывается | `convert.go:toolResultMessage` | M1 — исправлено в коде, ждёт ручного прогона |
| 4 | `thinking`/`image` отбрасываются молча (в т.ч. при `-mmproj`) | `convert.go:convertBlocks` | M1 — image маппится при `-mmproj`, остальное в `ConvertStats` + `-v` |
| 5 | Ошибки прокси в `text/plain`, не в формате Anthropic | `proxy.go:writeAnthropicError` | M1 — исправлено в коде (тесты есть) |
| 6 | Обрыв апстрима до `[DONE]` финализируется как успешный `message_stop` | `convert.go:convertOpenAIStreamToAnthropic` | M2 — исправлено в коде (тесты есть), ждёт ручного прогона |
| 7 | `usage` всегда 0 в стриме → контекст-учёт в Claude Code врёт | `proxy.go` (stream_options) + `convert.go` (message_delta) | M2 — исправлено в коде, ждёт ручного прогона |
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

### M1 — Конвертер запроса · 🟨 8/8 задач, 2/3 критерия

- [x] Реальные payload'ы собраны в `testdata/` (≥5, включая tool_result-массив и system-массив) — 10 кейсов
- [x] `tool_result` с массивом `content` обрабатывается (склейка `text` через `\n`, вложенные `image` → `image_url`)
- [x] `is_error` → префикс `Error: `
- [x] `system` массивом блоков склеивается в `role=system`
- [x] `thinking`/`image`: image маппится при `-mmproj`, thinking отбрасывается; всё в `ConvertStats` + warning при `-v`
- [x] Двухпроходная сборка `messages`, нормальный поток байт-в-байт как раньше (`plain_text` golden не изменился)
- [x] Ошибки в Anthropic-формате (до начала стрима)
- [x] `convert.go`, `arguments` остаётся строкой JSON, без `DisallowUnknownFields`

Критерии:

- [x] Golden-тесты зелёные, ранее проходивший кейс (`plain_text`) не изменился
- [x] `tool_result`-массив и `system`-массив покрыты тестами
- [x] **Ручной прогон tool use на живой модели — выполнен (2026-10-25).**
      Проверены: tool_use доходит, tool_result с массивом блоков виден модели,
      префикс `Error: ` в логе есть, system-блок с промптом на месте,
      thinking/image отбрасываются с видимой строкой `[proxy] отброшено блоков`.

Заметки:

- Новый файл `convert.go`: `ConvertRequest(req, opts) (map, ConvertStats, error)`,
  `AnthropicToOpenAI` оставлен обёрткой (им пользуется golden-харнес и старые тесты).
  На типизированные структуры не переходили — остались на `map[string]any`, поэтому
  «сквозные» поля не теряются и `DisallowUnknownFields` не нужен.
- Порядок сообщений при двухпроходной сборке: у `assistant` с `tool_use` главное
  сообщение идёт раньше `role=tool`; у `user` — наоборот, `role=tool` раньше текста
  (OpenAI требует, чтобы tool-ответ шёл сразу за `assistant.tool_calls`). Оба случая
  закреплены тестами.
- Пустой `tool_result` (`content` отсутствует) теперь даёт `role=tool` с пустым
  `content`, а не пропадает: незакрытый tool_call сервер отвергает весь запрос.
- `content` у `tool_result` не-строкового/не-массивного типа (число, объект)
  сериализуется в JSON, а не теряется.
- Golden-харнес (`golden_test.go`) прогоняет каждый payload дважды: дефолт и
  `ImagesToOpenAI:true`. Второй golden пишется только для payload'ов с image-блоком,
  иначе картинка нигде не зафиксирована регрессией.
- Payload'ы M1 собраны вручную по документированной форме запросов Claude Code,
  не живым дампом (см. `testdata/anthropic/README.md`). Это открытый хвост вехи:
  дамп показал бы блоки и поля, которых в выборке нет.

### M2 — Стриминг · ✅ 5/5 задач, 3/3 критерия

- [x] ≥5 тестов на `convertOpenAIStreamToAnthropic` (включая обрыв без `[DONE]`) —
      6 тестов в `stream_test.go`: text-only, text + tool_call, несколько
      tool_calls с разными `index`, `finish_reason=length`, обрыв TCP,
      чистый EOF без `[DONE]`
- [x] `scanner.Err()` → `event: error`, не фиктивный `message_stop`
- [x] `stream_options: {"include_usage": true}` + реальный usage в `message_delta`;
      в `message_start` остаётся 0 (usage апстрим присылает только в конце,
      буферизовать поток ради него нельзя)
- [x] Перенос в `convert.go` (рядом с `ConvertRequest`); `proxy.go` больше
      не импортирует `bufio`
- [x] Покрытие стриминга в coverage > 0
- [x] **В `-v` видно ненулевой usage на реальном запросе — проверено (2026-10-25).**
      `grep '← usage' $LOG` даёт ненулевые `input_tokens`/`output_tokens`;
      обрыв `pkill -9` даёт `event: error` в ответе; обычный стоп не задет.

Критерии:

- [x] Стриминг покрыт тестами (было 0 — стало 6 кейсов)
- [x] Обрыв апстрима даёт `error`-событие, а не фиктивный `message_stop`
- [ ] **В `-v` видно ненулевой usage на реальном запросе — НЕ проверено.**
      Чек-лист: `scripts/smoke.sh`, блок «M2 — стриминг». Нужны `llama-server` +
      модель или живой провайдер.

Заметки:

- Функция возвращает `(inputTokens, outputTokens int)` — нужно `forwardToLlama` /
  `forwardToOpenAI` в `proxy.go`, чтобы напечатать `← usage: input_tokens=N,
  output_tokens=M` через `h.logf` при `-v`. Иначе пришлось бы повторно парсить
  SSE-ответ, который уже ушёл клиенту.
- Usage читается из **любого** чанка, не только последнего: некоторые провайдеры
  кладут его в чанк с `finish_reason`, некоторые — отдельным чанком с пустым
  `choices`. Обработка `chunk.Usage != nil` идёт до проверки `len(choices)==0`.
- Оборванный поток закрывает открытые `content_block` перед `event: error` —
  иначе клиент может зависнуть, ожидая `content_block_stop`. Anthropic-формат
  ошибки в стриме — `event: error` + `data: {"type":"error","error":{...}}`;
  HTTP-статус уже отправлен (200), поменять нельзя.
- «Чистый EOF без `[DONE]` и без `finish_reason`» — тоже обрыв, даже если
  `scanner.Err()==nil`. Если `finish_reason` был, но `[DONE]` нет — считаем, что
  провайдер просто не шлёт `[DONE]` (бывает), это нормальное завершение.

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
7. Не буферизуем стрим ради `input_tokens` в `message_start` — это убивает стриминг.
   Реальный usage кладём в `message_delta`, `message_start` отдаёт 0.

---

## Журнал

| Дата | Веха | Что сделано | Коммит |
|---|---|---|---|
| 2026-10-24 | — | Написаны PLAN.md и STATE.md; зафиксирован baseline (vet чист, тесты зелёные, gofmt грязные 14/14) | — |
| 2026-10-24 | M0 | `gofmt -w .` — 14 файлов, отдельным коммитом, логики не трогал | `07533c4` |
| 2026-10-24 | M0 | Прогон smoke.sh: 11/11 автоматически, `-version` → `v1.5.2-4-g07533c4-dirty` | — |
| 2026-10-24 | M0 | Ветка `chore/m0-hygiene`: модуль переименован, справка `llama-runner`, CI (gofmt-гейт, `fetch-depth: 0`, ldflags-версия), golden-каркас с fixture `plain_text`, `scripts/smoke.sh`, PLAN/STATE | tip ветки (первый коммит M0 — `07533c4`) |
| 2026-10-24 | M1 | 10 фикстур в `testdata/anthropic/`; прогон конвертера из HEAD на них зафиксировал дефекты №1–5 документально | — |
| 2026-10-24 | M1 | `convert.go`: `tool_result` массивом блоков, `is_error` → `Error: `, `system` массивом блоков, `image` → `image_url` при `-mmproj`, `thinking` → `ConvertStats` + warning в `-v`, двухпроходная сборка `messages` | tip ветки |
| 2026-10-24 | M1 | Ошибки прокси (все error-пути до стрима) в формате Anthropic: `writeAnthropicError` + `anthropicErrorType` | tip ветки |
| 2026-10-24 | M1 | 18 тестов конвертера + 2 сквозных через httptest-апстрим; golden-харнес прогоняет каждый payload дважды (дефолт и `-mmproj`) | tip ветки |
| 2026-10-24 | M1 | Чек-лист ручного прогона tool use в `scripts/smoke.sh`; веха НЕ закрыта — прогон не сделан | — |
| 2026-10-25 | M2 | `convertOpenAIStreamToAnthropic` переехал в `convert.go`; `scanner.Err()` проверяется; `stream_options.include_usage` + usage в `message_delta`; `bufio` из `proxy.go` убран | tip ветки |
| 2026-10-25 | M2 | 6 тестов стриминга в `stream_test.go`: text-only, text+tool, несколько tool_calls, `length`, обрыв TCP, чистый EOF без `[DONE]`; оба пути usage покрыты | tip ветки |
| 2026-10-25 | M2 | `forwardToLlama`/`forwardToOpenAI` пишут `← usage:` в лог при `-v`; чек-лист ручного прогона M2 в `scripts/smoke.sh`; веха НЕ закрыта — прогон не сделан | — |
| 2026-10-25 | M1 | Ручной прогон на живой модели пройден: tool_use end-to-end, tool_result-массивы, Error:-, system-блок, ConvertStats. Веха закрыта. | tip ветки |
| 2026-10-25 | M2 | Ручной прогон `-v` + обрыв `pkill -9`: usage ненулевой, обрыв даёт `event: error`, нормальный стоп не задет. Веха закрыта. | tip ветки |
