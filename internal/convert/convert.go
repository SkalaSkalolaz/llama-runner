// Package convert translates between the Anthropic Messages API and the OpenAI
// chat/completions API in both directions: requests go to the upstream as
// OpenAI-shaped payloads, upstream replies (streaming or not) come back as
// Anthropic-shaped events.
//
// What lives where:
//
//	convert.go   — request conversion (Anthropic → OpenAI) and its statistics
//	summary.go   — human-readable digests and preview/truncation helpers for -v
//	stream.go    — OpenAI SSE → Anthropic SSE re-emission
//	errors.go    — Anthropic-shaped error bodies and error-type mapping
//
// Conversion never logs and never touches the network: it takes parsed
// structures and returns parsed structures, so the proxy layer owns transport
// concerns. Statistics about what a conversion dropped (thinking blocks,
// images without --mmproj, unknown content types) are returned as ConvertStats
// for the caller to report under -v.
package convert

import (
	"fmt"
	"strings"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// ============================================================
// Anthropic → OpenAI conversion (request)
// ============================================================
//
// Moved from proxy.go in milestone M1 (PLAN.md). Works with map[string]any and
// intentionally does NOT use json.Decoder with DisallowUnknownFields: unknown fields
// in the request must pass through, not drop the message entirely.
//
// What happens with content blocks:
//
//	text             → concatenated into content string (as in M0)
//	tool_use         → assistant.tool_calls[]; arguments — ALWAYS a string with JSON
//	                   (mustJSON), otherwise the OpenAI server won't accept the field
//	tool_result      → role=tool; content as a string OR block array (text blocks
//	                   concatenated via "\n"); is_error:true → "Error: " prefix.
//	                   In M0 array content was dropped entirely — on a local
//	                   model tool use broke (STATE.md, defects #1 and #3).
//	system           → string or block array → single role=system (defect #2)
//	image            → on -mmproj (ConvertOptions.ImagesToOpenAI) → OpenAI
//	                   image_url data-URL; otherwise the block is dropped but NOT silently:
//	                   it's counted in ConvertStats.DroppedImages and visible at -v
//	thinking,
//	redacted_thinking→ dropped: OpenAI chat/completions has no field for chain
//	                   of thought, and the thinking-block signature is not reproducible.
//	                   Counted in ConvertStats.DroppedThinking, visible at -v
//	unknown type     → ConvertStats.DroppedUnknown
//
// Message order (two-pass assembly, see convertBlocks): first
// assistant.tool_calls, then role=tool. Normal flow (assistant with tool_use,
// next user with tool_result) gives exactly the same result as M0 — this
// is fixed by golden tests and TestConvertRequest_NormalFlowOrder.

// ConvertOptions controls how an Anthropic request is mapped onto the OpenAI
// chat/completions API.
type ConvertOptions struct {
	// ImagesToOpenAI enables mapping Anthropic image blocks to OpenAI
	// image_url parts. Set it only when llama-server runs with a vision
	// projector (-mmproj): without a projector there is nothing that could
	// process the pixels, so image blocks are dropped instead — and counted,
	// never silently.
	ImagesToOpenAI bool
}

// ConvertStats counts content blocks that did not reach the model during
// conversion, one counter per drop reason. It exists so that lossy
// conversion is never silent: the proxy surfaces these numbers under -v (see
// proxyHandler.convert).
type ConvertStats struct {
	// DroppedThinking counts thinking/redacted_thinking blocks — OpenAI
	// chat/completions has no chain-of-thought field.
	DroppedThinking int
	// DroppedImages counts image blocks with no projector configured or no
	// usable source.
	DroppedImages int
	// DroppedUnknown counts blocks whose type is not recognised at all.
	DroppedUnknown int
}

// Dropped returns the total number of blocks dropped during conversion,
// summing all reasons.
func (s ConvertStats) Dropped() int {
	return s.DroppedThinking + s.DroppedImages + s.DroppedUnknown
}

// Summary renders a compact one-line description of what was dropped
// (e.g. "2 thinking, 1 image") for the -v log. It returns an empty string
// when nothing was dropped, so callers can test the result directly.
func (s ConvertStats) Summary() string {
	var parts []string
	if s.DroppedThinking > 0 {
		parts = append(parts, fmt.Sprintf("%d thinking", s.DroppedThinking))
	}
	if s.DroppedImages > 0 {
		parts = append(parts, fmt.Sprintf("%d image", s.DroppedImages))
	}
	if s.DroppedUnknown > 0 {
		parts = append(parts, fmt.Sprintf(i18n.T(i18n.MsgSummaryUnknown), s.DroppedUnknown))
	}
	return strings.Join(parts, ", ")
}

// AnthropicToOpenAI converts an Anthropic request with default options, i.e.
// image blocks are not mapped (no vision projector assumed). Its signature is
// preserved from the original M0 implementation because the golden tests use
// it; new code should call ConvertRequest to get drop statistics.
func AnthropicToOpenAI(anthropic map[string]any) (map[string]any, error) {
	out, _, err := ConvertRequest(anthropic, ConvertOptions{})
	return out, err
}

// ConvertRequest converts an Anthropic Messages API request body into an
// OpenAI chat/completions request body, returning per-reason statistics of
// content blocks that could not be carried over.
//
// Mapping: the system prompt (string or block array) becomes the first
// role=system message; each conversation message goes through
// convertMessage, which may yield several OpenAI messages from one Anthropic
// one. Sampling parameters pass through where an equivalent exists
// (max_tokens, temperature, top_p; stop_sequences → stop), and Anthropic
// tools {name, description, input_schema} become OpenAI
// {type: function, function: {...}} entries.
//
// The input is deliberately handled as plain map[string]any without
// DisallowUnknownFields: fields the converter does not know about (and
// therefore does not copy) must never cause the request to be rejected.
// The returned error is reserved for unrecoverable problems; lossy parts of
// the conversion are reported through ConvertStats instead.
func ConvertRequest(anthropic map[string]any, opts ConvertOptions) (map[string]any, ConvertStats, error) {
	var st ConvertStats

	openAI := map[string]any{
		"model": anthropic["model"],
	}

	// system → first role=system message.
	var messages []map[string]any
	if sys := systemText(anthropic["system"]); sys != "" {
		messages = append(messages, map[string]any{"role": "system", "content": sys})
	}

	// messages: content can be a string or block array.
	if inMsgs, ok := anthropic["messages"].([]any); ok {
		for _, m := range inMsgs {
			msg, ok := m.(map[string]any)
			if !ok {
				continue
			}
			role, _ := msg["role"].(string)
			messages = append(messages, convertMessage(role, msg["content"], opts, &st)...)
		}
	}
	openAI["messages"] = messages

	// Parameters
	if v, ok := anthropic["max_tokens"]; ok {
		openAI["max_tokens"] = v
	}
	if v, ok := anthropic["temperature"]; ok {
		openAI["temperature"] = v
	}
	if v, ok := anthropic["top_p"]; ok {
		openAI["top_p"] = v
	}
	if v, ok := anthropic["stop_sequences"]; ok {
		openAI["stop"] = v
	}

	// tools: Anthropic {name, description, input_schema} → OpenAI {type:function, function:{...}}
	if tools, ok := anthropic["tools"].([]any); ok && len(tools) > 0 {
		var openAITools []map[string]any
		for _, t := range tools {
			tool, ok := t.(map[string]any)
			if !ok {
				continue
			}
			openAITools = append(openAITools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        tool["name"],
					"description": tool["description"],
					"parameters":  tool["input_schema"],
				},
			})
		}
		openAI["tools"] = openAITools
	}

	return openAI, st, nil
}

