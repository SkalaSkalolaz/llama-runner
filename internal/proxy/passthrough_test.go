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

// localCfg — конфигурация прокси без внешнего провайдера: local-only режим,
// в котором всё проксируется на llama-server без изменений.
func localCfg() *config.Config { return &config.Config{} }

// upstreamEcho records the requests it receives and answers with a canned
// JSON body, so a test can assert exactly what the passthrough put on the
// wire.
func upstreamEcho(t *testing.T, status int, body string) (*httptest.Server, <-chan capturedRequest) {
	t.Helper()
	captured := make(chan capturedRequest, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var parsed map[string]any
		_ = json.Unmarshal(raw, &parsed)
		captured <- capturedRequest{
			authSet: r.Header.Get("Authorization") != "",
			auth:    r.Header.Get("Authorization"),
			body:    parsed,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, captured
}

func TestExternalMode_FalseWithoutProvider(t *testing.T) {
	h := &proxyHandler{cfg: localCfg()}
	if h.externalMode() {
		t.Fatal("externalMode() = true без внешнего провайдера")
	}
	if !(hybridCfg()).OpenAIProviderConfigured() {
		t.Fatal("hybridCfg должен быть полностью настроен")
	}
}

func TestPassthrough_MessagesReachLlamaUnchanged(t *testing.T) {
	// В local-only режиме тело /v1/messages доходит до llama-server байт в
	// байт: конвертер не участвует, Anthropic-формат сохраняется.
	upstream, captured := upstreamEcho(t, http.StatusOK, `{"id":"m1","type":"message"}`)
	h := &proxyHandler{llamaServerURL: upstream.URL, cfg: localCfg(), client: upstream.Client()}

	body := `{"model":"qwen-coder","max_tokens":8,"messages":[{"role":"user","content":"2+2?"}]}`
	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body)))

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", w.Code, w.Body.String())
	}
	if w.Body.String() != `{"id":"m1","type":"message"}` {
		t.Errorf("ответ не проходит как есть: %s", w.Body.String())
	}
	got := <-captured
	if got.body["model"] != "qwen-coder" {
		t.Errorf("model на апстриме = %v, want qwen-coder", got.body["model"])
	}
	if _, ok := got.body["messages"]; !ok {
		t.Errorf("messages потеряны: %v", got.body)
	}
}

func TestPassthrough_ModelsServedByLlama(t *testing.T) {
	// GET /v1/models в local-only режиме обслуживает сам llama-server:
	// список моделейClaude Code видит ровно тот же, что и при прямом соединении.
	list := `{"data":[{"id":"qwen-coder","object":"model"}],"object":"list"}`
	upstream, _ := upstreamEcho(t, http.StatusOK, list)
	h := &proxyHandler{llamaServerURL: upstream.URL, cfg: localCfg(), client: upstream.Client()}

	w := httptest.NewRecorder()
	h.handleModels(w, httptest.NewRequest(http.MethodGet, "/v1/models", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d", w.Code)
	}
	if w.Body.String() != list {
		t.Errorf("body = %s, want %s", w.Body.String(), list)
	}
}

func TestPassthrough_UpstreamErrorPassesThrough(t *testing.T) {
	// Ошибка llama-server передаётся как есть (статус и текст), а не
	// переписывается в Anthropic-ошибку: ровно то жеClaude Code получала и
	// напрямую.
	upstream, _ := upstreamEcho(t, http.StatusBadRequest, `{"error":{"message":"too long"}}`)
	h := &proxyHandler{llamaServerURL: upstream.URL, cfg: localCfg(), client: upstream.Client()}

	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"m","messages":[]}`)))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "too long") {
		t.Errorf("тело ошибки потеряно: %s", w.Body.String())
	}
}

func TestPassthrough_OtherRouteGoesUpstream(t *testing.T) {
	// count_tokens и прочие маршруты в local-only режиме тоже идут на
	// llama-server, а не на эмуляцию hybrid-прокси.
	upstream, _ := upstreamEcho(t, http.StatusOK, `{"input_tokens":42}`)
	h := &proxyHandler{llamaServerURL: upstream.URL, cfg: localCfg(), client: upstream.Client()}

	w := httptest.NewRecorder()
	h.handleOther(w, httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens",
		strings.NewReader(`{"model":"m","messages":[]}`)))

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "42") {
		t.Errorf("ответ апстрима потерян: %s", w.Body.String())
	}
}

func TestPassthrough_DeadUpstreamIs502(t *testing.T) {
	// Мёртвый апстрим: Anthropic-ошибка 502, а не паника и не 200.
	h := &proxyHandler{llamaServerURL: "http://127.0.0.1:1", cfg: localCfg(), client: http.DefaultClient}
	w := httptest.NewRecorder()
	h.passthrough(w, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{}`)))

	if w.Code != http.StatusBadGateway {
		t.Fatalf("code = %d, want 502", w.Code)
	}
}

