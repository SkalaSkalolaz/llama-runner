package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ============================================================
// Конвертация Anthropic → OpenAI (запрос)
// ============================================================
//
// Перенесено из proxy.go в вехе M1 (PLAN.md). Работает с map[string]any и
// намеренно НЕ использует json.Decoder с DisallowUnknownFields: незнакомые поля
// запроса должны проходить дальше, а не отбрасывать сообщение целиком.
//
// Что происходит с блоками content:
//
//	text             → склейка в строку content (как в M0)
//	tool_use         → assistant.tool_calls[]; arguments — ВСЕГДА строка с JSON
//	                   (mustJSON), иначе OpenAI-сервер не примет поле
//	tool_result      → role=tool; content строкой ИЛИ массивом блоков (text-блоки
//	                   склеиваются через "\n"); is_error:true → префикс "Error: ".
//	                   В M0 массивный content отбрасывался целиком — на локальной
//	                   модели tool use ломался (STATE.md, дефекты №1 и №3).
//	system           → строка или массив блоков → одно role=system (дефект №2)
//	image            → при -mmproj (ConvertOptions.ImagesToOpenAI) → OpenAI
//	                   image_url data-URL; иначе блок отбрасывается, но НЕ молча:
//	                   он учтён в ConvertStats.DroppedImages и виден при -v
//	thinking,
//	redacted_thinking→ отбрасываются: у OpenAI chat/completions нет поля для chain
//	                   of thought, а подпись thinking-блока невоспроизводима.
//	                   Учитываются в ConvertStats.DroppedThinking, видны при -v
//	неизвестный тип  → ConvertStats.DroppedUnknown
//
// Порядок сообщений (двухпроходная сборка, см. convertBlocks): сначала
// assistant.tool_calls, затем role=tool. Нормальный поток (assistant с tool_use,
// следующий user с tool_result) даёт ровно тот же результат, что и M0 — это
// зафиксировано golden-тестами и TestConvertRequest_NormalFlowOrder.

// ConvertOptions — настройки конвертации запроса.
type ConvertOptions struct {
	// ImagesToOpenAI — true, когда llama-server запущен с vision-проектором
	// (-mmproj): image-блоки имеют смысл и маппятся на OpenAI image_url.
	// Иначе их всё равно нечем обработать, и блок отбрасывается (со счётчиком).
	ImagesToOpenAI bool
}

// ConvertStats — сколько блоков не дошло до модели. Нужен для того, чтобы
// отбрасывание перестало быть молчаливым (см. -v в proxyHandler.convert).
type ConvertStats struct {
	DroppedThinking int
	DroppedImages   int
	DroppedUnknown  int
}

// Dropped — всего отброшенных блоков.
func (s ConvertStats) Dropped() int {
	return s.DroppedThinking + s.DroppedImages + s.DroppedUnknown
}

// Summary — одна строка для -v; пустая, если отброшено ничего.
func (s ConvertStats) Summary() string {
	var parts []string
	if s.DroppedThinking > 0 {
		parts = append(parts, fmt.Sprintf("%d thinking", s.DroppedThinking))
	}
	if s.DroppedImages > 0 {
		parts = append(parts, fmt.Sprintf("%d image", s.DroppedImages))
	}
	if s.DroppedUnknown > 0 {
		parts = append(parts, fmt.Sprintf("%d неизвестных", s.DroppedUnknown))
	}
	return strings.Join(parts, ", ")
}

// AnthropicToOpenAI — конвертация с дефолтными опциями (image-блоки не маппятся).
// Сохранена сигнатура из M0: ею пользуются golden-тесты.
func AnthropicToOpenAI(anthropic map[string]any) (map[string]any, error) {
	out, _, err := ConvertRequest(anthropic, ConvertOptions{})
	return out, err
}

