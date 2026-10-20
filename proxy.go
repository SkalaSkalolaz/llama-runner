package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// proxyHandler маршрутизирует запросы Anthropic API между llama-server и внешним OpenAI.
type proxyHandler struct {
	llamaServerURL string
	cfg            *Config
	client         *http.Client
}

// StartProxyServer поднимает локальный прокси и возвращает *http.Server для Shutdown.
func StartProxyServer(proxyAddr, llamaServerURL string, cfg *Config) (*http.Server, error) {
	h := &proxyHandler{
		llamaServerURL: strings.TrimRight(llamaServerURL, "/"),
		cfg:            cfg,
		client:         &http.Client{Timeout: 5 * time.Minute},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/messages", h.handleMessages)
	mux.HandleFunc("/v1/models", h.handleModels)
	mux.HandleFunc("/", h.handleOther)
    mux.HandleFunc("/v1/messages/count_tokens", h.handleOther)
    mux.HandleFunc("/v1/models/", h.handleOther)
    
	ln, err := net.Listen("tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("не удалось занять %s: %w", proxyAddr, err)
	}
	srv := &http.Server{Handler: mux}
	go func() {
		_ = srv.Serve(ln) // http.ErrServerClosed при Shutdown — нормально
	}()
	return srv, nil
}

// ---- /v1/models ----

func (h *proxyHandler) handleModels(w http.ResponseWriter, r *http.Request) {
	type model struct {
		ID          string `json:"id"`
		Type        string `json:"type"`
		DisplayName string `json:"display_name"`
		CreatedAt   string `json:"created_at"`
	}
	localID := h.cfg.Alias
	if localID == "" {
		localID = "local"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	resp := map[string]any{
		"data": []model{
			{ID: localID, Type: "model", DisplayName: localID, CreatedAt: now},
			{ID: h.cfg.OpenAIModel, Type: "model", DisplayName: h.cfg.OpenAIModel, CreatedAt: now},
		},
		"has_more": false,
		"first_id": localID,
		"last_id":  h.cfg.OpenAIModel,
	}
	writeJSON(w, http.StatusOK, resp)
}
// ---- /v1/messages ----

func (h *proxyHandler) handleMessages(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	model, _ := req["model"].(string)
	stream, _ := req["stream"].(bool)

	// Выбор маршрута: если модель совпадает с OpenAIModel — внешний, иначе — llama-server.
	// Сравнение без учёта регистра и с обрезкой пробелов — на всякий случай.
	isExternal := strings.EqualFold(strings.TrimSpace(model), strings.TrimSpace(h.cfg.OpenAIModel))

	if isExternal {
		h.forwardToOpenAI(w, r, req, stream)
		return
	}
	h.forwardToLlama(w, r, req, stream)
}

// ---- Маршрут в llama-server ----

func (h *proxyHandler) forwardToLlama(w http.ResponseWriter, r *http.Request, req map[string]any, stream bool) {
	// llama-server понимает только OpenAI API. Конвертируем Anthropic → OpenAI.
	openAIReq, err := AnthropicToOpenAI(req)
	if err != nil {
		http.Error(w, "convert: "+err.Error(), http.StatusInternalServerError)
		return
	}
	openAIReq["stream"] = stream

	target := h.llamaServerURL + "/v1/chat/completions"
	body, _ := json.Marshal(openAIReq)
	proxyReq, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, target, bytes.NewReader(body))
	proxyReq.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(proxyReq)
	if err != nil {
		http.Error(w, "llama-server: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		http.Error(w, "llama-server upstream: "+string(b), resp.StatusCode)
		return
	}

	if stream {
		convertOpenAIStreamToAnthropic(w, resp, h.cfg.Alias)
		return
	}
	convertOpenAIResponseToAnthropic(w, resp, h.cfg.Alias)
}

// ---- Маршрут во внешний OpenAI-совместимый сервис ----

func (h *proxyHandler) forwardToOpenAI(w http.ResponseWriter, r *http.Request, req map[string]any, stream bool) {
	openAIReq, err := AnthropicToOpenAI(req)
	if err != nil {
		http.Error(w, "convert: "+err.Error(), http.StatusInternalServerError)
		return
	}
	openAIReq["stream"] = stream

	target := strings.TrimRight(h.cfg.OpenAIBaseURL, "/") + "/chat/completions"
	body, _ := json.Marshal(openAIReq)
	proxyReq, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, target, bytes.NewReader(body))
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("Authorization", "Bearer "+h.cfg.OpenAIKey)

	resp, err := h.client.Do(proxyReq)
	if err != nil {
		http.Error(w, "openai: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		http.Error(w, "openai upstream: "+string(b), resp.StatusCode)
		return
	}

	if stream {
		convertOpenAIStreamToAnthropic(w, resp, h.cfg.OpenAIModel)
		return
	}
	convertOpenAIResponseToAnthropic(w, resp, h.cfg.OpenAIModel)
}

// ---- Прочее (/v1/complete и т.п.) ----

func (h *proxyHandler) handleOther(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/count_tokens") {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		model, _ := req["model"].(string)

		approx := len(body) / 4
		if approx < 1 {
			approx = 1
		}
		_ = model
		writeJSON(w, http.StatusOK, map[string]any{"input_tokens": approx})
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/models/") && r.Method == http.MethodGet {
		id := strings.TrimPrefix(r.URL.Path, "/v1/models/")
		writeJSON(w, http.StatusOK, map[string]any{
			"id": id, "type": "model", "display_name": id,
		})
		return
	}

	http.Error(w, "not implemented: "+r.URL.Path, http.StatusNotFound)
}

// ============================================================
// Конвертация Anthropic → OpenAI (запрос)
// ============================================================

func AnthropicToOpenAI(anthropic map[string]any) (map[string]any, error) {
	openAI := map[string]any{
		"model": anthropic["model"],
	}

	// system → первое сообщение role=system
	var messages []map[string]any
	if sys, ok := anthropic["system"].(string); ok && sys != "" {
		messages = append(messages, map[string]any{"role": "system", "content": sys})
	}

	// messages: content может быть строкой или массивом блоков
	if inMsgs, ok := anthropic["messages"].([]any); ok {
		for _, m := range inMsgs {
			msg, ok := m.(map[string]any)
			if !ok {
				continue
			}
			role, _ := msg["role"].(string)
			content, _ := msg["content"]

			switch v := content.(type) {
			case string:
				messages = append(messages, map[string]any{"role": role, "content": v})
			case []any:
				// Собираем текст из блоков, non-text блоки (tools) преобразуем в tool_calls
				var textParts []string
				var toolCalls []map[string]any
				for _, b := range v {
					block, ok := b.(map[string]any)
					if !ok {
						continue
					}
					btype, _ := block["type"].(string)
					switch btype {
					case "text":
						if t, ok := block["text"].(string); ok {
							textParts = append(textParts, t)
						}
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
						// tool_result в Anthropic идёт отдельным user-сообщением; для OpenAI это role=tool
						toolContent := block["content"]
						if s, ok := toolContent.(string); ok {
							messages = append(messages, map[string]any{
								"role":         "tool",
								"tool_call_id": block["tool_use_id"],
								"content":      s,
							})
						}
					}
				}
				if len(toolCalls) > 0 {
					messages = append(messages, map[string]any{
						"role":       "assistant",
						"content":    strings.Join(textParts, ""),
						"tool_calls": toolCalls,
					})
				} else if len(textParts) > 0 {
					messages = append(messages, map[string]any{
						"role":    role,
						"content": strings.Join(textParts, ""),
					})
				}
			}
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

	return openAI, nil
}

// ============================================================
// Конвертация OpenAI → Anthropic (ответ, non-streaming)
// ============================================================

func convertOpenAIResponseToAnthropic(w http.ResponseWriter, openAIResp *http.Response, modelName string) {
	var oai struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(openAIResp.Body).Decode(&oai); err != nil {
		http.Error(w, "decode: "+err.Error(), http.StatusBadGateway)
		return
	}

	var content []map[string]any
	if len(oai.Choices) > 0 && oai.Choices[0].Message.Content != "" {
		content = append(content, map[string]any{
			"type": "text",
			"text": oai.Choices[0].Message.Content,
		})
	}
	if len(oai.Choices) > 0 && len(oai.Choices[0].Message.ToolCalls) > 0 {
		for _, tc := range oai.Choices[0].Message.ToolCalls {
			var input map[string]any
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
			content = append(content, map[string]any{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Function.Name,
				"input": input,
			})
		}
	}

	stopReason := "end_turn"
	if len(oai.Choices) > 0 {
		switch oai.Choices[0].FinishReason {
		case "tool_calls":
			stopReason = "tool_use"
		case "length":
			stopReason = "max_tokens"
		}
	}

	resp := map[string]any{
		"id":            oai.ID,
		"type":          "message",
		"role":          "assistant",
		"model":         modelName,
		"content":       content,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":  oai.Usage.PromptTokens,
			"output_tokens": oai.Usage.CompletionTokens,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

// ============================================================
// Конвертация стриминга (SSE)
// ============================================================


func convertOpenAIStreamToAnthropic(w http.ResponseWriter, openAIResp *http.Response, modelName string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
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

	scanner := bufio.NewScanner(openAIResp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
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
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]

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

		if choice.FinishReason == "tool_calls" {
			stopReason = "tool_use"
		} else if choice.FinishReason == "length" {
			stopReason = "max_tokens"
		}
	}

	// Закрываем последний открытый блок
	if textBlockOpen {
		writeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": blockIndex})
	}
	for _, idx := range toolOrder {
		writeSSE("content_block_stop", map[string]any{"type": "content_block_stop", "index": tools[idx].blockIdx})
	}

	writeSSE("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
		"usage": map[string]any{"output_tokens": 0},
	})
	writeSSE("message_stop", map[string]any{"type": "message_stop"})
}

// ============================================================
// Утилиты
// ============================================================

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}