// systemText flattens the Anthropic system field into a single string for the
// role=system message. A plain string is returned as-is; a block array is
// reduced to its text blocks joined with "\n" — cache_control markers are
// discarded because OpenAI's prompt caching works differently and the marker
// has no meaning there. Anything else yields "".
func systemText(sys any) string {
	switch v := sys.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, b := range v {
			block, ok := b.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := block["type"].(string); t != "text" {
				continue
			}
			if s, ok := block["text"].(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// convertMessage converts one Anthropic message into zero, one or more
// OpenAI messages: a plain string content maps 1:1, a content-block array is
// handed to convertBlocks, and a message with absent or unrecognised content
// is skipped entirely (matching M0 behaviour).
func convertMessage(role string, content any, opts ConvertOptions, st *ConvertStats) []map[string]any {
	switch v := content.(type) {
	case string:
		return []map[string]any{{"role": role, "content": v}}
	case []any:
		return convertBlocks(role, v, opts, st)
	}
	// content is absent or in an unknown form — skip the message (as in M0).
	return nil
}

// convertBlocks performs the two-pass assembly of one message's content
// blocks. The first pass sorts blocks into a "main" message payload (text,
// images, tool_calls) and separate role=tool answers; the second pass emits
// them in the order the OpenAI API requires:
//
//	assistant with tool_calls → the assistant message first, then its
//	                             tool answers;
//	user (carrying tool_result) → tool answers first — they must
//	                             immediately follow the assistant's
//	                             tool_calls — then the user's own text.
//
// For the normal Anthropic flow (tool_use and tool_result in separate
// messages) the resulting order is byte-for-byte identical to M0. Images
// inside an assistant message are dropped (counted), since OpenAI assistant
// messages cannot carry multimodal content.
func convertBlocks(role string, blocks []any, opts ConvertOptions, st *ConvertStats) []map[string]any {
	var (
		textParts    []string
		parts        []any // content as an array — only needed when the message has images
		toolCalls    []map[string]any
		toolMsgs     []map[string]any
		mappedImages int
	)

	for _, b := range blocks {
		block, ok := b.(map[string]any)
		if !ok {
			continue
		}
		switch btype, _ := block["type"].(string); btype {
		case "text":
			if t, ok := block["text"].(string); ok {
				textParts = append(textParts, t)
				parts = append(parts, map[string]any{"type": "text", "text": t})
			}
		case "image":
			if url, ok := imageDataURL(block); ok && opts.ImagesToOpenAI {
				mappedImages++
				parts = append(parts, map[string]any{
					"type": "image_url", "image_url": map[string]any{"url": url},
				})
			} else {
				st.DroppedImages++
			}
		case "thinking", "redacted_thinking":
			st.DroppedThinking++
		case "tool_use":
			toolCalls = append(toolCalls, map[string]any{
				"id":   block["id"],
				"type": "function",
				"function": map[string]any{
					"name":      block["name"],
					"arguments": mustJSON(block["input"]),
				},
			})
		case "tool_result":
			if m, ok := toolResultMessage(block, opts, st); ok {
				toolMsgs = append(toolMsgs, m)
			}
		default:
			st.DroppedUnknown++
		}
	}

	var main map[string]any
	switch {
	case len(toolCalls) > 0:
		// In OpenAI, assistant messages cannot have multimodal content,
		// so image blocks have nowhere to go here — they go to the counter.
		st.DroppedImages += mappedImages
		mappedImages = 0
		main = map[string]any{
			"role":       "assistant",
			"content":    strings.Join(textParts, ""),
			"tool_calls": toolCalls,
		}
	case len(textParts) > 0 || mappedImages > 0:
		content := any(strings.Join(textParts, ""))
		if mappedImages > 0 {
			content = parts
		}
		main = map[string]any{"role": role, "content": content}
	}

	switch {
	case main == nil:
		return toolMsgs
	case len(toolCalls) > 0:
		return append([]map[string]any{main}, toolMsgs...)
	default:
		return append(toolMsgs, main)
	}
}

// toolResultMessage converts an Anthropic tool_result block into the
// corresponding role=tool message, keyed by tool_use_id.
//
// The result content may be a string, an array of blocks (text blocks joined
// with "\n", image blocks mapped to image_url when a projector is configured
// and counted as dropped otherwise), a scalar/object (serialised to JSON so
// no data is lost) or absent. Even an empty result produces a message: every
// tool call must be closed, or the OpenAI server rejects the whole request.
// When is_error is true the text is prefixed with "Error: " so the failure
// actually reaches the model — OpenAI has no structured error flag here.
func toolResultMessage(block map[string]any, opts ConvertOptions, st *ConvertStats) (map[string]any, bool) {
	var (
		text       string
		imageParts []any
	)

	switch c := block["content"].(type) {
	case nil:
		// Empty result: we still emit the role=tool message — the call must be
		// be closed, otherwise the OpenAI server will reject the entire request.
	case string:
		text = c
	case []any:
		var texts []string
		for _, b := range c {
			inner, ok := b.(map[string]any)
			if !ok {
				continue
			}
			switch t, _ := inner["type"].(string); t {
			case "text":
				if s, ok := inner["text"].(string); ok {
					texts = append(texts, s)
				}
			case "image":
				if url, ok := imageDataURL(inner); ok && opts.ImagesToOpenAI {
					imageParts = append(imageParts, map[string]any{
						"type": "image_url", "image_url": map[string]any{"url": url},
					})
				} else {
					st.DroppedImages++
				}
			default:
				st.DroppedUnknown++
			}
		}
		text = strings.Join(texts, "\n")
	default:
		// Number, bool, object — emit as JSON, no reason to lose data.
		text = mustJSON(block["content"])
	}

	if isErr, _ := block["is_error"].(bool); isErr {
		text = "Error: " + text
	}

	msg := map[string]any{
		"role":         "tool",
		"tool_call_id": block["tool_use_id"],
	}
	if len(imageParts) > 0 {
		content := make([]any, 0, len(imageParts)+1)
		if text != "" {
			content = append(content, map[string]any{"type": "text", "text": text})
		}
		content = append(content, imageParts...)
		msg["content"] = content
	} else {
		msg["content"] = text
	}
	return msg, true
}

// imageDataURL maps an Anthropic image block's source to an OpenAI image_url
// string: a base64 source becomes a data: URL (media_type defaults to
// application/octet-stream when missing), a url source is passed through
// unchanged. It reports false when the block has no usable source — wrong
// shape, empty base64 data or empty URL.
func imageDataURL(block map[string]any) (string, bool) {
	src, ok := block["source"].(map[string]any)
	if !ok {
		return "", false
	}
	switch kind, _ := src["type"].(string); kind {
	case "base64":
		data, _ := src["data"].(string)
		if data == "" {
			return "", false
		}
		mime, _ := src["media_type"].(string)
		if mime == "" {
			mime = "application/octet-stream"
		}
		return "data:" + mime + ";base64," + data, true
	case "url":
		u, _ := src["url"].(string)
		return u, u != ""
	}
	return "", false
}
