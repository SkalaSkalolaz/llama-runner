# PLAN.md — план усовершенствования llama-runner

**Как пользоваться:** каждый пункт — чекбокс. `[x]` = пункт реализован, проверен и
закоммичен. `[ ]` = не сделан. Пункт помечается `[x]` только когда выполнены **все**
пункты раздела «Критерий готовности» этой вехи. Прогресс по вехам зеркалится в
[STATE.md](STATE.md) — его обновляем тем же коммитом.

Текущая готовность: **3 / 8 вех** (см. STATE.md).
---

## Базовые факты на старте (замерено 2026-10-24)

| Проверка | Результат |
|---|---|
| `go vet ./...` | чисто |
| `go test ./...` | `ok server 5.631s` |
| `gofmt -l .` | **все 14 `.go`-файлов** не отформатированы |
| `go.mod` | `module server`, `go 1.26.8` |
| Версия | захардкожена: `main.go:15` → `1.5.2` |
| Покрытие стриминга | `convertOpenAIStreamToAnthropic` (`proxy.go:408-557`) — 0 тестов |

Порядок вех выбран так, чтобы после **каждого** коммита программа оставалась рабочей.

---

## 🔴 Красная зона: что нельзя делать буквально

Эти пять пунктов предыдущего обсуждения при буквальной реализации ломают интерактивный
запуск. Они уже учтены в формулировках задач ниже — здесь они собраны как стоп-сигнал
для ревью.

1. **`SysProcAttr{Setpgid: true}` для `claude`.** Интерактивный `claude` читает stdin;
   в не-foreground pgid он получит `SIGTTIN` на первом же `read(0)` и встанет на паузу.
   → `Setpgid` **только** для `llama-server` и `mcp-search-server` (терминал не читают).
   Для `claude` — сигналим прямому потомку, как сейчас. `tcsetpgrp` не трогаем.
2. **`signal.Stop()` «после первого обработанного сигнала» вместе с `for sig := range sigCh`.**
   `Stop` снимает регистрацию → следующего сигнала в канал не придёт → дефолтная обработка
   убьёт родителя с сиротами. → Либо цикл, либо `Stop`. `signal.Stop` — только перед
   `return` из `run()`.
3. **Hard-fail при `-proxy-addr` на не-loopback.** Сейчас у кого-то это работает.
   → Предупреждение + явный `-proxy-allow-remote`, а не отказ стартовать.
4. **Удаление автодетекта чат-шаблона по подстроке `qwen`** (`parse.go:159`).
   → Поведение остаётся дефолтом; путь выносится в конфиг как `chatTemplateFallback`.
5. **Ротация лога переименованием файла в родителе.** `llama-server` уже держит открытым
   old inode (`run.go:193-194`, `O_APPEND`) и продолжит писать в него.
   → Только через `io.Pipe` + горутину-писатель в родителе, либо внешний `logrotate`
   с `copytruncate`.

Дополнительные фактические поправки к исходным предложениям:

- **Диагноз про второй Ctrl-C неверный.** `signal.Notify` отключает дефолтную обработку на
  всё время регистрации. После выхода горутины (`run.go:277-281`) второй сигнал уходит в
  буфер `sigCh` (cap=1) и теряется. Реальный симптом — **Ctrl-C перестаёт работать вовсе**,
  и если `claude` завис, пользователь застревает. Фикс (цикл + эскалация) нужен, но по этой
  причине, а не из-за «мгновенной смерти родителя».
- **Порядок `assistant.tool_calls` vs `role=tool`** (`proxy.go:265` vs `273`) воспроизводится
  только при смешанных `tool_use` + `tool_result` в одном Anthropic-сообщении, чего по
  спецификации не бывает. Настоящая поломка tool use — это `tool_result` с массивом
  `content` (см. M1). Двухпроходный конвертер делаем ради устойчивости, а не как фикс №1.
- **`http.TimeoutReader` в Go не существует.** Правильный набор: `Client.Timeout = 0` +
  `Transport{ResponseHeaderTimeout: 30s}` + отмена через `r.Context()`.
- **`return 2` среди cleanup-веток ровно один** (`run.go:371`, невалидный `-mcp-addr`),
  а не «где-то 1, где-то 2». Дедупликация всё равно нужна.
- **`is_error` в `tool_result` нигде не обрабатывается** — при ошибке инструмента флаг
  теряется и модель читает текст ошибки как успешный результат.

---

## Веха 0 — Гигиена и страховка

