package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAnthropicToOpenAI_Basic(t *testing.T) {
	in := map[string]any{
		"model":  "local",
		"system": "you are helpful",
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
		},
		"max_tokens": 100,
	}
	out, err := AnthropicToOpenAI(in)
	if err != nil {
		t.Fatal(err)
	}
	if out["model"] != "local" {
		t.Errorf("model = %v", out["model"])
	}
	msgs := out["messages"].([]map[string]any)
	if len(msgs) != 2 || msgs[0]["role"] != "system" || msgs[1]["role"] != "user" {
		t.Errorf("messages = %v", msgs)
	}
}

func TestAnthropicToOpenAI_WithTools(t *testing.T) {
	in := map[string]any{
		"model": "local",
		"messages": []any{
			map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "text", "text": "calling tool"},
					map[string]any{
						"type": "tool_use", "id": "t1", "name": "read",
						"input": map[string]any{"path": "/x"},
					},
				},
			},
		},
		"tools": []any{
			map[string]any{
				"name": "read", "description": "read file",
				"input_schema": map[string]any{"type": "object"},
			},
		},
	}
	out, _ := AnthropicToOpenAI(in)

	openAITools := out["tools"].([]map[string]any)
	if len(openAITools) != 1 {
		t.Fatalf("tools = %v", openAITools)
	}
	fn := openAITools[0]["function"].(map[string]any)
	if fn["name"] != "read" || fn["parameters"] == nil {
		t.Errorf("function = %v", fn)
	}

	msgs := out["messages"].([]map[string]any)
	if len(msgs) != 1 || len(msgs[0]["tool_calls"].([]map[string]any)) != 1 {
		t.Errorf("tool_calls not converted: %v", msgs)
	}
}

func TestHandleModels_HasFirstLastID(t *testing.T) {
	h := &proxyHandler{cfg: &Config{OpenAIModel: "gpt-4"}}
	w := httptest.NewRecorder()
	h.handleModels(w, httptest.NewRequest(http.MethodGet, "/v1/models", nil))

	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["first_id"] != "llama-server" {
		t.Errorf("first_id = %v, want llama-server", got["first_id"])
	}
	if got["last_id"] != "opus" {
		t.Errorf("last_id = %v, want opus", got["last_id"])
	}
}

func TestHandleModels_AlwaysHasLocal(t *testing.T) {
	h := &proxyHandler{cfg: &Config{OpenAIModel: "gpt-4"}}
	w := httptest.NewRecorder()
	h.handleModels(w, httptest.NewRequest(http.MethodGet, "/v1/models", nil))

	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	data := got["data"].([]any)
	found := false
	for _, item := range data {
		if item.(map[string]any)["id"] == "llama-server" {
			found = true
			break
		}
	}
	if !found {
		t.Error("llama-server not found in /v1/models")
	}
}

