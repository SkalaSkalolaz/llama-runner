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

## Обязательные кейсы

| Имя файла | Что проверяет | Статус |
|---|---|---|
| `plain_text` | строковый `content`, `system` строкой, `max_tokens`, `temperature` | есть (M0) |
| `system_blocks` | `system` массивом блоков с `cache_control` → один `role=system` | есть (M1) |
| `tool_use` | `assistant` с `text` + `tool_use` → `tool_calls` (`arguments` — строка JSON) | есть (M1) |
| `tool_result_string` | `tool_result` со строковым `content` | есть (M1) |
| `tool_result_blocks` | `tool_result` с массивом блоков (склейка `text`-блоков) | есть (M1) |
| `tool_result_error` | `is_error: true` → префикс `Error: ` | есть (M1) |
| `thinking` | `thinking`-блоки отбрасываются, но попадают в `ConvertStats` | есть (M1) |
| `image` | `image`-блоки: без `-mmproj` отбрасываются, с `-mmproj` → `image_url` | есть (M1) |
| `tools_defs` | `tools` → `function`-схемы, `stop_sequences` → `stop` | есть (M1) |
| `long_history` | длинная история, смешанные роли, два `tool_result` подряд | есть (M1) |

`image` даёт два golden'а: `image_openai.json` (дефолт, картинка отброшена) и
`image_mmproj_openai.json` (`ConvertOptions{ImagesToOpenAI: true}` — data-URL).
Второй golden харнес создаёт сам для любого payload'а, в котором есть image-блок.

### Происхождение payload'ов (важно)

Все кейсы M1 собраны **вручную** по документированной форме запросов Claude Code
(`system` массивом блоков с `cache_control`, `tools` с `input_schema`,
`tool_use`/`tool_result`, `thinking` с `signature`), а не сняты живым дампом:
в окружении разработки не было прогона Claude Code через прокси с записью тел.

Форма и последовательность ролей от этого не страдают (golden-тесты ровно их и
проверяют), но живые payload'ы могут содержать блоки и поля, которых здесь нет.
Когда появится возможность снять дамп (см. «Как получить реальные payload'ы») —
класть их рядом под теми же именами и перечитывать diff golden'ов, а не
переписывать существующие.

## Важно

Golden из вехи M0 фиксирует **текущее** поведение. Если оно содержит баг
(например, потеря `tool_result` с массивом `content`), golden зафиксирует баг —
это ожидаемо и полезно: в M1 такой golden обновляется осознанно, и в diff коммита
видно, что именно изменилось в поведении прокси.
