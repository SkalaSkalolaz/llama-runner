package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

type fakeSwapper struct {
	current string
	aliases []string
	url     string
	err     error
	calls   []string
}

func (f *fakeSwapper) SwapTo(_ context.Context, a string) (string, error) {
	f.calls = append(f.calls, a)
	if f.err != nil {
		return "", f.err
	}
	f.current = a
	return f.url, nil
}
func (f *fakeSwapper) CurrentAlias() string { return f.current }
func (f *fakeSwapper) CurrentURL() string {
	if f.current == "" {
		return ""
	}
	return f.url
}
func (f *fakeSwapper) CatalogAliases() []string           { return f.aliases }
func (f *fakeSwapper) IncInflight()                       {}
func (f *fakeSwapper) DecInflight()                       {}
func (f *fakeSwapper) WaitInflight(context.Context) error { return nil }

func TestHandleMessages_SwitchTo_SwapsAndRoutes(t *testing.T) {
	upstream, captured := bodyCapturingUpstream(t, `{"id":"m1","type":"message"}`)
	sw := &fakeSwapper{current: "a", aliases: []string{"a", "b"}, url: upstream.URL}
	h := &proxyHandler{
		llamaServerURL: "http://oldserver:9999",
		cfg:            &config.Config{Alias: "a"},
		client:         upstream.Client(),
		localClient:    upstream.Client(),
		swapper:        sw,
	}
	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest("POST", "/v1/messages",
		strings.NewReader(`{"model":"switch-to:b","messages":[]}`)))
	if len(sw.calls) != 1 || sw.calls[0] != "b" {
		t.Errorf("calls = %v, want [b]", sw.calls)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", w.Code, w.Body.String())
	}
	if len(captured()) == 0 {
		t.Error("upstream not called after swap")
	}
}

func TestHandleMessages_SwitchTo_SameAlias_NoSwap(t *testing.T) {
	upstream, _ := bodyCapturingUpstream(t, `{}`)
	sw := &fakeSwapper{current: "a", aliases: []string{"a"}, url: upstream.URL}
	h := &proxyHandler{
		llamaServerURL: upstream.URL,
		cfg:            &config.Config{Alias: "a"},
		client:         upstream.Client(),
		localClient:    upstream.Client(),
		swapper:        sw,
	}
	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest("POST", "/v1/messages",
		strings.NewReader(`{"model":"switch-to:a","messages":[]}`)))
	if len(sw.calls) != 0 {
		t.Errorf("same-alias switch must not call SwapTo: %v", sw.calls)
	}
}

func TestHandleMessages_SwitchTo_Unknown_400(t *testing.T) {
	upstream, _ := bodyCapturingUpstream(t, `{}`)
	sw := &fakeSwapper{current: "a", aliases: []string{"a"}, url: upstream.URL}
	h := &proxyHandler{
		cfg:         &config.Config{Alias: "a"},
		client:      upstream.Client(),
		localClient: upstream.Client(),
		swapper:     sw,
	}
	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest("POST", "/v1/messages",
		strings.NewReader(`{"model":"switch-to:unknown","messages":[]}`)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", w.Code)
	}
	if len(sw.calls) != 0 {
		t.Errorf("SwapTo must not be called for unknown profile: %v", sw.calls)
	}
}

func TestHandleMessages_BareCatalogAlias_NoSwap(t *testing.T) {
	// This is the core anti-thrashing invariant: a bare alias, even if
	// it names a catalog profile, must NOT restart the server.
	upstream, _ := bodyCapturingUpstream(t, `{"ok":true}`)
	sw := &fakeSwapper{current: "a", aliases: []string{"a", "b"}, url: upstream.URL}
	h := &proxyHandler{
		llamaServerURL: upstream.URL,
		cfg:            &config.Config{Alias: "a"},
		client:         upstream.Client(),
		localClient:    upstream.Client(),
		swapper:        sw,
	}
	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest("POST", "/v1/messages",
		strings.NewReader(`{"model":"b","messages":[]}`)))
	if len(sw.calls) != 0 {
		t.Errorf("bare catalog alias must not trigger swap: %v", sw.calls)
	}
}

