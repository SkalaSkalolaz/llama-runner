package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