Ноль риска, но повышает уверенность во всём остальном.

- [x] `gofmt -w .` — все 14 файлов одним коммитом, **без единого изменения логики**
      (`git diff --stat` должен показать только отступы).
      *Проверено: `git diff -w` показал только переносы строк, сортировку импортов и
      пустые строки у комментариев; тесты зелёные.*
- [x] `gofmt` в CI: шаг с `gofmt -l .` и непустым сообщением в `.github/workflows/ci.yml`.
      *Первый прогон — после push.*
- [x] Переименовать модуль: `module server` → `module github.com/SkalaSkalolaz/llama-runner`
      (`go.mod:1`). Внутренних импортов нет (один пакет `main`), правка на одну строку.
      *Проверено: `go test` показывает новое имя модуля.*
- [x] Исправить текст справки: `server [флаги]` → `llama-runner [флаги]` (`run.go:499`),
      а также 4 примера вызова в конце `printUsage` (`run.go:570-581`).
- [x] Версия через `-ldflags "-X main.version=$(git describe --tags --always --dirty)"` в CI;
      **дефолт в `main.go:15` сохраняем**, чтобы локальные сборки без ldflags работали.
- [x] `scripts/smoke.sh` — проган, 11/11 автоматических проверок пройдены
      (формат, vet, `test -race`, сборка с ldflags, `-version`, коды возврата
      `-h`/неизвестного флага/`-openai` без `-key`/некорректного `-reasoning`/
      отсутствующей модели, имя бинаря в справке). Реальные запуски — за `--launch`,
      ручные проверки — чек-листом в конце вывода.
- [x] Каркас golden-тестов: `golden_test.go` + `testdata/anthropic/` (обход
      `*_request.json`, сравнение с `*_openai.json`, флаг `-update`, `t.Skip()` при
      отсутствии golden). Первый fixture `plain_text` есть, остальные 9 кейсов — в M1
      (таблица в `testdata/anthropic/README.md`).

**Критерий готовности**

- [x] `gofmt -l .` пустой, CI это проверяет.
- [x] `go vet ./...` и `go test -race ./...` зелёные.
- [x] Бинарь собирается, `-version` печатает версию, поведение не изменилось.
      *Проверено smoke: `-version` → `v1.5.2-4-g07533c4-dirty`, коды возврата всех
      путей ошибок прежние (2 — валидация, 1 — отсутствующая модель, 0 — `-h`).*

**Веха 0 закрыта 2026-10-24.**

---

## Веха 1 — Конвертер запроса Anthropic → OpenAI

Самый ценный фикс: сейчас на локальной модели молча теряются данные.
Порядок внутри вехи обязателен: **сначала падающие тесты на реальных payload'ах,
потом переписывание.**

- [x] Собрать 5–10 реальных Anthropic payload'ов из логов Claude Code в `testdata/`:
      чистый текст, tool_use, `tool_result` со **строкой**, `tool_result` с **массивом
      блоков**, `system` массивом блоков (с `cache_control`), `thinking`, `image`,
      смешанный. Зафиксировать ожидаемый OpenAI payload (golden).
      *10 кейсов лежат в `testdata/anthropic/`. Оговорка: собраны вручную по
      документированной форме запросов Claude Code, живым дампом не снимались
      (в окружении разработки не было прогона Claude Code через прокси) — см.
      «Происхождение payload'ов» в `testdata/anthropic/README.md`. При появлении
      дампа класть рядом и перечитывать diff, не перезаписывая.*
- [x] **`tool_result` с массивом `content`** (`proxy.go:261-270`): обрабатывать
      `string` и `[]any` (склейка `text`-блоков; вложенные `image` — см. ниже).
      Сейчас блок отбрасывается целиком → модель не видит результат инструмента.
      *`convert.go:toolResultMessage`; вложенные `image` → `image_url` при `-mmproj`.
      Тесты: `TestConvertRequest_ToolResultBlocks`, `TestProxy_EndToEnd_ToolResultReachesUpstream`.*
- [x] **`is_error`** в `tool_result`: при `true` префикс `Error: ` в `content`
      OpenAI `role=tool` сообщения.
      *Тесты: `TestConvertRequest_ToolResultIsError`, `TestConvertRequest_ToolResultErrorBlocks`.*
- [x] **`system` как массив блоков** (`proxy.go:220`): склеить все `text`-блоки в один
      `role=system`. Сейчас теряется весь системный промпт.
      *`convert.go:systemText`, склейка через `\n`, `cache_control` отбрасывается.
      Тест: `TestConvertRequest_SystemBlocks` + golden `system_blocks`.*