func TestPassthrough_AuthStillApplies(t *testing.T) {
	// Токен прокси проверяется и в local-only режиме: passthrough не
	// обходит authMiddleware.
	upstream, captured := upstreamEcho(t, http.StatusOK, `{}`)
	h := &proxyHandler{llamaServerURL: upstream.URL, cfg: &config.Config{ProxyToken: "secret"}, client: upstream.Client()}
	srv := httptest.NewServer(h.authMiddleware(newProxyMux(h)))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/messages", "application/json", strings.NewReader(`{"model":"m"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", resp.StatusCode)
	}
	select {
	case c := <-captured:
		t.Fatalf("апстрим вызван без токена: %v", c.body)
	default:
	}
}

// flushRecorder records whether Flush was called, standing in for the
// http.Flusher a live SSE consumer needs.
type flushRecorder struct {
	httptest.ResponseRecorder
	flushes int
}

func (f *flushRecorder) Flush() { f.flushes++ }

func TestPassthrough_FlushesStreamChunks(t *testing.T) {
	// SSE-ответ форсируется flush'ем на каждый чанк: стрим доходит
	// инкрементально, а не одним куском в конце.
	chunks := []string{
		"event: message_start\ndata: {\"type\":\"message_start\"}\n\n",
		"event: content_block_delta\ndata: {\"delta\":\"ok\"}\n\n",
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for i, c := range chunks {
			if i > 0 {
				// Разнос чанков по времени: иначе loopback успевает склеить
				// их в одно чтение прокси, и число flush'ей перестаёт быть
				// проверкой инкрементальности.
				time.Sleep(100 * time.Millisecond)
			}
			_, _ = io.WriteString(w, c)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer upstream.Close()

	h := &proxyHandler{llamaServerURL: upstream.URL, cfg: localCfg(), client: upstream.Client()}
	rec := &flushRecorder{ResponseRecorder: *httptest.NewRecorder()}
	h.passthrough(rec, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{}`)))

	if got := rec.Body.String(); got != strings.Join(chunks, "") {
		t.Errorf("стрим собран неверно:\n%q", got)
	}
	if rec.flushes < len(chunks) {
		t.Errorf("flush вызван %d раз(а) при %d чанках", rec.flushes, len(chunks))
	}
}

func TestPassthrough_PreservesMethodPathQuery(t *testing.T) {
	var gotMethod, gotPath, gotQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	h := &proxyHandler{llamaServerURL: upstream.URL, cfg: localCfg(), client: upstream.Client()}
	w := httptest.NewRecorder()
	h.passthrough(w, httptest.NewRequest(http.MethodGet, "/v1/models/qwen?detail=1", nil))

	if gotMethod != http.MethodGet || gotPath != "/v1/models/qwen" || gotQuery != "detail=1" {
		t.Errorf("method=%s path=%s query=%s", gotMethod, gotPath, gotQuery)
	}
}

func TestPassthrough_LocalModeEndToEndThroughMux(t *testing.T) {
	// Весь маршрут целиком: authMiddleware + mux + passthrough — как в
	// реальном запуске, через net/http, а не прямые вызовы обработчика.
	payload := `{"id":"gen-1","choices":[{"index":0,"message":{"role":"assistant","content":"привет"},"finish_reason":"stop"}]}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer upstream.Close()

	h := &proxyHandler{llamaServerURL: upstream.URL, cfg: localCfg(), client: upstream.Client()}
	front := httptest.NewServer(h.authMiddleware(newProxyMux(h)))
	defer front.Close()

	resp, err := http.Post(front.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"model":"m","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := new(bytes.Buffer)
	_, _ = io.Copy(body, resp.Body)

	if resp.StatusCode != http.StatusOK || body.String() != payload {
		t.Errorf("code = %d, body = %s, want payload verbatim", resp.StatusCode, body.String())
	}
}