// ConvertRequest конвертирует тело запроса Anthropic Messages API в тело запроса
// OpenAI chat/completions. Возвращает статистику отброшенных блоков.
func ConvertRequest(anthropic map[string]any, opts ConvertOptions) (map[string]any, ConvertStats, error) {
	var st ConvertStats

	openAI := map[string]any{
		"model": anthropic["model"],
	}

	// system → первое сообщение role=system.
	var messages []map[string]any
	if sys := systemText(anthropic["system"]); sys != "" {
		messages = append(messages, map[string]any{"role": "system", "content": sys})
	}

	// messages: content может быть строкой или массивом блоков.
	if inMsgs, ok := anthropic["messages"].([]any); ok {
		for _, m := range inMsgs {
			msg, ok := m.(map[string]any)
			if !ok {
				continue
			}
			role, _ := msg["role"].(string)
			messages = append(messages, convertMessage(role, msg["content"], opts, &st)...)
		}
	}
	openAI["messages"] = messages

	// Параметры
	if v, ok := anthropic["max_tokens"]; ok {
		openAI["max_tokens"] = v
	}
	if v, ok := anthropic["temperature"]; ok {
		openAI["temperature"] = v
	}
	if v, ok := anthropic["top_p"]; ok {
		openAI["top_p"] = v
	}
	if v, ok := anthropic["stop_sequences"]; ok {
		openAI["stop"] = v
	}

	// tools: Anthropic {name, description, input_schema} → OpenAI {type:function, function:{...}}
	if tools, ok := anthropic["tools"].([]any); ok && len(tools) > 0 {
		var openAITools []map[string]any
		for _, t := range tools {
			tool, ok := t.(map[string]any)
			if !ok {
				continue
			}
			openAITools = append(openAITools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        tool["name"],
					"description": tool["description"],
					"parameters":  tool["input_schema"],
				},
			})
		}
		openAI["tools"] = openAITools
	}

	return openAI, st, nil
}

