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
	"time"
)

// errSwitch — причина отказа переключения в тестах; её текст обязан дойти до
// клиента в сообщении Anthropic-ошибки.
var errSwitch = errors.New("switch boom")

// fakeSwitcher — заглушка ModelSwitcher: помнит вызовы, умеет переписывать
// имя модели на «алиас загруженного сервера» или падать.
type fakeSwitcher struct {
	mu      sync.Mutex
	handles map[string]string // model → servedAlias
	names   []string
	err     error
	ensured int
}

func (f *fakeSwitcher) Handles(model string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.handles[model]
	return ok
}

func (f *fakeSwitcher) EnsureLoaded(_ context.Context, model string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensured++
	if f.err != nil {
		return "", f.err
	}
	if alias, ok := f.handles[model]; ok {
		return alias, nil
	}
	return model, nil
}

func (f *fakeSwitcher) ModelNames() []string { return f.names }

func (f *fakeSwitcher) ensuredCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ensured
}

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

// localSwitchedHandler — прокси local-only + switcher, как в реальном запуске
// с профилями.
func localSwitchedHandler(t *testing.T, upstreamURL string, sw ModelSwitcher) *proxyHandler {
	t.Helper()
	return &proxyHandler{
		llamaServerURL: upstreamURL,
		cfg:            localCfg(),
		client:         http.DefaultClient,
		switcher:       sw,
	}
}

func TestLocalRoute_SwitchRewritesModelBeforeForwarding(t *testing.T) {
	upstream, captured := bodyCapturingUpstream(t, `{"id":"m1","type":"message"}`)
	sw := &fakeSwitcher{handles: map[string]string{"coder": "qwen-coder-v2"}}
	h := localSwitchedHandler(t, upstream.URL, sw)

	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(
		`{"model":"coder","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`)))

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", w.Code, w.Body.String())
	}
	var forwarded map[string]any
	body := captured()
	if err := json.Unmarshal(body, &forwarded); err != nil {
		t.Fatalf("апстрим получил не JSON: %s", string(body))
	}
	if forwarded["model"] != "qwen-coder-v2" {
		t.Errorf("model на апстриме = %v, want qwen-coder-v2", forwarded["model"])
	}
	if forwarded["max_tokens"] != float64(8) {
		t.Errorf("остальное тело исказилось: %v", forwarded)
	}
	if sw.ensuredCalls() != 1 {
		t.Errorf("EnsureLoaded вызван %d раз(а), want 1", sw.ensured)
	}
}

func TestLocalRoute_SwitchFailureIsAnthropicError(t *testing.T) {
	upstream, _ := bodyCapturingUpstream(t, `{"unexpected":true}`)
	sw := &fakeSwitcher{
		handles: map[string]string{"coder": "qwen-coder-v2"},
		err:     errSwitch,
	}
	h := localSwitchedHandler(t, upstream.URL, sw)

	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"coder","messages":[]}`)))

	if w.Code != http.StatusBadGateway {
		t.Fatalf("code = %d, want 502", w.Code)
	}
	var got struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("тело не Anthropic-ошибка: %s", w.Body.String())
	}
	if got.Type != "error" || got.Error.Type != "api_error" {
		t.Errorf("тело = %+v", got)
	}
	if !strings.Contains(got.Error.Message, "switch boom") {
		t.Errorf("причина переключения потеряна: %q", got.Error.Message)
	}
}

func TestLocalRoute_UnhandledModelForwardedWithoutSwitch(t *testing.T) {
	upstream, captured := bodyCapturingUpstream(t, `{}`)
	sw := &fakeSwitcher{handles: map[string]string{"coder": "qwen-coder-v2"}}
	h := localSwitchedHandler(t, upstream.URL, sw)

	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"unknown-model","messages":[]}`)))

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d", w.Code)
	}
	if sw.ensuredCalls() != 0 {
		t.Errorf("EnsureLoaded вызван для чужой модели %d раз(а)", sw.ensured)
	}
	if !strings.Contains(string(captured()), "unknown-model") {
		t.Errorf("имя не-профиля не должно переписываться: %s", string(captured()))
	}
}

func TestLocalRoute_NonJSONBodyPassesThrough(t *testing.T) {
	upstream, captured := bodyCapturingUpstream(t, `{}`)
	sw := &fakeSwitcher{handles: map[string]string{"coder": "x"}}
	h := localSwitchedHandler(t, upstream.URL, sw)

	raw := `this is not json at all`
	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(raw)))

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d", w.Code)
	}
	if string(captured()) != raw {
		t.Errorf("тело исказилось: %q", string(captured()))
	}
}

func TestLocalRoute_ModelsAdvertisesSwitcherNames(t *testing.T) {
	sw := &fakeSwitcher{names: []string{"main", "coder", "fast"}}
	h := localSwitchedHandler(t, "http://llama.invalid", sw)

	w := httptest.NewRecorder()
	h.handleModels(w, httptest.NewRequest(http.MethodGet, "/v1/models", nil))

	var got struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Data) != 3 || got.Data[0].ID != "main" || got.Data[1].ID != "coder" || got.Data[2].ID != "fast" {
		t.Errorf("список моделей = %+v", got.Data)
	}
}

func TestLocalRoute_SingleModelLookupEchoed(t *testing.T) {
	sw := &fakeSwitcher{names: []string{"main"}}
	h := localSwitchedHandler(t, "http://llama.invalid", sw)

	w := httptest.NewRecorder()
	h.handleOther(w, httptest.NewRequest(http.MethodGet, "/v1/models/coder", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 echo для имени из меню", w.Code)
	}
}

func TestLocalRoute_TokenCountingGoesUpstream(t *testing.T) {
	upstream, _ := bodyCapturingUpstream(t, `{"input_tokens":7}`)
	sw := &fakeSwitcher{names: []string{"main"}}
	h := localSwitchedHandler(t, upstream.URL, sw)

	w := httptest.NewRecorder()
	h.handleOther(w, httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens",
		strings.NewReader(`{"model":"main","messages":[]}`)))

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "7") {
		t.Errorf("count_tokens не дошёл до llama-server: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestLocalRoute_SwitchedStreamIsFlushed(t *testing.T) {
	// После переключения SSE идёт инкрементально: flush на каждый чанк.
	chunks := []string{
		"event: message_start\ndata: {}\n\n",
		"event: content_block_delta\ndata: {}\n\n",
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i, c := range chunks {
			if i > 0 {
				time.Sleep(100 * time.Millisecond) // см. passthrough_test: разнос чанков
			}
			_, _ = w.Write([]byte(c))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer upstream.Close()

	sw := &fakeSwitcher{handles: map[string]string{"coder": "coder"}}
	h := localSwitchedHandler(t, upstream.URL, sw)

	rec := &flushRecorder{ResponseRecorder: *httptest.NewRecorder()}
	h.handleMessages(rec, httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"coder","messages":[]}`)))

	if rec.Body.String() != strings.Join(chunks, "") {
		t.Errorf("стрим собран неверно: %q", rec.Body.String())
	}
	if rec.flushes < len(chunks) {
		t.Errorf("flush вызван %d раз(а) при %d чанках", rec.flushes, len(chunks))
	}
}
