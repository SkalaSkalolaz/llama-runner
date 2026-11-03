package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

// hybridCfg — полностью настроенный внешний провайдер: именно он включает
// прокси hybrid-режимом (см. proxyHandler.externalMode). URL намеренно
// несуществующий: маршрутизируемые тесты к нему не обращаются.
func hybridCfg() *config.Config {
	return &config.Config{OpenAIBaseURL: "http://provider.invalid/v1", OpenAIModel: "m", OpenAIKey: "k"}
}

func TestHandleMessages_InvalidJSONIsAnthropicError(t *testing.T) {
	h := &proxyHandler{cfg: hybridCfg()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{not json"))
	h.handleMessages(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", w.Code)
	}
	var got struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("тело не JSON Anthropic-ошибки: %v (%s)", err, w.Body.String())
	}
	if got.Type != "error" {
		t.Errorf("type = %q, want error", got.Type)
	}
}

// --- путь целиком: /v1/messages → конвертер → апстрим ---

func TestProxy_EndToEnd_ToolResultReachesUpstream(t *testing.T) {
	// Дефект №1 end-to-end: результат инструмента обязан дойти до модели.
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&upstreamBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1","choices":[{"message":{"content":"готово"},"finish_reason":"stop"}]}`))
	}))
	defer upstream.Close()

	h := &proxyHandler{
		llamaServerURL: upstream.URL,
		cfg:            hybridCfg(),
		client:         upstream.Client(),
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(
		`{"model":"llama-server","max_tokens":10,"messages":[`+
			`{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"Bash","input":{"command":"ls"}}]},`+
			`{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","content":`+
			`[{"type":"text","text":"file1"},{"type":"text","text":"file2"}]}]}]}`))
	h.handleMessages(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", w.Code, w.Body.String())
	}
	msgs := asMessages(t, upstreamBody["messages"])
	if len(msgs) != 2 {
		t.Fatalf("апстрим получил messages = %v", upstreamBody["messages"])
	}
	if msgs[1]["role"] != "tool" {
		t.Fatalf("roles = %v, want [assistant tool]", roles(msgs))
	}
	content, _ := msgs[1]["content"].(string)
	if !strings.Contains(content, "file1") || !strings.Contains(content, "file2") {
		t.Errorf("апстрим не получил результат инструмента: %q", content)
	}
}

func TestProxy_UpstreamErrorIsAnthropicFormat(t *testing.T) {
	// Дефект №5: ошибки апстрима отдаются в формате Anthropic, а не text/plain.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "context length exceeded", http.StatusBadRequest)
	}))
	defer upstream.Close()

	h := &proxyHandler{
		llamaServerURL: upstream.URL,
		cfg:            hybridCfg(),
		client:         upstream.Client(),
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"llama-server","messages":[{"role":"user","content":"hi"}]}`))
	h.handleMessages(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", w.Code)
	}
	var got struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("тело не JSON Anthropic-ошибки: %v (%s)", err, w.Body.String())
	}
	if got.Type != "error" || got.Error.Type != "invalid_request_error" {
		t.Errorf("тело = %+v", got)
	}
	if !strings.Contains(got.Error.Message, "context length exceeded") {
		t.Errorf("сообщение апстрима потеряно: %q", got.Error.Message)
	}
}

// --- хелперы ---

// bodyCapturingUpstream answers with a canned body and exposes the last
// request body it saw through a mutex-guarded getter.
func bodyCapturingUpstream(t *testing.T, response string) (*httptest.Server, func() []byte) {
	t.Helper()
	var mu sync.Mutex
	var last []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		last = body
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []byte {
		mu.Lock()
		defer mu.Unlock()
		return append([]byte(nil), last...)
	}
}

// asMessages — messages апстрима (после JSON-декодирования это []any).
func asMessages(t *testing.T, v any) []map[string]any {
	t.Helper()
	raw, ok := v.([]any)
	if !ok {
		t.Fatalf("messages = %T, want []any", v)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, m := range raw {
		mm, ok := m.(map[string]any)
		if !ok {
			t.Fatalf("элемент messages = %T", m)
		}
		out = append(out, mm)
	}
	return out
}
func roles(m []map[string]any) []string {
	out := make([]string, 0, len(m))
	for _, x := range m {
		out = append(out, x["role"].(string))
	}
	return out
}

// --- hybrid: маршрутизация по имени модели ---

func TestHybrid_LocalAliasRoutesToLlama(t *testing.T) {
	upstream, captured := bodyCapturingUpstream(t,
		`{"id":"c1","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
	h := &proxyHandler{
		llamaServerURL: upstream.URL,
		cfg: &config.Config{
			Alias:         "qwen3-coder",
			OpenAIBaseURL: "http://provider.invalid/v1",
			OpenAIModel:   "m",
			OpenAIKey:     "k",
		},
		client: upstream.Client(),
	}
	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(
		`{"model":"qwen3-coder","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", w.Code, w.Body.String())
	}
	if len(captured()) == 0 {
		t.Fatal("llama-server was not called")
	}
}

func TestHybrid_ModelsIncludesLocalAlias(t *testing.T) {
	h := &proxyHandler{
		cfg: &config.Config{
			Alias:         "qwen3-coder",
			OpenAIBaseURL: "https://api.example/v1",
			OpenAIModel:   "gpt-4",
			OpenAIKey:     "k",
		},
	}
	w := httptest.NewRecorder()
	h.handleModels(w, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if !strings.Contains(w.Body.String(), "qwen3-coder") {
		t.Errorf("local alias missing in /v1/models: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "gpt-4") {
		t.Errorf("external model missing in /v1/models: %s", w.Body.String())
	}
}
