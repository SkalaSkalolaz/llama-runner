package convert

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Тесты вехи M1 (PLAN.md): конвертер запроса Anthropic → OpenAI.
// Каждый тест описывает ОЖИДАЕМОЕ поведение, а не текущее: тесты писались до
// переписывания конвертера и должны были падать на версии из M0.

// msgs — сообщения результата в удобном виде.
func msgs(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	m, ok := out["messages"].([]map[string]any)
	if !ok {
		t.Fatalf("messages имеет неожиданный тип %T", out["messages"])
	}
	return m
}

// --- system ---

func TestConvertRequest_SystemString(t *testing.T) {
	out, _, err := ConvertRequest(map[string]any{
		"model":    "local",
		"system":   "you are helpful",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	}, ConvertOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := msgs(t, out); len(got) != 2 || got[0]["role"] != "system" || got[0]["content"] != "you are helpful" {
		t.Errorf("messages = %v", got)
	}
}

func TestConvertRequest_SystemBlocks(t *testing.T) {
	// Дефект №2 (STATE.md): system массивом блоков игнорировался целиком.
	in := map[string]any{
		"model": "local",
		"system": []any{
			map[string]any{"type": "text", "text": "You are Claude Code."},
			map[string]any{"type": "text", "text": "Be concise.", "cache_control": map[string]any{"type": "ephemeral"}},
		},
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	}
	out, _, err := ConvertRequest(in, ConvertOptions{})
	if err != nil {
		t.Fatal(err)
	}
	m := msgs(t, out)
	if len(m) != 2 || m[0]["role"] != "system" {
		t.Fatalf("messages = %v", m)
	}
	sys, ok := m[0]["content"].(string)
	if !ok {
		t.Fatalf("system content = %T, want string", m[0]["content"])
	}
	if !strings.Contains(sys, "You are Claude Code.") || !strings.Contains(sys, "Be concise.") {
		t.Errorf("system потерял блоки: %q", sys)
	}
	if strings.Contains(sys, "ephemeral") {
		t.Errorf("cache_control просочился в текст: %q", sys)
	}
}

// --- tool_result ---

func TestConvertRequest_ToolResultString(t *testing.T) {
	out := convertToolResult(t, "571 proxy.go", false, ConvertOptions{})
	m := msgs(t, out)
	tool := lastToolMsg(t, m)
	if tool["content"] != "571 proxy.go" {
		t.Errorf("content = %v", tool["content"])
	}
	if tool["tool_call_id"] != "tu1" {
		t.Errorf("tool_call_id = %v", tool["tool_call_id"])
	}
}

func TestConvertRequest_ToolResultBlocks(t *testing.T) {
	// Дефект №1 (STATE.md): tool_result с массивом content терялся целиком.
	out := convertToolResult(t, []any{
		map[string]any{"type": "text", "text": "# PLAN.md"},
		map[string]any{"type": "text", "text": "Всего вех: 8."},
	}, false, ConvertOptions{})
	tool := lastToolMsg(t, msgs(t, out))
	got, ok := tool["content"].(string)
	if !ok {
		t.Fatalf("content = %T, want string", tool["content"])
	}
	if !strings.Contains(got, "# PLAN.md") || !strings.Contains(got, "Всего вех: 8.") {
		t.Errorf("content = %q", got)
	}
}

func TestConvertRequest_ToolResultIsError(t *testing.T) {
	// Дефект №3 (STATE.md): is_error терялся, модель читала ошибку как успех.
	out := convertToolResult(t, "No such file or directory", true, ConvertOptions{})
	tool := lastToolMsg(t, msgs(t, out))
	got, ok := tool["content"].(string)
	if !ok {
		t.Fatalf("content = %T, want string", tool["content"])
	}
	if !strings.HasPrefix(got, "Error: ") {
		t.Errorf("content = %q, want префикс %q", got, "Error: ")
	}
}

func TestConvertRequest_ToolResultErrorBlocks(t *testing.T) {
	out := convertToolResult(t, []any{
		map[string]any{"type": "text", "text": "exit status 1"},
		map[string]any{"type": "text", "text": "bash: nope: command not found"},
	}, true, ConvertOptions{})
	tool := lastToolMsg(t, msgs(t, out))
	got, ok := tool["content"].(string)
	if !ok {
		t.Fatalf("content = %T, want string", tool["content"])
	}
	if !strings.HasPrefix(got, "Error: ") {
		t.Errorf("content = %q, want префикс %q", got, "Error: ")
	}
	if !strings.Contains(got, "command not found") {
		t.Errorf("content потерял текст ошибки: %q", got)
	}
}

// --- thinking / image ---

func TestConvertRequest_ThinkingDroppedButCounted(t *testing.T) {
	// Дефект №4 (STATE.md): thinking отбрасывался молча.
	in := map[string]any{
		"model": "local",
		"messages": []any{
			map[string]any{"role": "user", "content": "why?"},
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "thinking", "thinking": "hmm", "signature": "abc"},
				map[string]any{"type": "text", "text": "because"},
			}},
		},
	}
	out, st, err := ConvertRequest(in, ConvertOptions{})
	if err != nil {
		t.Fatal(err)
	}
	m := msgs(t, out)
	if len(m) != 2 || m[1]["content"] != "because" {
		t.Errorf("messages = %v", m)
	}
	if st.DroppedThinking != 1 {
		t.Errorf("DroppedThinking = %d, want 1 (блок должен быть виден в -v)", st.DroppedThinking)
	}
	if strings.Contains(mustJSON(out), "\"hmm\"") {
		t.Errorf("thinking просочился в запрос: %s", mustJSON(out))
	}
}

