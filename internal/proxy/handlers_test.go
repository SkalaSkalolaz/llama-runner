package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// --- hybrid: переключение локальной модели по имени профиля ---

// hybridSwitchedHandler — прокси hybrid + switcher, как в реальном запуске с
// внешним провайдером и профилями. Внешний URL остаётся нерабочим: эти
// тесты к нему не обращаются (все запросы идут локально).
func hybridSwitchedHandler(t *testing.T, upstreamURL string, sw ModelSwitcher) *proxyHandler {
	t.Helper()
	return &proxyHandler{
		llamaServerURL: upstreamURL,
		cfg:            hybridCfg(),
		client:         http.DefaultClient,
		switcher:       sw,
	}
}

func TestHybrid_ProfileSwitch_RewritesModelAndForwardsLocal(t *testing.T) {
	upstream, captured := bodyCapturingUpstream(t,
		`{"id":"c1","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
	sw := &fakeSwitcher{handles: map[string]string{"coder": "qwen-coder-v2"}}
	h := hybridSwitchedHandler(t, upstream.URL, sw)

	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(
		`{"model":"coder","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`)))

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", w.Code, w.Body.String())
	}
	if sw.ensuredCalls() != 1 {
		t.Errorf("EnsureLoaded вызван %d раз, want 1", sw.ensuredCalls())
	}
	var got map[string]any
	if err := json.Unmarshal(captured(), &got); err != nil {
		t.Fatalf("апстрим вернул не JSON: %v", err)
	}
	if got["model"] != "qwen-coder-v2" {
		t.Errorf("модель у апстрима = %v, want qwen-coder-v2", got["model"])
	}
}

func TestHybrid_ExternalNameDoesNotTouchSwitcher(t *testing.T) {
	sw := &fakeSwitcher{handles: map[string]string{"coder": "qwen-coder-v2"}}
	h := hybridSwitchedHandler(t, "http://provider.invalid", sw)

	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(
		`{"model":"m","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`)))

	if sw.ensuredCalls() != 0 {
		t.Errorf("имя внешнего провайдера не должно трогать switcher, вызовов: %d", sw.ensuredCalls())
	}
	// Внешний провайдер нерабочий — запрос обязан уйти вверх и упасть в 502.
	if w.Code != http.StatusBadGateway {
		t.Errorf("code = %d, want 502 (внешний маршрут)", w.Code)
	}
}

func TestHybrid_LocalTokenSkipsSwitcher(t *testing.T) {
	upstream, captured := bodyCapturingUpstream(t,
		`{"id":"c1","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
	sw := &fakeSwitcher{handles: map[string]string{"coder": "qwen-coder-v2"}}
	h := hybridSwitchedHandler(t, upstream.URL, sw)

	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(
		`{"model":"llama-server","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`)))

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", w.Code, w.Body.String())
	}
	if sw.ensuredCalls() != 0 {
		t.Errorf("токен llama-server не должен перезагружать сервер, вызовов: %d", sw.ensuredCalls())
	}
	var got map[string]any
	if err := json.Unmarshal(captured(), &got); err != nil {
		t.Fatalf("апстрим вернул не JSON: %v", err)
	}
	if got["model"] != "llama-server" {
		t.Errorf("модель у апстрима = %v, want llama-server", got["model"])
	}
}

func TestHybrid_SwitchFailureIsAnthropicError(t *testing.T) {
	sw := &fakeSwitcher{handles: map[string]string{"coder": "x"}, err: errSwitch}
	h := hybridSwitchedHandler(t, "http://llama.invalid", sw)

	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(
		`{"model":"coder","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`)))

	if w.Code != http.StatusBadGateway {
		t.Fatalf("code = %d, want 502", w.Code)
	}
	if !strings.Contains(w.Body.String(), "switch boom") {
		t.Errorf("причина переключения не дошла до клиента: %s", w.Body.String())
	}
}

func TestHybrid_ModelsListsSwitcherNamesWithoutDuplicates(t *testing.T) {
	sw := &fakeSwitcher{names: []string{"m", "coder", "writer"}}
	h := hybridSwitchedHandler(t, "http://llama.invalid", sw)

	w := httptest.NewRecorder()
	h.handleModels(w, httptest.NewRequest(http.MethodGet, "/v1/models", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d", w.Code)
	}
	var got struct {
		Data    []modelEntry `json:"data"`
		FirstID string       `json:"first_id"`
		LastID  string       `json:"last_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("не JSON: %v (%s)", err, w.Body.String())
	}
	ids := map[string]int{}
	for _, m := range got.Data {
		ids[strings.ToLower(m.ID)]++
	}
	if ids["coder"] != 1 || ids["writer"] != 1 {
		t.Errorf("имена switcher в списке: coder=%d writer=%d, want по одному", ids["coder"], ids["writer"])
	}
	if ids["m"] != 1 {
		t.Errorf("пересечение с внешней моделью должно дедуплицироваться, получили %d", ids["m"])
	}
	if got.FirstID != got.Data[0].ID || got.LastID != got.Data[len(got.Data)-1].ID {
		t.Errorf("first/last_id не соответствуют списку: %q..%q", got.FirstID, got.LastID)
	}
}
