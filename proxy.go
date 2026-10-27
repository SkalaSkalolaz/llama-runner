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

// localModelName is the model ID that routes to the local llama-server. Any
// other model name received by the proxy is forwarded to the external
// OpenAI-compatible provider.
const localModelName = "llama-server"

// maxRequestBodyBytes is the hard limit on incoming request bodies, enforced
// with http.MaxBytesReader. Real Claude Code payloads stay below 10 MB; the
// 100 MB ceiling adds a large safety margin for base64-encoded images and
// very long conversation histories while still capping pathological input.
const maxRequestBodyBytes int64 = 100 * 1024 * 1024

// proxyHandler implements the Anthropic Messages API facade that routes
// traffic between Claude Code and either the local llama-server or an
// external OpenAI-compatible provider, converting requests and responses
// between the two API dialects.
type proxyHandler struct {
	// llamaServerURL is the base URL (no trailing slash) of the local
	// llama-server used for requests whose model is localModelName.
	llamaServerURL string
	// cfg supplies the external provider settings, proxy token and verbosity.
	cfg *Config
	// client performs upstream requests without a whole-request timeout
	// (see newProxyClient).
	client *http.Client
	// logWriter receives [proxy]-prefixed diagnostic lines when -v is set;
	// nil disables logging entirely.
	logWriter io.Writer
}

// StartProxyServer binds an HTTP listener on proxyAddr and starts serving the
// proxy in a background goroutine, returning the *http.Server so the caller
// can shut it down gracefully.
//
// All incoming requests pass through authMiddleware: with cfg.ProxyToken set,
// every request must present the token; with it empty, no authentication is
// performed.
//
// logWriter is the sink for [proxy] diagnostic lines emitted under -v —
// normally os.Stderr when output is piped/redirected, or the open
// llama-server log file in interactive mode (to avoid corrupting the TUI).
// nil disables proxy logging.
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

// newProxyMux assembles the proxy's routing table:
//
//	/v1/messages                 → handleMessages (the core endpoint)
//	/v1/models                   → handleModels
//	/v1/messages/count_tokens    → handleOther (approximate count)
//	/v1/models/<id>              → handleOther (single model echo)
//	/                            → handleOther (404 for everything else)
//
// It is factored out of StartProxyServer so that end-to-end tests can serve
// the exact same wiring (httptest.NewServer over authMiddleware+mux) without
// binding a real port.
func newProxyMux(h *proxyHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/messages", h.handleMessages)
	mux.HandleFunc("/v1/models", h.handleModels)
	mux.HandleFunc("/", h.handleOther)
	mux.HandleFunc("/v1/messages/count_tokens", h.handleOther)
	mux.HandleFunc("/v1/models/", h.handleOther)
	return mux
}

