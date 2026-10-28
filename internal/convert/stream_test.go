package convert

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// ---------- хелперы ----------

// sseEvent — распарсенное SSE-событие из ответа прокси.
type sseEvent struct {
	Name string
	Data map[string]any
}

// parseSSE разбирает SSE-тело на события в порядке появления.
func parseSSE(t *testing.T, body string) []sseEvent {
	t.Helper()
	var events []sseEvent
	for _, block := range strings.Split(body, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		var (
			name    string
			dataStr string
		)
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				dataStr = strings.TrimPrefix(line, "data: ")
			}
		}
		if name == "" {
			continue
		}
		var data map[string]any
		if dataStr != "" {
			if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
				t.Fatalf("невалидный JSON в SSE-событии %q: %v (%s)", name, err, dataStr)
			}
		}
		events = append(events, sseEvent{Name: name, Data: data})
	}
	return events
}

// streamBody оборачивает payload'ы в SSE-тело OpenAI. "[DONE]" — как есть.
func streamBody(payloads ...string) io.ReadCloser {
	var b strings.Builder
	for _, p := range payloads {
		b.WriteString("data: ")
		b.WriteString(p)
		b.WriteString("\n\n")
	}
	return io.NopCloser(strings.NewReader(b.String()))
}

// brokenReader отдаёт data, затем err (имитация обрыва TCP).
type brokenReader struct {
	data []byte
	pos  int
	err  error
}

func (r *brokenReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (r *brokenReader) Close() error { return nil }

func eventNames(events []sseEvent) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Name
	}
	return out
}

func findEvents(events []sseEvent, name string) []sseEvent {
	var out []sseEvent
	for _, e := range events {
		if e.Name == name {
			out = append(out, e)
		}
	}
	return out
}

// ---------- тесты ----------

func TestConvertStream_TextOnly(t *testing.T) {
	resp := &http.Response{StatusCode: 200, Body: streamBody(
		`{"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`{"choices":[{"delta":{"content":" world"},"finish_reason":null}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2}}`,
		`[DONE]`,
	)}
	w := httptest.NewRecorder()

	in, out := ConvertOpenAIStreamToAnthropic(w, resp, "test-model")
	if in != 10 || out != 2 {
		t.Errorf("usage = (%d, %d), want (10, 2)", in, out)
	}

	events := parseSSE(t, w.Body.String())
	want := []string{
		"message_start",
		"content_block_start", "content_block_delta", "content_block_delta",
		"content_block_stop",
		"message_delta", "message_stop",
	}
	if got := eventNames(events); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %v\nwant %v", got, want)
	}

	deltas := findEvents(events, "content_block_delta")
	if len(deltas) != 2 {
		t.Fatalf("delta count = %d", len(deltas))
	}
	got := ""
	for _, d := range deltas {
		got += d.Data["delta"].(map[string]any)["text"].(string)
	}
	if got != "Hello world" {
		t.Errorf("text = %q", got)
	}

	md := findEvents(events, "message_delta")[0]
	if md.Data["delta"].(map[string]any)["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason = %v", md.Data["delta"])
	}
}

func TestConvertStream_TextWithToolCall(t *testing.T) {
	resp := &http.Response{StatusCode: 200, Body: streamBody(
		`{"choices":[{"delta":{"content":"Checking."},"finish_reason":null}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Bash","arguments":""}}]},"finish_reason":null}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":\"ls\"}"}}]},"finish_reason":null}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`[DONE]`,
	)}
	w := httptest.NewRecorder()
	ConvertOpenAIStreamToAnthropic(w, resp, "test-model")

	events := parseSSE(t, w.Body.String())
	want := []string{
		"message_start",
		"content_block_start", "content_block_delta", "content_block_stop",
		"content_block_start", "content_block_delta", "content_block_stop",
		"message_delta", "message_stop",
	}
	if got := eventNames(events); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %v\nwant %v", got, want)
	}

	starts := findEvents(events, "content_block_start")
	if len(starts) != 2 {
		t.Fatalf("starts = %d", len(starts))
	}
	tool := starts[1].Data["content_block"].(map[string]any)
	if tool["type"] != "tool_use" || tool["name"] != "Bash" || tool["id"] != "call_1" {
		t.Errorf("tool block = %v", tool)
	}

	md := findEvents(events, "message_delta")[0]
	if md.Data["delta"].(map[string]any)["stop_reason"] != "tool_use" {
		t.Errorf("stop_reason = %v", md.Data["delta"])
	}
}