- [x] **`thinking` / `image`**: не молчать. Либо маппить (`image` → OpenAI
      `image_url` data-URL, если задан `-mmproj`), либо явный warning в `-v` + счётчик
      отброшенных блоков. Решение зафиксировать в комментарии у функции.
      *Решение: `image` маппится на data-URL при `-mmproj` (`ConvertOptions.ImagesToOpenAI`),
      иначе отбрасывается; `thinking`/`redacted_thinking` отбрасываются всегда (аналога
      в chat/completions нет, подпись невоспроизводима). Всё считается в `ConvertStats`,
      при `-v` прокси печатает `[proxy] отброшено блоков: …`. Решение расписано в
      шапке `convert.go`.*
- [x] Двухпроходная сборка `messages` (сначала `assistant.tool_calls`, затем `tool`-ответы)
      — ради устойчивости к невалидным входам. Нормальный поток должен дать **байт-в-байт
      тот же** результат, что сейчас (проверяется golden-тестами).
      *`convert.go:convertBlocks`. Проверено: `testdata/anthropic/plain_text_openai.json`
      после переписывания не изменился (`git diff` пустой), порядок нормального потока —
      `TestConvertRequest_NormalFlowOrder`, смешанный вход — `MixedMessageAssistantFirst`
      и `UserTextWithToolResultKeepsToolFirst`.*
- [x] Ошибки прокси в формате Anthropic: `{"type":"error","error":{"type":...,"message":...}}`
      вместо `http.Error` (`proxy.go:85, 92, 115, 127, 134, 150, 163, 170`).
      Только для error-путей до начала стрима.
      *`writeAnthropicError` + `anthropicErrorType` (статус → тип Anthropic).
      Заменены все 8 перечисленных путей плюс 404 в `handleOther`, ошибка декодирования
      и «streaming unsupported». Тесты: `TestWriteAnthropicError_Format`,
      `TestHandleMessages_InvalidJSONIsAnthropicError`, `TestProxy_UpstreamErrorIsAnthropicFormat`.*
- [x] Вынести конвертацию в `convert.go`; при переходе на типизированные структуры:
      `arguments` у `tool_calls` **обязательно остаётся строкой с JSON** (сейчас это
      обеспечивает `mustJSON`, `proxy.go:569`), `DisallowUnknownFields` **не** включать,
      все сейчас копируемые поля (`max_tokens`, `temperature`, `top_p`,
      `stop_sequences`→`stop`, `tools`) покрыть тестами.
      *Конвертация переехала в `convert.go` (`AnthropicToOpenAI` сохранён как обёртка —
      им пользуется golden-харнес). На типизированные структуры НЕ переходили: остались
      на `map[string]any`, так что риск потерять «сквозные» поля отсутствует. Тесты:
      `TestConvertRequest_ArgumentsStaysJSONString`, `TestConvertRequest_Parameters`,
      `TestConvertRequest_Tools`, `TestConvertRequest_UnknownFieldsDoNotBreak`.*

**Критерий готовности**

- [x] Все golden-тесты зелёные; ни один ранее проходивший кейс не изменился.
      *11 golden'ов (10 кейсов + `image_mmproj`), `plain_text` побайтно прежний.*
- [x] `tool_result`-массив и `system`-массив покрыты тестами.
- [x] Ручной прогон: tool use на локальной модели доходит до результата инструмента
      (проверено 2026-10-25 на живой модели: tool_result-массив, `Error: `, system-блок).
---

## Веха 2 — Стриминг OpenAI → Anthropic

- [x] Тесты на `convertOpenAIStreamToAnthropic` через `httptest`:
      text-only, text + tool_calls + text после текста, несколько tool_calls с разными
      `index`, `finish_reason=length`, обрыв потока **до** `[DONE]`.
      *6 тестов в `stream_test.go`.*
- [x] **`scanner.Err()` проверяется**: обрыв апстрима → `event: error`, без `message_stop`.
      *`convert.go:convertOpenAIStreamToAnthropic`, ветка «Обработка обрыва потока».
      Тесты: `TestConvertStream_Interrupted`, `TestConvertStream_CleanEOFWithoutDone`.*