func TestConvertRequest_ImageMappedWithMmproj(t *testing.T) {
	in := map[string]any{
		"model": "local",
		"messages": []any{
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": "что это?"},
				map[string]any{"type": "image", "source": map[string]any{
					"type": "base64", "media_type": "image/png", "data": "AAAA",
				}},
			}},
		},
	}
	out, st, err := ConvertRequest(in, ConvertOptions{ImagesToOpenAI: true})
	if err != nil {
		t.Fatal(err)
	}
	if st.DroppedImages != 0 {
		t.Errorf("DroppedImages = %d, want 0 при -mmproj", st.DroppedImages)
	}
	parts, ok := msgs(t, out)[0]["content"].([]any)
	if !ok {
		t.Fatalf("content = %T, want []any", msgs(t, out)[0]["content"])
	}
	if len(parts) != 2 {
		t.Fatalf("parts = %v", parts)
	}
	img, ok := parts[1].(map[string]any)
	if !ok || img["type"] != "image_url" {
		t.Fatalf("parts[1] = %v", parts[1])
	}
	iu, ok := img["image_url"].(map[string]any)
	if !ok || iu["url"] != "data:image/png;base64,AAAA" {
		t.Errorf("image_url = %v", img["image_url"])
	}
}

func TestConvertRequest_ImageDroppedCountedWithoutMmproj(t *testing.T) {
	in := map[string]any{
		"model": "local",
		"messages": []any{
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": "что это?"},
				map[string]any{"type": "image", "source": map[string]any{
					"type": "base64", "media_type": "image/png", "data": "AAAA",
				}},
			}},
		},
	}
	out, st, err := ConvertRequest(in, ConvertOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if st.DroppedImages != 1 {
		t.Errorf("DroppedImages = %d, want 1 (без -mmproj блок отбрасывается, но не молча)", st.DroppedImages)
	}
	if got := msgs(t, out)[0]["content"]; got != "что это?" {
		t.Errorf("content = %v, want строка без картинки", got)
	}
}

// --- порядок сборки messages ---

