package convert

import "testing"

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
