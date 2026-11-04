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

func TestHybrid_LocalTokenStillRoutesLocal(t *testing.T) {
	// The canonical "llama-server" token must keep routing to llama-server
	// even when a custom Alias is set, so old env vars remain functional.
	upstream, captured := bodyCapturingUpstream(t,
		`{"id":"c1","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
	h := &proxyHandler{
		llamaServerURL: upstream.URL,
		cfg: &config.Config{
			Alias:         "qwen3-coder",
			OpenAIBaseURL: "http://provider.invalid/v1",
			OpenAIModel:   "gpt-4",
			OpenAIKey:     "k",
		},
		client: upstream.Client(),
	}
	w := httptest.NewRecorder()
	h.handleMessages(w, anthropicUserRequest("llama-server"))
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", w.Code, w.Body.String())
	}
	if len(captured()) == 0 {
		t.Fatal("llama-server was not called for the canonical token")
	}
}

func TestHybrid_ExternalNameGoesUpstream(t *testing.T) {
	// A name that is neither localModelName nor the local Alias must go to
	// the external provider. The provider URL is deliberately dead, so the
	// proxy reports 502.
	h := &proxyHandler{
		llamaServerURL: "http://llama.invalid",
		cfg: &config.Config{
			Alias:         "qwen3-coder",
			OpenAIBaseURL: "http://provider.invalid/v1",
			OpenAIModel:   "gpt-4",
			OpenAIKey:     "k",
		},
		client: http.DefaultClient,
	}
	w := httptest.NewRecorder()
	h.handleMessages(w, anthropicUserRequest("some-external-model"))
	if w.Code != http.StatusBadGateway {
		t.Errorf("code = %d, want 502 (external route)", w.Code)
	}
}

func TestHybrid_ModelsListLocalAliasWithoutDuplicates(t *testing.T) {
	// Even when Alias coincides with an existing entry, /v1/models must not
	// contain duplicates.
	for _, tc := range []struct {
		name  string
		alias string
	}{
		{"alias = external model", "gpt-4"},
		{"alias = standard (sonnet)", "sonnet"},
		{"alias = localModelName", "llama-server"},
		{"alias distinct", "qwen3-coder"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &proxyHandler{
				cfg: &config.Config{
					Alias:         tc.alias,
					OpenAIBaseURL: "https://api.example/v1",
					OpenAIModel:   "gpt-4",
					OpenAIKey:     "k",
				},
			}
			w := httptest.NewRecorder()
			h.handleModels(w, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
			var got struct {
				Data []modelEntry `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("bad JSON: %v", err)
			}
			ids := map[string]int{}
			for _, m := range got.Data {
				ids[strings.ToLower(m.ID)]++
			}
			for id, n := range ids {
				if n > 1 {
					t.Errorf("duplicate id %q (%d times) in %v", id, n, ids)
				}
			}
		})
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

// --- внешний endpoint: base + openaiPath ---

func TestForwardToOpenAI_Endpoint(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantPath string
	}{
		{"default", "/chat/completions", "/v1/chat/completions"},
		{"empty path (OVH-style)", "", "/v1"},
		{"custom path", "/completions", "/v1/completions"},
		{"path without leading slash", "chat/completions", "/v1/chat/completions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"c1","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
			}))
			defer upstream.Close()

			h := &proxyHandler{
				llamaServerURL: "http://llama.invalid",
				cfg: &config.Config{
					OpenAIBaseURL: upstream.URL + "/v1",
					OpenAIModel:   "ext/model-x",
					OpenAIKey:     "k",
					OpenAIPath:    tt.path,
				},
				client: http.DefaultClient,
			}
			w := httptest.NewRecorder()
			h.handleMessages(w, anthropicUserRequest("ext/model-x"))
			if w.Code != http.StatusOK {
				t.Fatalf("code = %d, body = %s", w.Code, w.Body.String())
			}
			if gotPath != tt.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tt.wantPath)
			}
		})
	}
}

func TestForwardToLlama_UsesLocalClient(t *testing.T) {
	// forwardToLlama (hybrid route) must use localClient, not client:
	// pointing a dead client at `client` and a working one at `localClient`
	// proves the route picks the right one.
	upstream, captured := bodyCapturingUpstream(t,
		`{"id":"c1","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadClient := dead.Client()
	dead.Close()

	h := &proxyHandler{
		llamaServerURL: upstream.URL,
		cfg: &config.Config{
			Alias:         "qwen3-coder",
			OpenAIBaseURL: "http://provider.invalid/v1",
			OpenAIModel:   "gpt-4",
			OpenAIKey:     "k",
		},
		client:      deadClient,
		localClient: upstream.Client(),
	}
	w := httptest.NewRecorder()
	h.handleMessages(w, anthropicUserRequest("qwen3-coder"))
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s (localClient must have been used)",
			w.Code, w.Body.String())
	}
	if len(captured()) == 0 {
		t.Fatal("llama-server was not called")
	}
}