func TestConvertOpenAIResponse_Text(t *testing.T) {
	body := `{
		"id": "resp1",
		"choices": [{"message": {"content": "hello"}, "finish_reason": "stop"}],
		"usage": {"prompt_tokens": 5, "completion_tokens": 2}
	}`
	resp := &http.Response{Body: newReadCloser(body)}
	w := httptest.NewRecorder()
	convertOpenAIResponseToAnthropic(w, resp, "local")

	if w.Code != 200 {
		t.Fatalf("code = %d", w.Code)
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["type"] != "message" || got["stop_reason"] != "end_turn" {
		t.Errorf("got = %v", got)
	}
	content := got["content"].([]any)
	if len(content) != 1 || content[0].(map[string]any)["text"] != "hello" {
		t.Errorf("content = %v", content)
	}
}

func TestBuildClaudeEnv_WithOpenAI(t *testing.T) {
	cfg := &Config{
		Alias:         "local",
		OpenAIBaseURL: "https://api.example.com/v1",
		OpenAIModel:   "gpt-4",
	}
	env := buildClaudeEnv(cfg, "http://proxy:8082", nil)
	m := map[string]string{}
	for _, kv := range env {
		p := strings.SplitN(kv, "=", 2)
		m[p[0]] = p[1]
	}
	// Все стандартные алиасы теперь ведут на внешнюю модель.
	if m["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "gpt-4" {
		t.Errorf("sonnet = %q, want gpt-4", m["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if m["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "gpt-4" {
		t.Errorf("opus = %q, want gpt-4", m["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
	if m["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "gpt-4" {
		t.Errorf("haiku = %q, want gpt-4", m["ANTHROPIC_DEFAULT_HAIKU_MODEL"])
	}
	// llama-server добавлен как кастомная опция.
	if m["ANTHROPIC_CUSTOM_MODEL_OPTION"] != "llama-server" {
		t.Errorf("custom option = %q, want llama-server", m["ANTHROPIC_CUSTOM_MODEL_OPTION"])
	}
}

func TestValidateConfig_OpenAIMissingKey(t *testing.T) {
	cfg := &Config{
		Host: "127.0.0.1", Port: "8080", McpAddr: "127.0.0.1:8090",
		ReadyTimeout:  1e9,
		OpenAIBaseURL: "https://api.example.com/v1",
		// OpenAIModel задана, но нет ключа
		OpenAIModel: "gpt-4",
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected error about missing -key")
	}
}

// хелпер
type readCloser struct{ *bytes.Buffer }

func (readCloser) Close() error          { return nil }
func newReadCloser(s string) *readCloser { return &readCloser{bytes.NewBufferString(s)} }

func TestNewProxyClient_Timeout(t *testing.T) {
	c := newProxyClient()
	if c.Timeout != 0 {
		t.Errorf("Timeout = %v, want 0 (иначе 5 минут режут длинную генерацию)", c.Timeout)
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport = %T, want *http.Transport", c.Transport)
	}
	if tr.ResponseHeaderTimeout != 30*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want 30s", tr.ResponseHeaderTimeout)
	}
}

func TestHandleMessages_TooLarge(t *testing.T) {
	// Огромное тело: 100 МБ + 1 байт.
	// Собираем не целиком в памяти, а через io.LimitReader от "бесконечного" читателя.
	h := &proxyHandler{cfg: &Config{}}
	w := httptest.NewRecorder()

	huge := io.LimitReader(infiniteReader(' '), maxRequestBodyBytes+1)
	r := httptest.NewRequest(http.MethodPost, "/v1/messages", huge)
	h.handleMessages(w, r)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("code = %d, want 413", w.Code)
	}
	var got struct {
		Type  string `json:"type"`
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if got.Type != "error" || got.Error.Type != "request_too_large" {
		t.Errorf("body = %+v", got)
	}
}

// infiniteReader отдаёт n байт c, не памятуя их.
type infiniteReader byte

func (r infiniteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(r)
	}
	return len(p), nil
}

// ---------- M4: аутентификация прокси ----------

func TestIsLoopbackHost(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"127.0.0.1:8082", true},
		{"127.0.0.1:0", true},
		{"[::1]:8082", true},
		{"localhost:8082", true},
		{"0.0.0.0:8082", false},
		{"[::]:8082", false},
		{"192.168.1.1:8082", false},
		{"example.com:8082", false},
		{"", false},
		{"bad", false},
	}
	for _, tt := range tests {
		if got := isLoopbackHost(tt.in); got != tt.want {
			t.Errorf("isLoopbackHost(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestRequestToken(t *testing.T) {
	tests := []struct {
		name   string
		header map[string]string
		want   string
	}{
		{"none", nil, ""},
		{"bearer", map[string]string{"Authorization": "Bearer abc"}, "abc"},
		{"x-api-key", map[string]string{"X-Api-Key": "abc"}, "abc"},
		{"bearer wins", map[string]string{"Authorization": "Bearer from-auth", "X-Api-Key": "from-api"}, "from-auth"},
		{"not bearer", map[string]string{"Authorization": "Basic abc"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tt.header {
				r.Header.Set(k, v)
			}
			if got := requestToken(r); got != tt.want {
				t.Errorf("requestToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShouldWarnProxyExposure(t *testing.T) {
	tests := []struct {
		name string
		addr string
		cfg  *Config
		want bool
	}{
		{"loopback", "127.0.0.1:8082", &Config{}, false},
		{"loopback with token", "127.0.0.1:8082", &Config{ProxyToken: "x"}, false},
		{"all ifaces no auth", "0.0.0.0:8082", &Config{}, true},
		{"all ifaces with token", "0.0.0.0:8082", &Config{ProxyToken: "x"}, false},
		{"all ifaces allow-remote", "0.0.0.0:8082", &Config{ProxyAllowRemote: true}, false},
		{"all ifaces both", "0.0.0.0:8082", &Config{ProxyToken: "x", ProxyAllowRemote: true}, false},
		{"nil cfg", "0.0.0.0:8082", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldWarnProxyExposure(tt.addr, tt.cfg); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProxyAuthMiddleware_NoToken(t *testing.T) {
	h := &proxyHandler{cfg: &Config{}}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	srv := httptest.NewServer(h.authMiddleware(next))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("code = %d, want 200", resp.StatusCode)
	}
	if !called {
		t.Error("next handler not called when token not configured")
	}
}

func TestProxyAuthMiddleware_RequiresToken(t *testing.T) {
	h := &proxyHandler{cfg: &Config{ProxyToken: "secret"}}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	srv := httptest.NewServer(h.authMiddleware(next))
	defer srv.Close()

	// Без заголовков → 401.
	resp, _ := http.Get(srv.URL)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no-token: code = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	// Bearer → 200.
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("bearer: code = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// x-api-key → 200.
	req, _ = http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("x-api-key", "secret")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("x-api-key: code = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Неверный токен → 401.
	req, _ = http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("Authorization", "Bearer wrong")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong: code = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestProxy_AuthRequired_EndToEnd(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer upstream.Close()

	h := &proxyHandler{
		llamaServerURL: upstream.URL,
		cfg:            &Config{OpenAIModel: "m", ProxyToken: "secret"},
		client:         upstream.Client(),
	}
	srv := httptest.NewServer(h.authMiddleware(newProxyMux(h)))
	defer srv.Close()

	body := `{"model":"llama-server","messages":[{"role":"user","content":"hi"}]}`

	// Без токена → 401, upstream не вызван.
	resp, _ := http.Post(srv.URL+"/v1/messages", "application/json", strings.NewReader(body))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no-token: %d, want 401", resp.StatusCode)
	}
	if upstreamCalled {
		t.Error("upstream called without auth")
	}
	resp.Body.Close()

	// С токеном → 200.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("with-token: %d, want 200", resp.StatusCode)
	}
	if !upstreamCalled {
		t.Error("upstream not called with valid token")
	}
	resp.Body.Close()
}