func TestConvertRequest_NormalFlowOrder(t *testing.T) {
	// Нормальный поток: assistant c tool_use → user c tool_result.
	// Порядок должен остаться прежним (инвариант M1: байт-в-байт как в M0).
	in := map[string]any{
		"model": "local",
		"messages": []any{
			map[string]any{"role": "user", "content": "wc"},
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "text", "text": "считаю"},
				map[string]any{"type": "tool_use", "id": "tu1", "name": "Bash",
					"input": map[string]any{"command": "wc -l x"}},
			}},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "tu1", "content": "10 x"},
			}},
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "text", "text": "10 строк"},
			}},
			map[string]any{"role": "user", "content": "спасибо"},
		},
	}
	out, _, err := ConvertRequest(in, ConvertOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"user", "assistant", "tool", "assistant", "user"}
	got := msgs(t, out)
	if len(got) != len(want) {
		t.Fatalf("roles = %s, want %s", roles(got), want)
	}
	for i, r := range want {
		if got[i]["role"] != r {
			t.Errorf("role[%d] = %v, want %s", i, got[i]["role"], r)
		}
	}
	if got[3]["content"] != "10 строк" {
		t.Errorf("content[3] = %v", got[3]["content"])
	}
}

func TestConvertRequest_MixedMessageAssistantFirst(t *testing.T) {
	// Невалидный по спецификации вход: tool_use и tool_result в одном сообщении.
	// Двухпроходная сборка обязана выдать assistant.tool_calls ДО role=tool,
	// иначе OpenAI-совместимый сервер не свяжет результат с вызовом.
	in := map[string]any{
		"model": "local",
		"messages": []any{
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "tu0", "content": "ранний результат"},
				map[string]any{"type": "tool_use", "id": "tu1", "name": "Bash", "input": map[string]any{}},
			}},
		},
	}
	out, _, err := ConvertRequest(in, ConvertOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := msgs(t, out)
	if len(got) != 2 {
		t.Fatalf("messages = %v", got)
	}
	if got[0]["role"] != "assistant" || got[1]["role"] != "tool" {
		t.Errorf("roles = %s, want [assistant tool]", roles(got))
	}
}

func TestConvertRequest_UserTextWithToolResultKeepsToolFirst(t *testing.T) {
	// user-сообщение с текстом и tool_result: role=tool должен идти сразу за
	// assistant.tool_calls, а текст — после (как в M0).
	in := map[string]any{
		"model": "local",
		"messages": []any{
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "tool_use", "id": "tu1", "name": "Bash", "input": map[string]any{}},
			}},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": "и ещё вопрос"},
				map[string]any{"type": "tool_result", "tool_use_id": "tu1", "content": "ok"},
			}},
		},
	}
	out, _, err := ConvertRequest(in, ConvertOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := msgs(t, out)
	if len(got) != 3 {
		t.Fatalf("messages = %v", got)
	}
	if r := roles(got); r[0] != "assistant" || r[1] != "tool" || r[2] != "user" {
		t.Errorf("roles = %s, want [assistant tool user]", r)
	}
}

// --- поля, которые уже копировались (покрываем тестами по требованию M1) ---

func TestConvertRequest_ArgumentsStaysJSONString(t *testing.T) {
	in := map[string]any{
		"model": "local",
		"messages": []any{
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "tool_use", "id": "tu1", "name": "Bash",
					"input": map[string]any{"command": "ls", "timeout": float64(30)}},
			}},
		},
	}
	out, _, err := ConvertRequest(in, ConvertOptions{})
	if err != nil {
		t.Fatal(err)
	}
	calls := msgs(t, out)[0]["tool_calls"].([]map[string]any)
	fn := calls[0]["function"].(map[string]any)
	args, ok := fn["arguments"].(string)
	if !ok {
		t.Fatalf("arguments = %T, want string (OpenAI требует строку с JSON)", fn["arguments"])
	}
	if !json.Valid([]byte(args)) {
		t.Errorf("arguments не валидный JSON: %q", args)
	}
}

