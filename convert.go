package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ============================================================
// Anthropic → OpenAI conversion (request)
// ============================================================
//
// Moved from proxy.go in milestone M1 (PLAN.md). Works with map[string]any and
// intentionally does NOT use json.Decoder with DisallowUnknownFields: unknown fields
// in the request must pass through, not drop the message entirely.
//
// What happens with content blocks:
//
//	text             → concatenated into content string (as in M0)
//	tool_use         → assistant.tool_calls[]; arguments — ALWAYS a string with JSON
//	                   (mustJSON), otherwise the OpenAI server won't accept the field
//	tool_result      → role=tool; content as a string OR block array (text blocks
//	                   concatenated via "\n"); is_error:true → "Error: " prefix.
//	                   In M0 array content was dropped entirely — on a local
//	                   model tool use broke (STATE.md, defects #1 and #3).
//	system           → string or block array → single role=system (defect #2)
//	image            → on -mmproj (ConvertOptions.ImagesToOpenAI) → OpenAI
//	                   image_url data-URL; otherwise the block is dropped but NOT silently:
//	                   it's counted in ConvertStats.DroppedImages and visible at -v
//	thinking,
//	redacted_thinking→ dropped: OpenAI chat/completions has no field for chain
//	                   of thought, and the thinking-block signature is not reproducible.
//	                   Counted in ConvertStats.DroppedThinking, visible at -v
//	unknown type     → ConvertStats.DroppedUnknown
//
// Message order (two-pass assembly, see convertBlocks): first
// assistant.tool_calls, then role=tool. Normal flow (assistant with tool_use,
// next user with tool_result) gives exactly the same result as M0 — this
// is fixed by golden tests and TestConvertRequest_NormalFlowOrder.

// ConvertOptions controls how an Anthropic request is mapped onto the OpenAI
// chat/completions API.
type ConvertOptions struct {
	// ImagesToOpenAI enables mapping Anthropic image blocks to OpenAI
	// image_url parts. Set it only when llama-server runs with a vision
	// projector (-mmproj): without a projector there is nothing that could
	// process the pixels, so image blocks are dropped instead — and counted,
	// never silently.
	ImagesToOpenAI bool
}

// ConvertStats counts content blocks that did not reach the model during
// conversion, one counter per drop reason. It exists so that lossy
// conversion is never silent: the proxy surfaces these numbers under -v (see
// proxyHandler.convert).
type ConvertStats struct {
	// DroppedThinking counts thinking/redacted_thinking blocks — OpenAI
	// chat/completions has no chain-of-thought field.
	DroppedThinking int
	// DroppedImages counts image blocks with no projector configured or no
	// usable source.
	DroppedImages int
	// DroppedUnknown counts blocks whose type is not recognised at all.
	DroppedUnknown int
}

// Dropped returns the total number of blocks dropped during conversion,
// summing all reasons.
func (s ConvertStats) Dropped() int {
	return s.DroppedThinking + s.DroppedImages + s.DroppedUnknown
}

// Summary renders a compact one-line description of what was dropped
// (e.g. "2 thinking, 1 image") for the -v log. It returns an empty string
// when nothing was dropped, so callers can test the result directly.
func (s ConvertStats) Summary() string {
	var parts []string
	if s.DroppedThinking > 0 {
		parts = append(parts, fmt.Sprintf("%d thinking", s.DroppedThinking))
	}
	if s.DroppedImages > 0 {
		parts = append(parts, fmt.Sprintf("%d image", s.DroppedImages))
	}
	if s.DroppedUnknown > 0 {
		parts = append(parts, fmt.Sprintf(T(msgSummaryUnknown), s.DroppedUnknown))
	}
	return strings.Join(parts, ", ")
}

// AnthropicToOpenAI converts an Anthropic request with default options, i.e.
// image blocks are not mapped (no vision projector assumed). Its signature is
// preserved from the original M0 implementation because the golden tests use
// it; new code should call ConvertRequest to get drop statistics.
func AnthropicToOpenAI(anthropic map[string]any) (map[string]any, error) {
	out, _, err := ConvertRequest(anthropic, ConvertOptions{})
	return out, err
}

