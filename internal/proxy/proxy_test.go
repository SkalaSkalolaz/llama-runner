package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

func TestHandleModels_HasFirstLastID(t *testing.T) {
	h := &proxyHandler{cfg: &config.Config{OpenAIModel: "gpt-4"}}
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
	h := &proxyHandler{cfg: &config.Config{OpenAIModel: "gpt-4"}}
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
	h := &proxyHandler{cfg: &config.Config{}}
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
		cfg  *config.Config
		want bool
	}{
		{"loopback", "127.0.0.1:8082", &config.Config{}, false},
		{"loopback with token", "127.0.0.1:8082", &config.Config{ProxyToken: "x"}, false},
		{"all ifaces no auth", "0.0.0.0:8082", &config.Config{}, true},
		{"all ifaces with token", "0.0.0.0:8082", &config.Config{ProxyToken: "x"}, false},
		{"all ifaces allow-remote", "0.0.0.0:8082", &config.Config{ProxyAllowRemote: true}, false},
		{"all ifaces both", "0.0.0.0:8082", &config.Config{ProxyToken: "x", ProxyAllowRemote: true}, false},
		{"nil cfg", "0.0.0.0:8082", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldWarnProxyExposure(tt.addr, tt.cfg); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProxyAuthMiddleware_NoToken(t *testing.T) {
	h := &proxyHandler{cfg: &config.Config{}}
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
	h := &proxyHandler{cfg: &config.Config{ProxyToken: "secret"}}
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
		cfg:            &config.Config{OpenAIModel: "m", ProxyToken: "secret"},
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