// systemText собирает system в одну строку: строка как есть, массив блоков —
// склейка text-блоков через "\n" (cache_control отбрасывается: у OpenAI своего
// prompt caching в этом виде нет).
func systemText(sys any) string {
	switch v := sys.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, b := range v {
			block, ok := b.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := block["type"].(string); t != "text" {
				continue
			}
			if s, ok := block["text"].(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// convertMessage — одно сообщение Anthropic → ноль, одно или несколько сообщений OpenAI.
func convertMessage(role string, content any, opts ConvertOptions, st *ConvertStats) []map[string]any {
	switch v := content.(type) {
	case string:
		return []map[string]any{{"role": role, "content": v}}
	case []any:
		return convertBlocks(role, v, opts, st)
	}
	// content отсутствует или неизвестной формы — сообщение пропускается (как в M0).
	return nil
}

// convertBlocks — двухпроходная сборка: за один обход блоков собираем
// «главное» сообщение (text + image + tool_calls) и отдельно role=tool ответы,
// потом выдаём их в порядке, который понимает OpenAI:
//
//	assistant с tool_calls  →  сначала он, затем tool-ответы
//	user                    →  сначала tool-ответы (они должны идти сразу за
//	                           assistant.tool_calls), затем текст пользователя
//
// Для нормального потока (tool_use и tool_result в разных сообщениях) порядок
// совпадает с M0 байт-в-байт.
func convertBlocks(role string, blocks []any, opts ConvertOptions, st *ConvertStats) []map[string]any {
	var (
		textParts    []string
		parts        []any // content массивом — нужен только если в сообщении есть image
		toolCalls    []map[string]any
		toolMsgs     []map[string]any
		mappedImages int
	)

	for _, b := range blocks {
		block, ok := b.(map[string]any)
		if !ok {
			continue
		}
		switch btype, _ := block["type"].(string); btype {
		case "text":
			if t, ok := block["text"].(string); ok {
				textParts = append(textParts, t)
				parts = append(parts, map[string]any{"type": "text", "text": t})
			}
		case "image":
			if url, ok := imageDataURL(block); ok && opts.ImagesToOpenAI {
				mappedImages++
				parts = append(parts, map[string]any{
					"type": "image_url", "image_url": map[string]any{"url": url},
				})
			} else {
				st.DroppedImages++
			}
		case "thinking", "redacted_thinking":
			st.DroppedThinking++
		case "tool_use":
			toolCalls = append(toolCalls, map[string]any{
				"id":   block["id"],
				"type": "function",
				"function": map[string]any{
					"name":      block["name"],
					"arguments": mustJSON(block["input"]),
				},
			})
		case "tool_result":
			if m, ok := toolResultMessage(block, opts, st); ok {
				toolMsgs = append(toolMsgs, m)
			}
		default:
			st.DroppedUnknown++
		}
	}

	var main map[string]any
	switch {
	case len(toolCalls) > 0:
		// У assistant-сообщения в OpenAI content мультимодальным не бывает,
		// поэтому image-блоки здесь некуда положить — они уходят в счётчик.
		st.DroppedImages += mappedImages
		mappedImages = 0
		main = map[string]any{
			"role":       "assistant",
			"content":    strings.Join(textParts, ""),
			"tool_calls": toolCalls,
		}
	case len(textParts) > 0 || mappedImages > 0:
		content := any(strings.Join(textParts, ""))
		if mappedImages > 0 {
			content = parts
		}
		main = map[string]any{"role": role, "content": content}
	}

	switch {
	case main == nil:
		return toolMsgs
	case len(toolCalls) > 0:
		return append([]map[string]any{main}, toolMsgs...)
	default:
		return append(toolMsgs, main)
	}
}

// toolResultMessage — tool_result → сообщение role=tool.
// content бывает строкой, массивом блоков или отсутствовать; is_error:true
// обязан дойти до модели, поэтому текст получает префикс "Error: ".
func toolResultMessage(block map[string]any, opts ConvertOptions, st *ConvertStats) (map[string]any, bool) {
	var (
		text       string
		imageParts []any
	)

	switch c := block["content"].(type) {
	case nil:
		// Пустой результат: сообщение role=tool всё равно отдаём — вызов обязан
		// быть закрыт, иначе OpenAI-сервер отвергнет весь запрос.
	case string:
		text = c
	case []any:
		var texts []string
		for _, b := range c {
			inner, ok := b.(map[string]any)
			if !ok {
				continue
			}
			switch t, _ := inner["type"].(string); t {
			case "text":
				if s, ok := inner["text"].(string); ok {
					texts = append(texts, s)
				}
			case "image":
				if url, ok := imageDataURL(inner); ok && opts.ImagesToOpenAI {
					imageParts = append(imageParts, map[string]any{
						"type": "image_url", "image_url": map[string]any{"url": url},
					})
				} else {
					st.DroppedImages++
				}
			default:
				st.DroppedUnknown++
			}
		}
		text = strings.Join(texts, "\n")
	default:
		// Число, bool, объект — отдаём как JSON, терять данные незачем.
		text = mustJSON(block["content"])
	}

	if isErr, _ := block["is_error"].(bool); isErr {
		text = "Error: " + text
	}

	msg := map[string]any{
		"role":         "tool",
		"tool_call_id": block["tool_use_id"],
	}
	if len(imageParts) > 0 {
		content := make([]any, 0, len(imageParts)+1)
		if text != "" {
			content = append(content, map[string]any{"type": "text", "text": text})
		}
		content = append(content, imageParts...)
		msg["content"] = content
	} else {
		msg["content"] = text
	}
	return msg, true
}

// imageDataURL — Anthropic image-блок → OpenAI image_url.
// base64 → data-URL, url → как есть.
func imageDataURL(block map[string]any) (string, bool) {
	src, ok := block["source"].(map[string]any)
	if !ok {
		return "", false
	}
	switch kind, _ := src["type"].(string); kind {
	case "base64":
		data, _ := src["data"].(string)
		if data == "" {
			return "", false
		}
		mime, _ := src["media_type"].(string)
		if mime == "" {
			mime = "application/octet-stream"
		}
		return "data:" + mime + ";base64," + data, true
	case "url":
		u, _ := src["url"].(string)
		return u, u != ""
	}
	return "", false
}

// requestSummary — краткое описание сконвертированного запроса для -v.
// По нему видно, что системный промпт и результаты инструментов действительно
// дошли до апстрима, не читая тело запроса целиком.
func requestSummary(openAIReq map[string]any) string {
	msgs, _ := openAIReq["messages"].([]map[string]any)
	var (
		hasSystem bool
		toolMsgs  int
		toolCalls int
	)
	for _, m := range msgs {
		switch m["role"] {
		case "system":
			hasSystem = true
		case "tool":
			toolMsgs++
		case "assistant":
			if _, ok := m["tool_calls"]; ok {
				toolCalls++
			}
		}
	}
	tools, _ := openAIReq["tools"].([]map[string]any)
	return fmt.Sprintf("%d сообщений: system=%v, assistant с tool_calls=%d, tool-ответов=%d, схем инструментов=%d",
		len(msgs), hasSystem, toolCalls, toolMsgs, len(tools))
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// toolMessagesPreview — по одной строке на каждое сообщение role=tool с
// обрезанным превью content. Нужен для -v, чтобы в логе было видно, что
// именно вернул инструмент, включая префикс "Error: " у is_error:true.
func toolMessagesPreview(openAIReq map[string]any) []string {
	msgs, _ := openAIReq["messages"].([]map[string]any)
	var out []string
	for i, m := range msgs {
		if m["role"] != "tool" {
			continue
		}
		out = append(out, fmt.Sprintf("tool[%d] tool_call_id=%v content=%q",
			i, m["tool_call_id"], previewContent(m["content"], 80)))
	}
	return out
}

// previewContent — превью content для лога. У role=tool content обычно
// строка, но может быть []any (с image_url при -mmproj) — тогда показываем
// пометку, чтобы не вываливать base64 в лог.
func previewContent(v any, maxLen int) string {
	switch c := v.(type) {
	case string:
		return truncate(c, maxLen)
	case []any:
		return "[multimodal content]"
	default:
		return truncate(mustJSON(v), maxLen)
	}
}

// truncate — обрезает строку до maxLen рун, экранируя переносы строк, чтобы
// лог оставался однострочным. Счёт по рунам, не по байтам, чтобы не рвать UTF-8.
func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}

// ============================================================
// Конвертация стриминга OpenAI → Anthropic (SSE)
// ============================================================
//
// Стриминг устроен так: апстрим присылает чанки `data: {...}\n\n`
// (по одному на дельту контента/tool_calls) и завершает поток
// `data: [DONE]\n\n`. Мы конвертируем это в SSE-последовательность
// Anthropic:
//
//	message_start
//	  content_block_start (text) / content_block_delta (text) / content_block_stop
//	  content_block_start (tool_use) / content_block_delta (input_json_delta) / content_block_stop
//	message_delta (stop_reason, usage)
//	message_stop
//
// При обрыве потока до [DONE] (ошибка чтения апстрима или чистый EOF без
// finish_reason) вместо message_stop отдаём `event: error` — см. дефект №6.
// Usage запрашивается через stream_options.include_usage и проставляется
// в message_delta (дефект №7). В message_start usage = 0: у OpenAI usage
// приходит только в конце, а message_start уходит первым — достоверно
// input_tokens там взять неоткуда, выдумывать не будем.
//
// Возвращает input_tokens/output_tokens (нужно для лога при -v).
// Если апстрим usage не прислал — оба 0.
func convertOpenAIStreamToAnthropic(w http.ResponseWriter, openAIResp *http.Response, modelName string) (int, int) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAnthropicError(w, http.StatusInternalServerError, "api_error", "streaming unsupported")
		return 0, 0
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	writeSSE := func(event string, data any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		flusher.Flush()
	}

	writeSSE("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": "msg_proxy", "type": "message", "role": "assistant",
			"model": modelName, "content": []any{},
			"stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
		},
	})

	// Индекс текущего content-блока (text или tool_use)
	blockIndex := 0
	textBlockOpen := false

	// Состояние tool_calls: OpenAI присылает их кусками (index → {id, name, args})
	type toolState struct {
		id        string
		name      string
		argsAccum string
		blockIdx  int
	}
	tools := map[int]*toolState{}
	toolOrder := []int{}

	stopReason := "end_turn"
	var inputTokens, outputTokens int
	gotDone := false
	gotFinishReason := false

	scanner := bufio.NewScanner(openAIResp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			gotDone = true
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			// Финальный чанк с stream_options.include_usage:
			// {"choices":[],"usage":{"prompt_tokens":N,"completion_tokens":M}}
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}

		// Usage приходит отдельным чанком, обычно с пустым choices —
		// обрабатываем ДО проверки len(choices)==0.
		if chunk.Usage != nil {
			inputTokens = chunk.Usage.PromptTokens
			outputTokens = chunk.Usage.CompletionTokens
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if choice.FinishReason != "" {
			gotFinishReason = true
		}

		// --- Текстовые дельты ---
		if choice.Delta.Content != "" {
			if !textBlockOpen {
				writeSSE("content_block_start", map[string]any{
					"type": "content_block_start", "index": blockIndex,
					"content_block": map[string]any{"type": "text", "text": ""},
				})
				textBlockOpen = true
			}
			writeSSE("content_block_delta", map[string]any{
				"type": "content_block_delta", "index": blockIndex,
				"delta": map[string]any{"type": "text_delta", "text": choice.Delta.Content},
			})
		}

		// --- tool_calls дельты ---
		for _, tc := range choice.Delta.ToolCalls {
			st, exists := tools[tc.Index]
			if !exists {
				// Закрываем text-блок, если открыт
				if textBlockOpen {
					writeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": blockIndex})
					blockIndex++
					textBlockOpen = false
				}
				st = &toolState{id: tc.ID, name: tc.Function.Name, blockIdx: blockIndex}
				tools[tc.Index] = st
				toolOrder = append(toolOrder, tc.Index)
				blockIndex++

				writeSSE("content_block_start", map[string]any{
					"type": "content_block_start", "index": st.blockIdx,
					"content_block": map[string]any{
						"type": "tool_use", "id": st.id, "name": st.name, "input": map[string]any{},
					},
				})
			}
			if tc.Function.Arguments != "" {
				st.argsAccum += tc.Function.Arguments
				writeSSE("content_block_delta", map[string]any{
					"type": "content_block_delta", "index": st.blockIdx,
					"delta": map[string]any{
						"type": "input_json_delta", "partial_json": tc.Function.Arguments,
					},
				})
			}
		}

		switch choice.FinishReason {
		case "tool_calls":
			stopReason = "tool_use"
		case "length":
			stopReason = "max_tokens"
		}
	}

	// --- Обработка обрыва потока (дефект №6) ---
	//
	// scanner.Err() != nil  — TCP RST/таймаут/ошибка декодирования.
	// !gotDone && !gotFinishReason — чистый EOF, но ни [DONE], ни finish_reason:
	//   поток оборван на середине. Если finish_reason был, а [DONE] нет —
	//   считаем провайдер просто не шлёт [DONE] (некоторые так делают).
	streamErr := scanner.Err()
	if streamErr == nil && !gotDone && !gotFinishReason {
		streamErr = fmt.Errorf("upstream stream ended before [DONE]")
	}
	if streamErr != nil {
		// Закрываем открытые блоки, чтобы клиент не завис на незакрытом
		// content_block (некоторые клиенты так умеют).
		if textBlockOpen {
			writeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": blockIndex})
		}
		for _, idx := range toolOrder {
			writeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": tools[idx].blockIdx})
		}
		// event: error — Anthropic-формат ошибки в стриме. JSON-тело уже
		// нельзя: заголовки отправлены, message_start ушёл.
		writeSSE("error", map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    "api_error",
				"message": "upstream stream interrupted: " + streamErr.Error(),
			},
		})
		return inputTokens, outputTokens
	}

	// --- Нормальное завершение ---
	if textBlockOpen {
		writeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": blockIndex})
	}
	for _, idx := range toolOrder {
		writeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": tools[idx].blockIdx})
	}

	writeSSE("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
		// input_tokens в message_delta — «сверх» Anthropic-спеки, но безвредно:
		// message_start уходит раньше, чем мы узнаём usage, поэтому реальный
		// input_tokens доступен только здесь. Клиент читает output_tokens,
		// input_tokens — для полноты.
		"usage": map[string]any{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	})
	writeSSE("message_stop", map[string]any{"type": "message_stop"})

	return inputTokens, outputTokens
}