// ConvertRequest converts an Anthropic Messages API request body into an
// OpenAI chat/completions request body, returning per-reason statistics of
// content blocks that could not be carried over.
//
// Mapping: the system prompt (string or block array) becomes the first
// role=system message; each conversation message goes through
// convertMessage, which may yield several OpenAI messages from one Anthropic
// one. Sampling parameters pass through where an equivalent exists
// (max_tokens, temperature, top_p; stop_sequences → stop), and Anthropic
// tools {name, description, input_schema} become OpenAI
// {type: function, function: {...}} entries.
//
// The input is deliberately handled as plain map[string]any without
// DisallowUnknownFields: fields the converter does not know about (and
// therefore does not copy) must never cause the request to be rejected.
// The returned error is reserved for unrecoverable problems; lossy parts of
// the conversion are reported through ConvertStats instead.
func ConvertRequest(anthropic map[string]any, opts ConvertOptions) (map[string]any, ConvertStats, error) {
	var st ConvertStats

	openAI := map[string]any{
		"model": anthropic["model"],
	}

	// system → first role=system message.
	var messages []map[string]any
	if sys := systemText(anthropic["system"]); sys != "" {
		messages = append(messages, map[string]any{"role": "system", "content": sys})
	}

	// messages: content can be a string or block array.
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

	// Parameters
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

// systemText flattens the Anthropic system field into a single string for the
// role=system message. A plain string is returned as-is; a block array is
// reduced to its text blocks joined with "\n" — cache_control markers are
// discarded because OpenAI's prompt caching works differently and the marker
// has no meaning there. Anything else yields "".
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

// convertMessage converts one Anthropic message into zero, one or more
// OpenAI messages: a plain string content maps 1:1, a content-block array is
// handed to convertBlocks, and a message with absent or unrecognised content
// is skipped entirely (matching M0 behaviour).
func convertMessage(role string, content any, opts ConvertOptions, st *ConvertStats) []map[string]any {
	switch v := content.(type) {
	case string:
		return []map[string]any{{"role": role, "content": v}}
	case []any:
		return convertBlocks(role, v, opts, st)
	}
	// content is absent or in an unknown form — skip the message (as in M0).
	return nil
}

// convertBlocks performs the two-pass assembly of one message's content
// blocks. The first pass sorts blocks into a "main" message payload (text,
// images, tool_calls) and separate role=tool answers; the second pass emits
// them in the order the OpenAI API requires:
//
//	assistant with tool_calls → the assistant message first, then its
//	                             tool answers;
//	user (carrying tool_result) → tool answers first — they must
//	                             immediately follow the assistant's
//	                             tool_calls — then the user's own text.
//
// For the normal Anthropic flow (tool_use and tool_result in separate
// messages) the resulting order is byte-for-byte identical to M0. Images
// inside an assistant message are dropped (counted), since OpenAI assistant
// messages cannot carry multimodal content.
func convertBlocks(role string, blocks []any, opts ConvertOptions, st *ConvertStats) []map[string]any {
	var (
		textParts    []string
		parts        []any // content as an array — only needed when the message has images
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
		// In OpenAI, assistant messages cannot have multimodal content,
		// so image blocks have nowhere to go here — they go to the counter.
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

// toolResultMessage converts an Anthropic tool_result block into the
// corresponding role=tool message, keyed by tool_use_id.
//
// The result content may be a string, an array of blocks (text blocks joined
// with "\n", image blocks mapped to image_url when a projector is configured
// and counted as dropped otherwise), a scalar/object (serialised to JSON so
// no data is lost) or absent. Even an empty result produces a message: every
// tool call must be closed, or the OpenAI server rejects the whole request.
// When is_error is true the text is prefixed with "Error: " so the failure
// actually reaches the model — OpenAI has no structured error flag here.
func toolResultMessage(block map[string]any, opts ConvertOptions, st *ConvertStats) (map[string]any, bool) {
	var (
		text       string
		imageParts []any
	)

	switch c := block["content"].(type) {
	case nil:
		// Empty result: we still emit the role=tool message — the call must be
		// be closed, otherwise the OpenAI server will reject the entire request.
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
		// Number, bool, object — emit as JSON, no reason to lose data.
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

// imageDataURL maps an Anthropic image block's source to an OpenAI image_url
// string: a base64 source becomes a data: URL (media_type defaults to
// application/octet-stream when missing), a url source is passed through
// unchanged. It reports false when the block has no usable source — wrong
// shape, empty base64 data or empty URL.
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

// requestSummary renders a brief human-readable digest of a converted OpenAI
// request for the -v log: total message count, whether a system message is
// present, how many assistant messages carry tool_calls, how many role=tool
// answers there are, and how many tool schemas were sent. It lets you verify
// at a glance that the system prompt and tool results actually reached the
// upstream, without dumping the full request body.
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
	return fmt.Sprintf(T(msgSummaryMessages),
		len(msgs), hasSystem, toolCalls, toolMsgs, len(tools))
}

// mustJSON serialises v to its compact JSON representation for the
// tool_calls arguments field, which OpenAI requires to be a JSON string.
// Marshalling errors are swallowed (the zero value "null" is produced);
// callers pass plain decoded JSON values, which always serialise.
func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// toolMessagesPreview produces one log line per role=tool message in a
// converted request, with the tool_call_id and a truncated content preview
// (see previewContent). It backs the -v log so you can see exactly what each
// tool returned — including the "Error: " prefix set for is_error results —
// without dumping full payloads.
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

// previewContent renders a message content value for the log. role=tool
// content is normally a string (truncated to maxLen), but with -mmproj it can
// be a multimodal block array — that is replaced by a fixed placeholder note
// rather than dumping base64 image data into the log. Anything else is
// serialised to JSON and truncated.
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

// truncate prepares a string for a single-line log entry: literal "\n" and
// "\r" sequences are escaped, and the result is cut to maxLen runes (counted
// by runes, never bytes, so multi-byte UTF-8 characters are never broken),
// with a trailing ellipsis when anything was cut off.
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
// OpenAI → Anthropic streaming conversion (SSE)
// ============================================================

// convertOpenAIStreamToAnthropic consumes an OpenAI streaming
// chat/completions response and re-emits it live as an Anthropic Messages SSE
// stream, flushing every event as it is produced.
//
// The upstream sends `data: {...}\n\n` chunks carrying content or tool_calls
// deltas and terminates with `data: [DONE]\n\n`. The emitted Anthropic
// sequence is:
//
//	message_start
//	  content_block_start/delta/stop for the text block (opened lazily on
//	  the first non-empty content delta)
//	  content_block_start/delta/stop per tool_use block (each tool call,
//	  accumulated from its fragmented deltas across chunks)
//	message_delta (mapped stop_reason and token usage)
//	message_stop
//
// Semantics worth knowing:
//   - usage is requested via stream_options.include_usage and arrives in a
//     final chunk, so it is reported in message_delta; message_start carries
//     zeros because real input_tokens cannot be known before anything was
//     streamed and fabricating them would lie (defect #7);
//   - a broken stream — scanner error, or a clean EOF with neither [DONE]
//     nor a finish_reason — ends with an `event: error` instead of
//     message_stop, after closing every open content block so no client
//     hangs (defect #6). A provider that sets finish_reason but never sends
//     [DONE] is treated as complete;
//   - the response writer must implement http.Flusher; if it does not, a 500
//     is written instead of streaming.
//
// Returns the input/output token counts taken from the upstream usage chunk
// (both 0 when the upstream sent none); the caller logs them under -v.
func convertOpenAIStreamToAnthropic(w http.ResponseWriter, openAIResp *http.Response, modelName string) (int, int) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAnthropicError(w, http.StatusInternalServerError, "api_error", T(msgStreamingUnsupp))
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

	// Current content-block index (text or tool_use)
	blockIndex := 0
	textBlockOpen := false

	// tool call state: OpenAI sends them in pieces (index → {id, name, args})
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
			// Final chunk with stream_options.include_usage:
			// {"choices":[],"usage":{"prompt_tokens":N,"completion_tokens":M}}
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}

		// Usage arrives as a separate chunk, usually with empty choices —
		// process before checking len(choices)==0.
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

		// --- Text deltas ---
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

		// --- tool_calls deltas ---
		for _, tc := range choice.Delta.ToolCalls {
			st, exists := tools[tc.Index]
			if !exists {
				// Close text block if open
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

	// --- Stream break handling (defect #6) ---
	//
	// scanner.Err() != nil  — TCP RST/timeout/decode error.
	// !gotDone && !gotFinishReason — clean EOF, neither [DONE] nor finish_reason:
	//   stream was cut mid-way. If finish_reason was set but [DONE] wasn't —
	//   assume the provider simply doesn't send [DONE] (some do this).
	streamErr := scanner.Err()
	if streamErr == nil && !gotDone && !gotFinishReason {
		streamErr = errors.New(T(msgStreamNoDone))
	}
	if streamErr != nil {
		// Close open blocks so the client doesn't hang on an unclosed
		// content_block (some clients behave that way).
		if textBlockOpen {
			writeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": blockIndex})
		}
		for _, idx := range toolOrder {
			writeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": tools[idx].blockIdx})
		}
		// event: error — Anthropic error format in stream. JSON body is already
		// impossible: headers are sent, message_start went out.
		writeSSE("error", map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    "api_error",
				"message": strings.TrimSuffix(TF(msgStreamInterrupte, streamErr), "\n"),
			},
		})
		return inputTokens, outputTokens
	}

	// --- Normal completion ---
	if textBlockOpen {
		writeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": blockIndex})
	}
	for _, idx := range toolOrder {
		writeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": tools[idx].blockIdx})
	}

	writeSSE("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
		// input_tokens in message_delta — "beyond" the Anthropic spec, but harmless:
		// message_start goes before we learn usage, so real
		// input_tokens is only available here. The client reads output_tokens,
		// input_tokens — for completeness.
		"usage": map[string]any{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	})
	writeSSE("message_stop", map[string]any{"type": "message_stop"})

	return inputTokens, outputTokens
}
