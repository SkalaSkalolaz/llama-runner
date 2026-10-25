package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const localModelName = "llama-server"

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
	now := time.Now().UTC().Format(time.RFC3339)

	data := []model{
		{ID: localModelName, Type: "model", DisplayName: localModelName, CreatedAt: now},
		{ID: h.cfg.OpenAIModel, Type: "model", DisplayName: h.cfg.OpenAIModel, CreatedAt: now},
	}
	// Стандартные алиасы — для наглядности в /v1/models.
	// Маршрутизация всё равно пойдёт на openai, т.к. они не "llama-server".
	for _, alias := range []string{"sonnet", "haiku", "opus"} {
		data = append(data, model{ID: alias, Type: "model", DisplayName: alias, CreatedAt: now})
	}

	resp := map[string]any{
		"data":     data,
		"has_more": false,
		"first_id": localModelName,
		"last_id":  "opus",
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---- /v1/messages ----

func (h *proxyHandler) handleMessages(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	defer r.Body.Close()

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON: "+err.Error())
		return
	}
	model, _ := req["model"].(string)
	stream, _ := req["stream"].(bool)

	// Локальный сервер — только явное имя "llama-server".
	// Всё остальное (sonnet, haiku, opus, gpt-4, ...) — внешний провайдер.
	isLocal := strings.EqualFold(strings.TrimSpace(model), localModelName)

	if isLocal {
		h.forwardToLlama(w, r, req, stream)
		return
	}
	h.forwardToOpenAI(w, r, req, stream)
}

// ---- Маршрут в llama-server ----

func (h *proxyHandler) forwardToLlama(w http.ResponseWriter, r *http.Request, req map[string]any, stream bool) {
	// llama-server понимает только OpenAI API. Конвертируем Anthropic → OpenAI.
	openAIReq, err := h.convert(req)
	if err != nil {
		writeAnthropicError(w, http.StatusInternalServerError, "api_error", "convert: "+err.Error())
		return
	}
	openAIReq["stream"] = stream

	target := h.llamaServerURL + "/v1/chat/completions"
	body, _ := json.Marshal(openAIReq)
	proxyReq, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, target, bytes.NewReader(body))
	proxyReq.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(proxyReq)
	if err != nil {
		writeAnthropicError(w, http.StatusBadGateway, "api_error", "llama-server: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		writeAnthropicError(w, resp.StatusCode, anthropicErrorType(resp.StatusCode),
			"llama-server upstream: "+string(b))
		return
	}

	if stream {
		convertOpenAIStreamToAnthropic(w, resp, "llama-server")
		return
	}
	convertOpenAIResponseToAnthropic(w, resp, "llama-server")
}

// ---- Маршрут во внешний OpenAI-совместимый сервис ----

func (h *proxyHandler) forwardToOpenAI(w http.ResponseWriter, r *http.Request, req map[string]any, stream bool) {
	openAIReq, err := h.convert(req)
	if err != nil {
		writeAnthropicError(w, http.StatusInternalServerError, "api_error", "convert: "+err.Error())
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
		writeAnthropicError(w, http.StatusBadGateway, "api_error", "openai: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		writeAnthropicError(w, resp.StatusCode, anthropicErrorType(resp.StatusCode),
			"openai upstream: "+string(b))
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

	writeAnthropicError(w, http.StatusNotFound, "not_found_error", "not implemented: "+r.URL.Path)
}

// Конвертация запроса Anthropic → OpenAI — в convert.go (веха M1).

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
		writeAnthropicError(w, http.StatusBadGateway, "api_error", "decode: "+err.Error())
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
		writeAnthropicError(w, http.StatusInternalServerError, "api_error", "streaming unsupported")
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

// writeAnthropicError — ошибка в формате Anthropic Messages API:
//
//	{"type":"error","error":{"type":"...","message":"..."}}
//
// http.Error отдаёт text/plain, который Claude Code показывает как «API Error»
// без разбора. Применять только для error-путей ДО начала стрима: когда заголовки
// и первый SSE-event отправлены, остаётся только `event: error` (см. M2).
func writeAnthropicError(w http.ResponseWriter, status int, errType, message string) {
	writeJSON(w, status, map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errType,
			"message": message,
		},
	})
}

// anthropicErrorType — тип ошибки Anthropic по HTTP-статусу апстрима.
func anthropicErrorType(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusRequestEntityTooLarge:
		return "request_too_large"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	}
	return "api_error"
}

// logf — вывод прокси только при -v.
func (h *proxyHandler) logf(format string, args ...any) {
	if h.cfg != nil && h.cfg.Verbose {
		fmt.Fprintf(os.Stderr, "[proxy] "+format+"\n", args...)
	}
}

// convert конвертирует запрос и при -v сообщает, что не дошло до модели
// (thinking/image/неизвестные блоки). image маппится только с vision-проектором.
func (h *proxyHandler) convert(req map[string]any) (map[string]any, error) {
	opts := ConvertOptions{ImagesToOpenAI: h.cfg != nil && h.cfg.Mmproj != ""}
	out, st, err := ConvertRequest(req, opts)
	if err != nil {
		return out, err
	}
	h.logf("→ апстрим: %s", requestSummary(out))
	if st.Dropped() > 0 {
		h.logf("отброшено блоков: %s (thinking не имеет аналога в OpenAI, image требует -mmproj)", st.Summary())
	}
	return out, nil
}
