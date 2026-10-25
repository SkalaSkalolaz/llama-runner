package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const localModelName = "llama-server"

// maxRequestBodyBytes — лимит тела запроса. Реальные payload'ы Claude Code
// < 10 МБ; 100 МБ — с большим запасом (image base64, длинная история).
const maxRequestBodyBytes int64 = 100 * 1024 * 1024

type proxyHandler struct {
	llamaServerURL string
	cfg            *Config
	client         *http.Client
	logWriter      io.Writer
}

// StartProxyServer поднимает локальный прокси и возвращает *http.Server для Shutdown.
// logWriter — приёмник [proxy]-строк при -v; обычно os.Stderr (пайп/редирект)
// или открытый лог-файл llama-server (интерактивный режим). nil — логи выключены.
func StartProxyServer(proxyAddr, llamaServerURL string, cfg *Config, logWriter io.Writer) (*http.Server, error) {
	h := &proxyHandler{
		llamaServerURL: strings.TrimRight(llamaServerURL, "/"),
		cfg:            cfg,
		client:         newProxyClient(),
		logWriter:      logWriter,
	}

	mux := newProxyMux(h)

	ln, err := net.Listen("tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("не удалось занять %s: %w", proxyAddr, err)
	}
	srv := &http.Server{Handler: h.authMiddleware(mux)}
	go func() {
		_ = srv.Serve(ln) // http.ErrServerClosed при Shutdown — нормально
	}()
	return srv, nil
}

// newProxyMux собирает маршруты прокси. Вынесено из StartProxyServer
// для end-to-end-тестов: httptest.NewServer(h.authMiddleware(newProxyMux(h)))
// проверяет ту же схему, что и боевой запуск.
func newProxyMux(h *proxyHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/messages", h.handleMessages)
	mux.HandleFunc("/v1/models", h.handleModels)
	mux.HandleFunc("/", h.handleOther)
	mux.HandleFunc("/v1/messages/count_tokens", h.handleOther)
	mux.HandleFunc("/v1/models/", h.handleOther)
	return mux
}

// authMiddleware проверяет токен, если -proxy-token задан. Пустой токен —
// проверка отключена (backward-compat: дефолтный запуск работает как раньше).
func (h *proxyHandler) authMiddleware(next http.Handler) http.Handler {
	if h.cfg == nil || h.cfg.ProxyToken == "" {
		return next
	}
	want := h.cfg.ProxyToken
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestToken(r) != want {
			h.logf("auth rejected: %s %s (token mismatch or missing)", r.Method, r.URL.Path)
			writeAnthropicError(w, http.StatusUnauthorized,
				"authentication_error", "invalid or missing API token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requestToken извлекает токен из заголовков запроса. Разные версии
// Claude Code шлют либо Authorization: Bearer, либо x-api-key.
func requestToken(r *http.Request) string {
	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
		return strings.TrimPrefix(a, "Bearer ")
	}
	if k := r.Header.Get("x-api-key"); k != "" {
		return k
	}
	return ""
}

// isLoopbackHost — true, если адрес слушателя loopback (127.0.0.1, ::1,
// localhost). Нужно для предупреждения «прокси открыт в сеть».
func isLoopbackHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil || host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// shouldWarnProxyExposure — true, если прокси слушает не-loopback адрес,
// аутентификация выключена и явного разрешения нет. Только предупреждение,
// не hard-fail (красная зона №3).
func shouldWarnProxyExposure(proxyAddr string, cfg *Config) bool {
	if cfg == nil || isLoopbackHost(proxyAddr) {
		return false
	}
	return cfg.ProxyToken == "" && !cfg.ProxyAllowRemote
}

// newProxyClient — http.Client для запросов к апстриму.
//
// Client.Timeout = 0: иначе 5-минутный лимит режет длинную генерацию
// посреди стрима (дефект №8). Защита от зависшего соединения — на уровне
// Transport.ResponseHeaderTimeout: 30s (заголовки ждём не дольше 30 с,
// тело стрима — сколько нужно). Отмена — по r.Context() в
// forwardToLlama/forwardToOpenAI, там NewRequestWithContext.
//
// Транспорт — копия http.DefaultTransport с одним изменённым полем:
// сохраняем проксирование из env, HTTP/2, keep-alive.
func newProxyClient() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{Timeout: 0, Transport: t}
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
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeAnthropicError(w, http.StatusRequestEntityTooLarge,
				"request_too_large", fmt.Sprintf("тело запроса больше %d байт", maxErr.Limit))
			return
		}
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
	if stream {
		openAIReq["stream_options"] = map[string]any{"include_usage": true}
	}

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
		in, out := convertOpenAIStreamToAnthropic(w, resp, "llama-server")
		h.logf("← usage: input_tokens=%d, output_tokens=%d", in, out)
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
	if stream {
		openAIReq["stream_options"] = map[string]any{"include_usage": true}
	}
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
		in, out := convertOpenAIStreamToAnthropic(w, resp, h.cfg.OpenAIModel)
		h.logf("← usage: input_tokens=%d, output_tokens=%d", in, out)
		return
	}
	convertOpenAIResponseToAnthropic(w, resp, h.cfg.OpenAIModel)
}

// ---- Прочее (/v1/complete и т.п.) ----

func (h *proxyHandler) handleOther(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/count_tokens") {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
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

// logf — вывод прокси только при -v. Пишет в h.logWriter, чтобы в
// интерактивном режиме не засорять TUI Claude Code: в TTY при -quiet-server
// логи уходят в тот же файл, что и у llama-server, в пайпе — в stderr.
func (h *proxyHandler) logf(format string, args ...any) {
	if h.cfg == nil || !h.cfg.Verbose || h.logWriter == nil {
		return
	}
	fmt.Fprintf(h.logWriter, "[proxy] "+format+"\n", args...)
}

func (h *proxyHandler) convert(req map[string]any) (map[string]any, error) {
	opts := ConvertOptions{ImagesToOpenAI: h.cfg != nil && h.cfg.Mmproj != ""}
	out, st, err := ConvertRequest(req, opts)
	if err != nil {
		return out, err
	}
	h.logf("→ апстрим: %s", requestSummary(out))
	for _, line := range toolMessagesPreview(out) {
		h.logf("%s", line)
	}
	if st.Dropped() > 0 {
		h.logf("отброшено блоков: %s (thinking не имеет аналога в OpenAI, image требует -mmproj)", st.Summary())
	}
	return out, err
}
