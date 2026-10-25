package main

import (
	"encoding/json"
	"fmt"
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
