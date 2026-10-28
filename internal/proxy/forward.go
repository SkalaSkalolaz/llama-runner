package proxy

import (
	"encoding/json"
	"net/http"

	"github.com/SkalaSkalolaz/llama-runner/internal/convert"
)

// ============================================================
// OpenAI → Anthropic conversion (response, non-streaming)
// ============================================================

// convertOpenAIResponseToAnthropic reads a complete (non-streaming) OpenAI
// chat/completions response, decodes it and writes the equivalent Anthropic
// Messages API JSON response to w: text content becomes a text block, each
// tool_calls entry a tool_use block (arguments parsed back into an input
// object), and finish_reason is mapped to a stop_reason
// (tool_calls→tool_use, length→max_tokens, otherwise end_turn). Token usage
// is carried over; a decode failure of the upstream body yields 502.
func convertOpenAIResponseToAnthropic(w http.ResponseWriter, openAIResp *http.Response, modelName string) {
	var oai struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(openAIResp.Body).Decode(&oai); err != nil {
		convert.WriteAnthropicError(w, http.StatusBadGateway, "api_error", "decode: "+err.Error())
		return
	}

	var content []map[string]any
	if len(oai.Choices) > 0 && oai.Choices[0].Message.Content != "" {
		content = append(content, map[string]any{
			"type": "text",
			"text": oai.Choices[0].Message.Content,
		})
	}
	if len(oai.Choices) > 0 && len(oai.Choices[0].Message.ToolCalls) > 0 {
		for _, tc := range oai.Choices[0].Message.ToolCalls {
			var input map[string]any
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
			content = append(content, map[string]any{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Function.Name,
				"input": input,
			})
		}
	}

	stopReason := "end_turn"
	if len(oai.Choices) > 0 {
		switch oai.Choices[0].FinishReason {
		case "tool_calls":
			stopReason = "tool_use"
		case "length":
			stopReason = "max_tokens"
		}
	}

	resp := map[string]any{
		"id":            oai.ID,
		"type":          "message",
		"role":          "assistant",
		"model":         modelName,
		"content":       content,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":  oai.Usage.PromptTokens,
			"output_tokens": oai.Usage.CompletionTokens,
		},
	}
	convert.WriteJSON(w, http.StatusOK, resp)
}

// ============================================================
// Utility functions
// ============================================================