func TestConvertRequest_Parameters(t *testing.T) {
	in := map[string]any{
		"model":          "local",
		"max_tokens":     float64(2048),
		"temperature":    0.7,
		"top_p":          0.95,
		"stop_sequences": []any{"\n\nHuman:"},
		"messages":       []any{map[string]any{"role": "user", "content": "hi"}},
	}
	out, _, err := ConvertRequest(in, ConvertOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if out["max_tokens"] != float64(2048) || out["temperature"] != 0.7 || out["top_p"] != 0.95 {
		t.Errorf("параметры потеряны: %v", out)
	}
	stop, ok := out["stop"].([]any)
	if !ok || len(stop) != 1 || stop[0] != "\n\nHuman:" {
		t.Errorf("stop = %v, want перенос из stop_sequences", out["stop"])
	}
	if _, ok := out["stop_sequences"]; ok {
		t.Errorf("stop_sequences не должен попасть в OpenAI-запрос")
	}
}

func TestConvertRequest_Tools(t *testing.T) {
	in := map[string]any{
		"model":    "local",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
		"tools": []any{
			map[string]any{"name": "Read", "description": "read",
				"input_schema": map[string]any{"type": "object"}},
		},
	}
	out, _, err := ConvertRequest(in, ConvertOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tools, ok := out["tools"].([]map[string]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %v", out["tools"])
	}
	if tools[0]["type"] != "function" {
		t.Errorf("tools[0].type = %v", tools[0]["type"])
	}
	fn := tools[0]["function"].(map[string]any)
	if fn["name"] != "Read" || fn["description"] != "read" || fn["parameters"] == nil {
		t.Errorf("function = %v", fn)
	}
}

func TestConvertRequest_UnknownFieldsDoNotBreak(t *testing.T) {
	// DisallowUnknownFields включать нельзя (STATE.md, решение №6):
	// незнакомые поля должны пропускаться, а не ронять конвертацию.
	in := map[string]any{
		"model":        "local",
		"metadata":     map[string]any{"user_id": "u1"},
		"service_tier": "auto",
		"messages":     []any{map[string]any{"role": "user", "content": "hi"}},
	}
	if _, _, err := ConvertRequest(in, ConvertOptions{}); err != nil {
		t.Fatalf("незнакомые поля сломали конвертацию: %v", err)
	}
}

// --- ошибки прокси в формате Anthropic (дефект №5) ---

func TestWriteAnthropicError_Format(t *testing.T) {
	w := httptest.NewRecorder()
	WriteAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "bad json")

	if w.Code != http.StatusBadRequest {
		t.Errorf("code = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	var got struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("тело не JSON: %v (%s)", err, w.Body.String())
	}
	if got.Type != "error" || got.Error.Type != "invalid_request_error" || got.Error.Message != "bad json" {
		t.Errorf("тело = %+v", got)
	}
}

// --- хелперы ---

// convertToolResult собирает диалог «assistant вызвал Bash → user вернул tool_result».
func convertToolResult(t *testing.T, content any, isErr bool, opts ConvertOptions) map[string]any {
	t.Helper()
	block := map[string]any{"type": "tool_result", "tool_use_id": "tu1", "content": content}
	if isErr {
		block["is_error"] = true
	}
	in := map[string]any{
		"model": "local",
		"messages": []any{
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "tool_use", "id": "tu1", "name": "Bash",
					"input": map[string]any{"command": "true"}},
			}},
			map[string]any{"role": "user", "content": []any{block}},
		},
	}
	out, _, err := ConvertRequest(in, opts)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func lastToolMsg(t *testing.T, m []map[string]any) map[string]any {
	t.Helper()
	for i := len(m) - 1; i >= 0; i-- {
		if m[i]["role"] == "tool" {
			return m[i]
		}
	}
	t.Fatalf("нет role=tool среди %v", m)
	return nil
}

func roles(m []map[string]any) []string {
	out := make([]string, 0, len(m))
	for _, x := range m {
		out = append(out, x["role"].(string))
	}
	return out
}
