# testdata/anthropic — golden-выборка конвертера запроса

Каркас регрессионного барьера для `AnthropicToOpenAI` (прокси, веха **M1** в PLAN.md).

## Формат

```
<имя>_request.json   вход: тело запроса Anthropic Messages API
<имя>_openai.json    ожидаемый выход: тело запроса OpenAI chat/completions
```

Golden'ы — это канонический `json.MarshalIndent` от результата конвертера. Их не
пишут руками:

```bash
go test -run TestGoldenAnthropicToOpenAI -update   # перегенерировать
go test -run TestGoldenAnthropicToOpenAI           # проверить
```

Если golden-файла нет, тест **пропускается** (не падает) — чтобы каркас из вехи M0
не ломал CI до наполнения выборки.

## Как получить реальные payload'ы

Нужны настоящие запросы Claude Code, а не выдуманные — иначе проверим не то.

1. Поднять прокси с дампом тел запросов (временный хак на время съёма):
   в `handleMessages` после `io.ReadAll` дописать запись `body` в файл.
2. Прогнать в Claude Code сценарии из таблицы ниже.
3. Сохранить каждое тело как `<имя>_request.json`, хак убрать, прогнать `-update`,
   **вручную прочитать diff** и только потом коммитить.

## Обязательные кейсы (заполняются в M1)

| Имя файла | Что проверяет | Статус |
|---|---|---|
| `plain_text` | строковый `content`, `system` строкой, `max_tokens`, `temperature` | есть (M0) |
| `system_blocks` | `system` массивом блоков с `cache_control` — сейчас теряется | ждать M1 |
| `tool_use` | `assistant` с `text` + `tool_use` → `tool_calls` (`arguments` — строка JSON) | ждать M1 |
| `tool_result_string` | `tool_result` со строковым `content` | ждать M1 |
| `tool_result_blocks` | `tool_result` с массивом блоков — **сейчас теряется целиком** | ждать M1 |
| `tool_result_error` | `is_error: true` — сейчас флаг игнорируется | ждать M1 |
| `thinking` | `thinking`-блоки — сейчас отбрасываются молча | ждать M1 |
| `image` | `image`-блоки при `-mmproj` — сейчас отбрасываются молча | ждать M1 |
| `tools_defs` | `tools` → `function`-схемы | ждать M1 |
| `long_history` | длинная история, смешанные роли | ждать M1 |

## Важно

Golden из вехи M0 фиксирует **текущее** поведение. Если оно содержит баг
(например, потеря `tool_result` с массивом `content`), golden зафиксирует баг —
это ожидаемо и полезно: в M1 такой golden обновляется осознанно, и в diff коммита
видно, что именно изменилось в поведении прокси.