- [x] `usage`: запрашивается `stream_options: {"include_usage": true}`, реальные
      `input_tokens`/`output_tokens` в `message_delta`; в `message_start` — 0
      (у OpenAI usage приходит только в конце, достоверно неоткуда взять).
      *`proxy.go:forwardToLlama/forwardToOpenAI`, `convert.go`. Тесты:
      `TestConvertStream_UsageInMessageDelta`, `TestConvertStream_TextOnly`.*
- [x] Функция перенесена в `convert.go`; `proxy.go` больше не импортирует `bufio`.

**Критерий готовности**

- [x] Стриминг покрыт тестами (6 кейсов).
- [x] Обрыв апстрима даёт `error`-событие, не фиктивный `message_stop`.
- [x] В `-v` видно ненулевой usage на реальном запросе
      (проверено 2026-10-25: `grep '← usage'` даёт ненулевые значения;
      обрыв `pkill -9` даёт `event: error`, нормальный стоп не задет).
---

## Веха 3 — Ключ, лимиты тела, таймауты

Аддитивно, backward-compatible.

- [x] Ключ из окружения / файла: `LLAMA_RUNNER_OPENAI_KEY` (или `OPENAI_API_KEY`),
      опционально `~/.config/llama-runner/credentials` с правами `0600`.
      Приоритет: **CLI > env > файл > пусто**. Флаг `-key` продолжает работать.
      Ключ не должен попадать ни в stdout, ни в stderr, ни в лог — проверить `-v`-выветку
      (`run.go:332` печатает `OpenAIBaseURL`; ключ там появиться не должен).
