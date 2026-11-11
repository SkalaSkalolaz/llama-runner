package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

// Auto mode: a manual switch must neither restart the server nor fail the
// request — it is served by the model the orchestrator loaded.
func TestHandleMessages_AutoMode_SwitchIgnored(t *testing.T) {
	upstream, captured := bodyCapturingUpstream(t, `{"ok":true}`)
	sw := &fakeSwapper{current: "auto", aliases: []string{"auto", "coder"}, url: upstream.URL}
	h := &proxyHandler{
		llamaServerURL: upstream.URL,
		cfg:            &config.Config{Alias: "auto", Mode: "auto"},
		client:         upstream.Client(),
		localClient:    upstream.Client(),
		swapper:        sw,
	}
	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest("POST", "/v1/messages",
		strings.NewReader(`{"model":"switch-to:coder","messages":[]}`)))

	if len(sw.calls) != 0 {
		t.Errorf("SwapTo must not be called in auto mode: %v", sw.calls)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s (ignoring is not an error)", w.Code, w.Body.String())
	}
	if len(captured()) == 0 {
		t.Error("the running server was not reached")
	}
}

// Hybrid auto mode: the ignored switch must route LOCALLY, not be mistaken
// for an external model name and forwarded to the provider.
func TestHandleMessages_AutoMode_HybridSwitchStaysLocal(t *testing.T) {
	upstream, captured := bodyCapturingUpstream(t, `{"ok":true}`)
	sw := &fakeSwapper{current: "auto", aliases: []string{"auto", "coder"}, url: upstream.URL}
	h := &proxyHandler{
		llamaServerURL: upstream.URL,
		cfg: &config.Config{
			Alias: "auto", Mode: "auto",
			OpenAIBaseURL: "http://provider.invalid/v1",
			OpenAIModel:   "ext", OpenAIKey: "k",
		},
		client:      http.DefaultClient, // dead external route
		localClient: upstream.Client(),
		swapper:     sw,
	}
	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest("POST", "/v1/messages",
		strings.NewReader(`{"model":"switch-to:coder","messages":[]}`)))

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", w.Code, w.Body.String())
	}
	if len(captured()) == 0 {
		t.Error("request went to the external provider instead of llama-server")
	}
	if len(sw.calls) != 0 {
		t.Errorf("SwapTo called: %v", sw.calls)
	}
}

// Regression: without auto mode the very same request still swaps.
func TestHandleMessages_NonAutoMode_SwitchStillSwaps(t *testing.T) {
	upstream, _ := bodyCapturingUpstream(t, `{"ok":true}`)
	sw := &fakeSwapper{current: "auto", aliases: []string{"auto", "coder"}, url: upstream.URL}
	h := &proxyHandler{
		llamaServerURL: "http://old:9999",
		cfg:            &config.Config{Alias: "auto"},
		client:         upstream.Client(),
		localClient:    upstream.Client(),
		swapper:        sw,
	}
	w := httptest.NewRecorder()
	h.handleMessages(w, httptest.NewRequest("POST", "/v1/messages",
		strings.NewReader(`{"model":"switch-to:coder","messages":[]}`)))
	if len(sw.calls) != 1 || sw.calls[0] != "coder" {
		t.Errorf("calls = %v, want [coder]", sw.calls)
	}
}
