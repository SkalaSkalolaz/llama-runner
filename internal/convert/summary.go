package convert

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// RequestSummary renders a brief human-readable digest of a converted OpenAI
// request for the -v log: total message count, whether a system message is
// present, how many assistant messages carry tool_calls, how many role=tool
// answers there are, and how many tool schemas were sent. It lets you verify
// at a glance that the system prompt and tool results actually reached the
// upstream, without dumping the full request body.
func RequestSummary(openAIReq map[string]any) string {
	msgs, _ := openAIReq["messages"].([]map[string]any)
	var (
		hasSystem bool
		toolMsgs  int
		toolCalls int
	)
	for _, m := range msgs {
		switch m["role"] {
		case "system":
			hasSystem = true
		case "tool":
			toolMsgs++
		case "assistant":
			if _, ok := m["tool_calls"]; ok {
				toolCalls++
			}
		}
	}
	tools, _ := openAIReq["tools"].([]map[string]any)
	return fmt.Sprintf(i18n.T(i18n.MsgSummaryMessages),
		len(msgs), hasSystem, toolCalls, toolMsgs, len(tools))
}

// mustJSON serialises v to its compact JSON representation for the
// tool_calls arguments field, which OpenAI requires to be a JSON string.
// Marshalling errors are swallowed (the zero value "null" is produced);
// callers pass plain decoded JSON values, which always serialise.
func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// ToolMessagesPreview produces one log line per role=tool message in a
// converted request, with the tool_call_id and a truncated content preview
// (see previewContent). It backs the -v log so you can see exactly what each
// tool returned — including the "Error: " prefix set for is_error results —
// without dumping full payloads.
func ToolMessagesPreview(openAIReq map[string]any) []string {
	msgs, _ := openAIReq["messages"].([]map[string]any)
	var out []string
	for i, m := range msgs {
		if m["role"] != "tool" {
			continue
		}
		out = append(out, fmt.Sprintf("tool[%d] tool_call_id=%v content=%q",
			i, m["tool_call_id"], previewContent(m["content"], 80)))
	}
	return out
}

// previewContent renders a message content value for the log. role=tool
// content is normally a string (truncated to maxLen), but with -mmproj it can
// be a multimodal block array — that is replaced by a fixed placeholder note
// rather than dumping base64 image data into the log. Anything else is
// serialised to JSON and truncated.
func previewContent(v any, maxLen int) string {
	switch c := v.(type) {
	case string:
		return truncate(c, maxLen)
	case []any:
		return "[multimodal content]"
	default:
		return truncate(mustJSON(v), maxLen)
	}
}

// truncate prepares a string for a single-line log entry: literal "\n" and
// "\r" sequences are escaped, and the result is cut to maxLen runes (counted
// by runes, never bytes, so multi-byte UTF-8 characters are never broken),
// with a trailing ellipsis when anything was cut off.
func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}
