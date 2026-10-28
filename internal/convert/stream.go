package convert

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// ============================================================
// OpenAI → Anthropic streaming conversion (SSE)
// ============================================================

// ConvertOpenAIStreamToAnthropic consumes an OpenAI streaming
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
func ConvertOpenAIStreamToAnthropic(w http.ResponseWriter, openAIResp *http.Response, modelName string) (int, int) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteAnthropicError(w, http.StatusInternalServerError, "api_error", i18n.T(i18n.MsgStreamingUnsupp))
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
		streamErr = errors.New(i18n.T(i18n.MsgStreamNoDone))
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
				"message": strings.TrimSuffix(i18n.TF(i18n.MsgStreamInterrupte, streamErr), "\n"),
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