// authMiddleware wraps next with bearer-token authentication. When
// -proxy-token is configured, every request must present a matching token
// (see requestToken); mismatches and missing tokens are rejected with
// 401 authentication_error before any routing happens.
//
// With an empty token the request chain is returned unwrapped — no
// authentication at all. This preserves backward compatibility: the default
// localhost-only launch keeps working without any credentials.
func (h *proxyHandler) authMiddleware(next http.Handler) http.Handler {
	if h.cfg == nil || h.cfg.ProxyToken == "" {
		return next
	}
	want := h.cfg.ProxyToken
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestToken(r) != want {
			h.logf(T(msgProxyAuthReject), r.Method, r.URL.Path)
			writeAnthropicError(w, http.StatusUnauthorized,
				"authentication_error", "invalid or missing API token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requestToken extracts the client-supplied API token from the request
// headers, checking Authorization: Bearer first and then x-api-key.
// Different Claude Code versions use one or the other scheme, so the proxy
// accepts both. Returns "" when no token is present in either header.
func requestToken(r *http.Request) string {
	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
		return strings.TrimPrefix(a, "Bearer ")
	}
	if k := r.Header.Get("x-api-key"); k != "" {
		return k
	}
	return ""
}

// isLoopbackHost reports whether the host part of a host:port listener
// address refers to the local machine: the literal "localhost", or any IP
// literal in the loopback ranges (127.0.0.0/8, ::1). A value that cannot be
// split into host and port, or an empty host, counts as non-loopback so the
// caller errs on the side of warning. Used by the "proxy exposed to the
// network" safety check.
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

// shouldWarnProxyExposure reports whether starting the proxy is worth a
// security warning: it listens on a non-loopback address, no authentication
// token is configured, and the user has not opted in with
// -proxy-allow-remote. In that combination anyone on the network could use
// the proxy — and through it the paid external provider key.
//
// This only drives a stderr warning; it is deliberately not a hard failure,
// since binding a reachable address may be exactly what the user wants.
func shouldWarnProxyExposure(proxyAddr string, cfg *Config) bool {
	if cfg == nil || isLoopbackHost(proxyAddr) {
		return false
	}
	return cfg.ProxyToken == "" && !cfg.ProxyAllowRemote
}

// newProxyClient builds the http.Client used for all upstream requests.
//
// Client.Timeout is 0 on purpose: a whole-request timeout would cut long
// generations off mid-stream after a fixed period (defect #8). Protection
// against a dead upstream is instead applied where it belongs —
// Transport.ResponseHeaderTimeout of 30 seconds, which bounds only the wait
// for response headers while leaving the stream body open as long as needed.
// Abandoning an in-flight request is driven by cancellation of the incoming
// request context (NewRequestWithContext with r.Context() in
// forwardToLlama/forwardToOpenAI).
//
// The transport is a Clone of http.DefaultTransport with that single field
// changed, preserving environment proxy settings, HTTP/2 and keep-alives.
func newProxyClient() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{Timeout: 0, Transport: t}
}

// ---- /v1/models ----

// handleModels serves GET /v1/models with an Anthropic-format model list so
// the proxy passes Claude Code's startup model check. It advertises the local
// llama-server, the configured external model, and the standard aliases
// (sonnet/haiku/opus) — the aliases are listed for visibility only; actual
// routing sends anything that is not localModelName upstream.
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
	// Standard aliases — for visibility in /v1/models.
	// Routing still goes to openai since they aren't "llama-server".
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

// handleMessages is the core route, POST /v1/messages. It reads the body
// under maxRequestBodyBytes (oversized bodies get 413 request_too_large,
// other read errors 400), parses the Anthropic request, and routes it by the
// requested model: the localModelName (case- and whitespace-insensitive) goes
// to the local llama-server, everything else to the external provider.
//
// The stream flag from the request body selects between streaming (SSE) and
// single-response conversion downstream.
func (h *proxyHandler) handleMessages(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeAnthropicError(w, http.StatusRequestEntityTooLarge,
				"request_too_large", strings.TrimSuffix(TF(msgBodyTooLarge, maxErr.Limit), "\n"))
			return
		}
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	defer r.Body.Close()

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", strings.TrimSuffix(TF(msgInvalidJSON, err), "\n"))
		return
	}
	model, _ := req["model"].(string)
	stream, _ := req["stream"].(bool)

	isLocal := strings.EqualFold(strings.TrimSpace(model), localModelName)

	if isLocal {
		h.forwardToLlama(w, r, req, stream)
		return
	}
	h.forwardToOpenAI(w, r, req, stream)
}

// ---- Route to llama-server ----

// forwardToLlama converts the Anthropic request to OpenAI chat/completions
// format and sends it to the local llama-server (which needs no
// authentication). For streaming requests stream_options.include_usage is
// added so token usage arrives in the final chunk.
//
// Upstream failures map to Anthropic errors: transport errors become 502,
// non-200 responses are relayed with their status and a mapped error type.
// On success the response is converted back to Anthropic format — SSE events
// when streaming, a single JSON message otherwise — and token usage is logged
// under -v.
func (h *proxyHandler) forwardToLlama(w http.ResponseWriter, r *http.Request, req map[string]any, stream bool) {
	openAIReq, err := h.convert(req)
	if err != nil {
		writeAnthropicError(w, http.StatusInternalServerError, "api_error", strings.TrimSuffix(TF(msgConvertErr, err), "\n"))
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
		writeAnthropicError(w, http.StatusBadGateway, "api_error", strings.TrimSuffix(TF(msgUpstreamErr, err), "\n"))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		writeAnthropicError(w, resp.StatusCode, anthropicErrorType(resp.StatusCode),
			strings.TrimSuffix(TF(msgLlamaUpstream, string(b)), "\n"))
		return
	}
	if stream {
		in, out := convertOpenAIStreamToAnthropic(w, resp, "llama-server")
		h.logf(T(msgProxyUsage), in, out)
		return
	}
	convertOpenAIResponseToAnthropic(w, resp, "llama-server")
}

// ---- Route to external OpenAI-compatible service ----

