package convert

import (
	"strings"
	"testing"
)

func TestRequestSummary(t *testing.T) {
	// Сводка для -v: по ней проверяют в ручном прогоне, что system и tool-ответы дошли.
	t.Setenv("LC_ALL", "ru_RU.UTF-8") // сводка локализована
	out, _, err := ConvertRequest(map[string]any{
		"model":  "local",
		"system": "s",
		"tools":  []any{map[string]any{"name": "Bash"}},
		"messages": []any{
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "tool_use", "id": "tu1", "name": "Bash", "input": map[string]any{}},
			}},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "tu1", "content": "ok"},
			}},
		},
	}, ConvertOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := RequestSummary(out)
	for _, want := range []string{"system=true", "tool_calls=1", "tool-ответов=1", "схем инструментов=1"} {
		if !strings.Contains(got, want) {
			t.Errorf("в сводке %q нет %q", got, want)
		}
	}
}
func TestToolMessagesPreview(t *testing.T) {
	out := map[string]any{
		"messages": []map[string]any{
			{"role": "system", "content": "s"},
			{"role": "assistant", "content": "", "tool_calls": []map[string]any{}},
			{"role": "tool", "tool_call_id": "tu1", "content": "ok"},
			{"role": "tool", "tool_call_id": "tu2", "content": "Error: bash: nope: command not found"},
		},
	}
	got := ToolMessagesPreview(out)
	if len(got) != 2 {
		t.Fatalf("lines = %v", got)
	}
	if !strings.Contains(got[0], `tool_call_id=tu1`) || !strings.Contains(got[0], `content="ok"`) {
		t.Errorf("line[0] = %q", got[0])
	}
	if !strings.Contains(got[1], "Error: ") {
		t.Errorf("line[1] = %q (префикс Error: должен быть виден в логе)", got[1])
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"short", 10, "short"},
		{"exactly-ten", 11, "exactly-ten"},
		{"line1\nline2", 100, `line1\nline2`},
		{"aaaaaaaaaaaaaaaaaaaa", 5, "aaaaa…"},
		{"привет мир", 6, "привет…"},
	}
	for _, c := range cases {
		if got := truncate(c.in, c.max); got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
		}
	}
}