- [x] `http.MaxBytesReader` на чтение тела: `proxy.go:83` и `proxy.go:185`.
      Лимит 100 МБ (реальные payload'ы < 10 МБ).
- [x] Убрать тотальный `Client.Timeout: 5*time.Minute` (`proxy.go:29`) — он режет длинную
      генерацию посреди стрима. Вместо него:
      `Client.Timeout = 0` + `Transport{ResponseHeaderTimeout: 30s}` + отмена по
      `r.Context()` (уже передаётся в `NewRequestWithContext`, `proxy.go:122, 157`).
      Никакого «`http.TimeoutReader`» — такого API в Go нет.

**Критерий готовности**

- [x] Запуск без `-key`, но с `LLAMA_RUNNER_OPENAI_KEY`, работает; с `-key` — работает как раньше.
- [x] Длинная (>$5$ мин) генерация не обрывается; зависший апстрим отсекается по
      `ResponseHeaderTimeout`.
- [x] `go test -race ./...` зелёные.

---

## Веха 4 — Аутентификация прокси

Делать **только после** M3. Правки в прокси и в `buildClaudeEnv` — одним коммитом,
иначе Claude Code перестанет ходить на прокси.

- [ ] Токен на входящих запросах: принимать **и** `Authorization: Bearer <tok>`,
      **и** `x-api-key: <tok>` (Claude Code шлёт по-разному в разных версиях).
- [ ] Тот же токен передавать в `buildClaudeEnv` (`parse.go:229-236`) —
      `ANTHROPIC_AUTH_TOKEN`. Дефолт оставляем `local-llama`, чтобы поведение не менялось;
      сгенерированный токен — только при включённой проверке.
- [ ] Не-loopback `-proxy-addr`: **предупреждение** + требование явного
      `-proxy-allow-remote` (или заданного токена). Не hard-fail.
- [ ] 401 в формате Anthropic-ошибки.

**Критерий готовности**

- [ ] Обычный запуск (без новых флагов) работает точно как сейчас.
- [ ] С включённой проверкой запрос без токена получает 401, с токеном — 200.
- [ ] `-proxy-addr 0.0.0.0:8082` по-прежнему стартует, но с предупреждением.

---

## Веха 5 — Сигналы, остановка, cleanup

Самая рискованная веха. См. красную зону, пункты 1 и 2.

- [ ] Горутину сигнала (`run.go:277-281`) сделать циклом `for sig := range sigCh`
      **без** `signal.Stop` внутри. `signal.Stop(sigCh)` — только перед `return` из `run()`
      (через `defer`). Иначе второй Ctrl-C уйдёт в дефолтную обработку.
- [ ] Эскалация: SIGINT → подождать → SIGTERM → 10–15 с → SIGKILL. Порог не занижать,
      чтобы не рвать сохранение файлов.
- [ ] `Setpgid: true` + `kill(-pgid)` — **только** для `llama-server` и `mcp-search-server`.
      Для `claude` оставить `Process.Signal` прямому потомку (красная зона №1).
- [ ] Дедупликация cleanup-блоков (8 копий: `run.go:315-318`, `338-341`, `354-357`,
      `368-371`, `405-408`, `429-432`, `436-439`, `457-460`) в один `defer`-омываемый
      shutdown-хелпер. **Порядок остановки и ожидания: `claude` → `llama-server` →
      `mcp` → `proxy`.** Сохранить единственный `return 2` (`run.go:371`).
- [ ] Разбить `run()` (460 строк) на `resolvePaths` / `startServer` / `startProxy` /
      `startClaude` — без изменения последовательности вызовов.
- [ ] Объединить `waitForServer` (`main.go:86-107`) и `waitForServerProcess`
      (`main.go:42-83`) в одну context-aware функцию; сохранить семантику «прекратить
      ожидание при смерти процесса» и убрать `time.Sleep(1s)` после последней попытки.

**Критерий готовности**

- [ ] Двойной и тройной Ctrl-C: дочерние процессы остановлены, сирот нет
      (`pgrep -f llama-server` пуст), интерактив `claude` не виснет.
- [ ] Висящий `claude` убивается по эскалации за ~15 с.
- [ ] Коды возврата всех путей не изменились (в т.ч. `return 2` на `-mcp-addr`).
- [ ] `go test -race ./...` зелёные.

---

## Веха 6 — Конфиг-файл, профили, doctor

Строго аддитивно: при отсутствии конфиг-файла поведение побайтно прежнее.

- [ ] `-config` (по умолчанию `~/.config/llama-runner/config.json`) со всеми текущими
      опциями. Приоритет: **CLI > env > файл > дефолты**.
- [ ] Именованные профили: `llama-runner -p coder`, `-p fast`.
- [ ] Вынести зашитые личные пути: `~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf` (`parse.go:26`)
      и `~/GGUF/qwen-fixed-chat-template.jinja` (`parse.go:160`) — в конфиг как
      `modelDefault` и `chatTemplateFallback`. **Автодетект по подстроке `qwen` остаётся
      дефолтом** (красная зона №4).
- [ ] `llama-runner doctor`: разрешённые пути, версия, что будет запущено, — без запуска.
- [ ] Обновить `README.md` / `README_RU.md` и `printUsage`.

**Критерий готовности**

- [ ] Все существующие команды из README-примеров работают без изменений.
- [ ] Конфиг переопределяется флагом; это покрыто тестом.
- [ ] `doctor` не запускает дочерние процессы.

---

## Веха 7 — Ротация логов

Отдельной задачей, не в составе M5.

- [ ] Ротация через `io.Pipe` + горутину-писателя в родителе: `cmd.Stdout/Stderr`
      (`run.go:193-194`, `run.go:220-221`) указывают на pipe, писатель сам переставляет
      файл по достижении `-log-max-mb` (дефолт, например, 50 МБ, N архивов).
      Переименование файла в родителе **запрещено** (красная зона №5).
- [ ] Альтернатива для тех, кто предпочитает внешнее решение: документированный
      `logrotate`-конфиг с `copytruncate`.
- [ ] Права `0600` сохраняются на новых файлах.

**Критерий готовности**

- [ ] Логи `llama-server` реально попадают в новый файл после ротации (проверено на живом
      процессе, не только в тесте).
- [ ] Нет потери строк на границе ротации; `go test -race` зелёные.

---

## Финальная проверка перед релизом

- [ ] `gofmt -l .` пустой
- [ ] `go vet ./...` чисто
- [ ] `go test -race -coverprofile=coverage.out ./...` зелёные; покрытие стриминга > 0
- [ ] Ручной smoke (`scripts/smoke.sh`): `-server`, обычный запуск, гибрид с `-openai`,
      `-webui-mcp-proxy`, `-update --help`-путь, двойной Ctrl-C
- [ ] Tool use на локальной модели работает end-to-end
- [ ] README.md / README_RU.md актуальны, бейджи («tests», «race-clean») соответствуют
      реальному положению дел
- [ ] Версия поднята, `CHANGELOG`/сообщение релиза описывает фиксы конвертера
- [ ] STATE.md обновлён: все пройденные вехи помечены

---

## Приложение: команды

```bash
gofmt -l .                                    # список «грязных» файлов
gofmt -w .                                    # форматирование
go vet ./...
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep -E 'proxy|total'
go build -ldflags "-X main.version=$(git describe --tags)" -o llama-runner .
```