// forwardToOpenAI converts the Anthropic request to OpenAI chat/completions
// format and forwards it to the configured external provider
// (cfg.OpenAIBaseURL + /chat/completions), authenticating with
// Authorization: Bearer cfg.OpenAIKey. Like forwardToLlama it requests usage
// accounting for streams, maps transport/upstream failures to Anthropic error
// responses, and converts successful responses — streaming or not — back to
// the Anthropic format.
func (h *proxyHandler) forwardToOpenAI(w http.ResponseWriter, r *http.Request, req map[string]any, stream bool) {
	openAIReq, err := h.convert(req)
	if err != nil {
		writeAnthropicError(w, http.StatusInternalServerError, "api_error", strings.TrimSuffix(TF(msgConvertErr, err), "\n"))
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
		writeAnthropicError(w, http.StatusBadGateway, "api_error", strings.TrimSuffix(TF(msgUpstreamOpenai, err), "\n"))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		writeAnthropicError(w, resp.StatusCode, anthropicErrorType(resp.StatusCode),
			strings.TrimSuffix(TF(msgOpenaiUpstream, string(b)), "\n"))
		return
	}
	if stream {
		in, out := convertOpenAIStreamToAnthropic(w, resp, h.cfg.OpenAIModel)
		h.logf(T(msgProxyUsage), in, out)
		return
	}
	convertOpenAIResponseToAnthropic(w, resp, h.cfg.OpenAIModel)
}

// ---- Other (/v1/complete, etc.) ----

// handleOther is the catch-all route for everything that is not
// /v1/messages or the /v1/models collection:
//
//   - */count_tokens — answered with an approximate token count
//     (body size / 4, minimum 1); Claude Code only needs a plausible number
//     to render its context meter, not an exact tokenizer result;
//   - GET /v1/models/<id> — echoes a minimal single-model object so lookups
//     of a known ID succeed;
//   - anything else — 404 not_found_error.
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

// ============================================================
// OpenAI → Anthropic conversion (response, non-streaming)
// ============================================================

// convertOpenAIResponseToAnthropic reads a complete (non-streaming) OpenAI
// chat/completions response, decodes it and writes the equivalent Anthropic
// Messages API JSON response to w: text content becomes a text block, each
// tool_calls entry a tool_use block (arguments parsed back into an input
// object), and finish_reason is mapped to a stop_reason
// (tool_calls→tool_use, length→max_tokens, otherwise end_turn). Token usage
// is carried over; a decode failure of the upstream body yields 502.
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
// Utility functions
// ============================================================

// writeJSON sets the JSON content type, writes status, and encodes v as the
// response body. Encoding errors are swallowed: headers are already sent, so
// there is nothing meaningful left to report to the client.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeAnthropicError writes a well-formed Anthropic Messages API error:
//
//	{"type":"error","error":{"type":"...","message":"..."}}
//
// http.Error must not be used instead: it returns text/plain, which Claude
// Code displays as an opaque "API Error" without parsing the message.
//
// Only usable on error paths BEFORE a response body has started. Once the
// headers and the first SSE event are on the wire, the status is fixed and
// the only way to signal failure inside a stream is an `event: error` SSE
// event (see convertOpenAIStreamToAnthropic).
func writeAnthropicError(w http.ResponseWriter, status int, errType, message string) {
	writeJSON(w, status, map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errType,
			"message": message,
		},
	})
}

// anthropicErrorType maps an upstream HTTP status code to the closest
// Anthropic error type (invalid_request_error, authentication_error,
// permission_error, not_found_error, request_too_large, rate_limit_error);
// any unmapped status falls back to the generic api_error.
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

// logf writes a [proxy]-prefixed diagnostic line to h.logWriter, but only
// when -v is set (or drops it silently otherwise, including when no writer
// was configured).
//
// Routing the lines through logWriter rather than os.Stderr directly keeps
// Claude Code's interactive TUI clean: with -quiet-server on a TTY the sink
// is the llama-server log file, and when output is piped it is stderr —
// either way never into the live terminal UI.
func (h *proxyHandler) logf(format string, args ...any) {
	if h.cfg == nil || !h.cfg.Verbose || h.logWriter == nil {
		return
	}
	fmt.Fprintf(h.logWriter, "[proxy] "+format+"\n", args...)
}

// convert runs the Anthropic→OpenAI request conversion with options derived
// from cfg (image blocks are only mapped when a vision projector is
// configured via -mmproj) and logs the result under -v: a one-line summary of
// the converted request, a preview of each role=tool message, and a note
// listing dropped content blocks (thinking has no OpenAI equivalent; image
// needs -mmproj). The conversion statistics make silent data loss visible.
func (h *proxyHandler) convert(req map[string]any) (map[string]any, error) {
	opts := ConvertOptions{ImagesToOpenAI: h.cfg != nil && h.cfg.Mmproj != ""}
	out, st, err := ConvertRequest(req, opts)
	if err != nil {
		return out, err
	}
	h.logf(T(msgProxyUpstream), requestSummary(out))
	for _, line := range toolMessagesPreview(out) {
		h.logf("%s", line)
	}
	if st.Dropped() > 0 {
		h.logf(T(msgProxyDropped), st.Summary())
	}
	return out, err
}