func TestConvertStream_MultipleToolCalls(t *testing.T) {
	resp := &http.Response{StatusCode: 200, Body: streamBody(
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"Bash","arguments":""}}]},"finish_reason":null}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"Read","arguments":""}}]},"finish_reason":null}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":\"ls\"}"}}]},"finish_reason":null}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"file\":\"a\"}"}}]},"finish_reason":null}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`[DONE]`,
	)}
	w := httptest.NewRecorder()
	ConvertOpenAIStreamToAnthropic(w, resp, "test-model")

	events := parseSSE(t, w.Body.String())
	starts := findEvents(events, "content_block_start")
	if len(starts) != 2 {
		t.Fatalf("starts = %d, want 2", len(starts))
	}
	if starts[0].Data["content_block"].(map[string]any)["name"] != "Bash" {
		t.Errorf("starts[0].name = %v", starts[0].Data["content_block"])
	}
	if starts[1].Data["content_block"].(map[string]any)["name"] != "Read" {
		t.Errorf("starts[1].name = %v", starts[1].Data["content_block"])
	}

	stops := findEvents(events, "content_block_stop")
	if len(stops) != 2 {
		t.Fatalf("stops = %d, want 2", len(stops))
	}
	if stops[0].Data["index"].(float64) == stops[1].Data["index"].(float64) {
		t.Errorf("indices must differ: %v vs %v", stops[0].Data["index"], stops[1].Data["index"])
	}
}

func TestConvertStream_FinishReasonLength(t *testing.T) {
	resp := &http.Response{StatusCode: 200, Body: streamBody(
		`{"choices":[{"delta":{"content":"cut"},"finish_reason":null}]}`,
		`{"choices":[{"delta":{},"finish_reason":"length"}]}`,
		`[DONE]`,
	)}
	w := httptest.NewRecorder()
	ConvertOpenAIStreamToAnthropic(w, resp, "test-model")

	md := findEvents(parseSSE(t, w.Body.String()), "message_delta")[0]
	if md.Data["delta"].(map[string]any)["stop_reason"] != "max_tokens" {
		t.Errorf("stop_reason = %v", md.Data["delta"])
	}
}

func TestConvertStream_Interrupted(t *testing.T) {
	raw := "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n"
	resp := &http.Response{StatusCode: 200, Body: &brokenReader{
		data: []byte(raw),
		err:  errors.New("connection reset by peer"),
	}}
	w := httptest.NewRecorder()
	ConvertOpenAIStreamToAnthropic(w, resp, "test-model")

	events := parseSSE(t, w.Body.String())
	names := eventNames(events)
	for _, n := range names {
		if n == "message_stop" {
			t.Fatalf("message_stop при обрыве: %v", names)
		}
	}
	errEvents := findEvents(events, "error")
	if len(errEvents) != 1 {
		t.Fatalf("error events = %d, want 1; events = %v", len(errEvents), names)
	}
	if errEvents[0].Data["type"] != "error" {
		t.Errorf("error payload = %v", errEvents[0].Data)
	}
	msg := errEvents[0].Data["error"].(map[string]any)["message"].(string)
	if !strings.Contains(msg, "connection reset") {
		t.Errorf("error message = %q", msg)
	}
}

func TestConvertStream_CleanEOFWithoutDone(t *testing.T) {
	resp := &http.Response{StatusCode: 200, Body: streamBody(
		`{"choices":[{"delta":{"content":"half"},"finish_reason":null}]}`,
	)}
	w := httptest.NewRecorder()
	ConvertOpenAIStreamToAnthropic(w, resp, "test-model")

	events := parseSSE(t, w.Body.String())
	if len(findEvents(events, "message_stop")) != 0 {
		t.Errorf("message_stop без [DONE]: %v", eventNames(events))
	}
	if len(findEvents(events, "error")) != 1 {
		t.Errorf("ожидался event: error, получено %v", eventNames(events))
	}
}

func TestConvertStream_UsageInMessageDelta(t *testing.T) {
	resp := &http.Response{StatusCode: 200, Body: streamBody(
		`{"choices":[{"delta":{"content":"hi"},"finish_reason":null}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":25,"completion_tokens":3}}`,
		`[DONE]`,
	)}
	w := httptest.NewRecorder()
	in, out := ConvertOpenAIStreamToAnthropic(w, resp, "test-model")
	if in != 25 || out != 3 {
		t.Errorf("usage = (%d, %d), want (25, 3)", in, out)
	}

	md := findEvents(parseSSE(t, w.Body.String()), "message_delta")[0]
	usage := md.Data["usage"].(map[string]any)
	if usage["input_tokens"].(float64) != 25 || usage["output_tokens"].(float64) != 3 {
		t.Errorf("message_delta usage = %v", usage)
	}
}
