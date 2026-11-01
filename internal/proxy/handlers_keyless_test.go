package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

// capturedRequest is what the stand-in provider saw on the wire. authSet
// records the PRESENCE of the Authorization header, which is a fact distinct
// from its value: "no header" and "empty header" are different requests, and
// the keyless contract depends on the first one.
type capturedRequest struct {
	authSet bool
	auth    string
	body    map[string]any
}

// upstreamCapture starts a stand-in OpenAI-compatible provider that records
// the one request it receives and answers with a minimal chat completion. The
// capture is handed over through a channel, so the test goroutine reads it
// without racing the server goroutine.
func upstreamCapture(t *testing.T) (string, <-chan capturedRequest) {
	t.Helper()
	captured := make(chan capturedRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vals := r.Header.Values("Authorization")
		got := capturedRequest{authSet: len(vals) > 0, body: map[string]any{}}
		if got.authSet {
			got.auth = vals[0]
		}
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		captured <- got
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL, captured
}

// anthropicUserRequest is the smallest /v1/messages body that routes to the
// named model.
func anthropicUserRequest(model string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(
		`{"model":"`+model+`","max_tokens":8,"messages":[{"role":"user","content":"2+2?"}]}`))
}

func TestProxy_OpenAIUpstreamSendsBearerKey(t *testing.T) {
	url, captured := upstreamCapture(t)
	h := &proxyHandler{
		llamaServerURL: "http://llama.invalid",
		cfg: &config.Config{
			OpenAIBaseURL: url + "/v1",
			OpenAIModel:   "ext/model-x",
			OpenAIKey:     "sk-secret",
		},
		client: http.DefaultClient,
	}
	w := httptest.NewRecorder()
	h.handleMessages(w, anthropicUserRequest("ext/model-x"))

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", w.Code, w.Body.String())
	}
	got := <-captured
	if !got.authSet {
		t.Fatal("Authorization header missing, want Bearer sk-secret")
	}
	if got.auth != "Bearer sk-secret" {
		t.Errorf("Authorization = %q, want \"Bearer sk-secret\"", got.auth)
	}
}

func TestProxy_KeylessUpstreamSendsNoAuthorizationHeader(t *testing.T) {
	// The wire format of -key none: an unauthenticated request, matching
	// `curl -H 'Content-Type: application/json' <base>/chat/completions`. An
	// empty header or "Bearer " would be a different request — some providers
	// answer 401 to it — so what is asserted is the absence itself, while the
	// rest of the request stays exactly as in the authenticated case.
	url, captured := upstreamCapture(t)
	h := &proxyHandler{
		llamaServerURL: "http://llama.invalid",
		cfg: &config.Config{
			OpenAIBaseURL: url + "/v1",
			OpenAIModel:   "Qwen3.6-27B",
			OpenAIKeyless: true,
		},
		client: http.DefaultClient,
	}
	w := httptest.NewRecorder()
	h.handleMessages(w, anthropicUserRequest("Qwen3.6-27B"))

	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", w.Code, w.Body.String())
	}
	got := <-captured
	if got.authSet {
		t.Errorf("Authorization = %q present, want no header at all for a keyless provider", got.auth)
	}
	if got.body["model"] != "Qwen3.6-27B" {
		t.Errorf("upstream model = %v, want Qwen3.6-27B", got.body["model"])
	}
	if _, ok := got.body["messages"]; !ok {
		t.Errorf("upstream body has no messages: %v", got.body)
	}
}