func TestHandleMessages_SwitchFails_502(t *testing.T) {
	sw := &fakeSwapper{current: "a", aliases: []string{"a", "b"}, err: errors.New("boom")}
	h := &proxyHandler{cfg: &config.Config{Alias: "a"}, swapper: sw}
	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest("POST", "/v1/messages",
		strings.NewReader(`{"model":"switch-to:b","messages":[]}`)))
	if w.Code != http.StatusBadGateway {
		t.Errorf("code = %d, want 502", w.Code)
	}
}

func TestHandleMessages_Swapper_Nil_Unchanged(t *testing.T) {
	// Existing passthrough tests already prove nil swapper → old path; add
	// an explicit guard here so a future regression is caught.
	upstream, _ := bodyCapturingUpstream(t, `{"ok":true}`)
	h := &proxyHandler{llamaServerURL: upstream.URL, cfg: &config.Config{},
		client: upstream.Client(), localClient: upstream.Client()}
	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest("POST", "/v1/messages",
		strings.NewReader(`{"model":"x","messages":[]}`)))
	if w.Code != http.StatusOK {
		t.Errorf("code = %d", w.Code)
	}
}

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

// --- путь целиком: /v1/messages → verbatim passthrough → апстрим ---

func TestHybridLocal_PassthroughVerbatim(t *testing.T) {
	// In hybrid mode, requests that name the local model are forwarded to
	// the local llama-server through the SAME verbatim passthrough as
	// local-only mode: llama-server serves the Anthropic Messages API
	// natively on /v1/messages, so no conversion is needed. This test sends
	// a tool_use/tool_result conversation and asserts the upstream sees it
	// byte-for-byte, and the Anthropic-format response comes back verbatim.
	anthropicBody := `{"model":"qwen3-coder","max_tokens":10,"messages":[` +
		`{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"Bash","input":{"command":"ls"}}]},` +
		`{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","content":` +
		`[{"type":"text","text":"file1"},{"type":"text","text":"file2"}]}]}]}`
	wantResp := `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}]}`

	upstream, captured := bodyCapturingUpstream(t, wantResp)
	h := &proxyHandler{
		llamaServerURL: upstream.URL,
		cfg: &config.Config{
			Alias:         "qwen3-coder",
			OpenAIBaseURL: "http://provider.invalid/v1",
			OpenAIModel:   "gpt-4",
			OpenAIKey:     "k",
		},
		client:      upstream.Client(),
		localClient: upstream.Client(),
	}
	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(anthropicBody)))
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", w.Code, w.Body.String())
	}
	if got := string(captured()); got != anthropicBody {
		t.Errorf("upstream body differs:\n got: %s\nwant: %s", got, anthropicBody)
	}
	if w.Body.String() != wantResp {
		t.Errorf("response = %s, want %s", w.Body.String(), wantResp)
	}
}

func TestHybridLocal_UpstreamErrorPassesThrough(t *testing.T) {
	// The local route in hybrid mode is a verbatim passthrough: an upstream
	// error is relayed with its status and body unchanged, exactly as if
	// Claude Code had talked to llama-server directly.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "context length exceeded", http.StatusBadRequest)
	}))
	defer upstream.Close()

	h := &proxyHandler{
		llamaServerURL: upstream.URL,
		cfg: &config.Config{
			Alias:         "qwen3-coder",
			OpenAIBaseURL: "http://provider.invalid/v1",
			OpenAIModel:   "gpt-4",
			OpenAIKey:     "k",
		},
		client:      upstream.Client(),
		localClient: upstream.Client(),
	}
	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"qwen3-coder","messages":[{"role":"user","content":"hi"}]}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "context length exceeded") {
		t.Errorf("error message lost: %s", w.Body.String())
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

// --- hybrid: маршрутизация по имени модели ---

func TestHybrid_LocalAliasRoutesToLlama(t *testing.T) {
	upstream, captured := bodyCapturingUpstream(t,
		`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}]}`)
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
		`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}]}`)
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
