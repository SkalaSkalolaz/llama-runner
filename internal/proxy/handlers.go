package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/convert"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// ---- /v1/models ----

// handleModels serves GET /v1/models with an Anthropic-format model list so
// the proxy passes Claude Code's startup model check. It advertises the local
// llama-server, the configured external model, and the standard aliases
// (sonnet/haiku/opus) — the aliases are listed for visibility only. When a
// switcher is wired, the local model names it can serve are advertised as
// well; routing sends everything else upstream.
func (h *proxyHandler) handleModels(w http.ResponseWriter, r *http.Request) {
	if !h.externalMode() {
		if h.switcher == nil {
			h.passthrough(w, r)
			return
		}
		h.handleModelsLocal(w)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)

	data := []modelEntry{
		{ID: localModelName, Type: "model", DisplayName: localModelName, CreatedAt: now},
		{ID: h.cfg.OpenAIModel, Type: "model", DisplayName: h.cfg.OpenAIModel, CreatedAt: now},
	}
	// Standard aliases — for visibility in /v1/models.
	// Routing still goes to openai since they aren't "llama-server".
	for _, alias := range []string{"sonnet", "haiku", "opus"} {
		data = append(data, modelEntry{ID: alias, Type: "model", DisplayName: alias, CreatedAt: now})
	}
	// With a switcher wired, the local model names it routes to the local
	// route are advertised too, so Claude Code's model checks see exactly
	// the names it may send (menu entries included).
	for _, name := range h.advertisedLocalNames() {
		if indexOfModelID(data, name) >= 0 {
			continue
		}
		data = append(data, modelEntry{ID: name, Type: "model", DisplayName: name, CreatedAt: now})
	}

	resp := map[string]any{
		"data":     data,
		"has_more": false,
		"first_id": data[0].ID,
		"last_id":  data[len(data)-1].ID,
	}
	convert.WriteJSON(w, http.StatusOK, resp)
}

// modelEntry is one model object in an Anthropic-format /v1/models list.
type modelEntry struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

// advertisedLocalNames lists the local model names worth publishing on
// /v1/models: the switcher's menu identifiers, or nothing when no switcher is
// wired (then only the fixed entries exist).
func (h *proxyHandler) advertisedLocalNames() []string {
	if h.switcher == nil {
		return nil
	}
	return h.switcher.ModelNames()
}

// indexOfModelID returns the position of id (case-insensitive) in the model
// list, or -1 when absent. Used to keep /v1/models free of duplicates when
// the switcher's names overlap the fixed entries.
func indexOfModelID(data []modelEntry, id string) int {
	for i, m := range data {
		if strings.EqualFold(m.ID, id) {
			return i
		}
	}
	return -1
}

// ---- /v1/messages ----

// handleMessages is the core route, POST /v1/messages. It reads the body
// under maxRequestBodyBytes (oversized bodies get 413 request_too_large,
// other read errors 400), parses the Anthropic request, and routes it by the
// requested model: the localModelName (case- and whitespace-insensitive) goes
// to the local llama-server, everything else to the external provider.
//
// When a switcher is wired, names it recognises (profile display names and
// the loaded model's alias) also go local: the switcher first makes the
// llama-server actually serve that model — possibly restarting it — and the
// request's model field is rewritten to the served alias before conversion.
// The gateway is untouched: any other name still goes upstream.
//
// The stream flag from the request body selects between streaming (SSE) and
// single-response conversion downstream.
func (h *proxyHandler) handleMessages(w http.ResponseWriter, r *http.Request) {
	if !h.externalMode() {
		if h.switcher == nil {
			h.passthrough(w, r)
			return
		}
		h.handleMessagesLocal(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			convert.WriteAnthropicError(w, http.StatusRequestEntityTooLarge,
				"request_too_large", strings.TrimSuffix(i18n.TF(i18n.MsgBodyTooLarge, maxErr.Limit), "\n"))
			return
		}
		convert.WriteAnthropicError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	defer r.Body.Close()

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		convert.WriteAnthropicError(w, http.StatusBadRequest, "invalid_request_error", strings.TrimSuffix(i18n.TF(i18n.MsgInvalidJSON, err), "\n"))
		return
	}
	model, _ := req["model"].(string)
	stream, _ := req["stream"].(bool)

	isLocal := strings.EqualFold(strings.TrimSpace(model), localModelName)

	// A name the switcher claims (a profile display name, the as-launched
	// alias) is local too: make the server serve it first, then rewrite the
	// request to the alias the (possibly restarted) server reports.
	if !isLocal && h.switcher != nil && h.switcher.Handles(model) {
		alias, swErr := h.switcher.EnsureLoaded(r.Context(), model)
		if swErr != nil {
			h.logf("переключение модели %q не удалось: %v", model, swErr)
			convert.WriteAnthropicError(w, http.StatusBadGateway, "api_error",
				strings.TrimSuffix(i18n.TF(i18n.MsgModelSwitchFailed, swErr), "\n"))
			return
		}
		if !strings.EqualFold(model, alias) {
			req["model"] = alias
			h.logf("модель запроса переписана %q → %q", model, alias)
		}
		isLocal = true
	}

	if isLocal {
		h.forwardToLlama(w, r, req, stream)
		return
	}
	h.forwardToOpenAI(w, r, req, stream)
}

// ---- Route to llama-server ----

// forwardToLlama converts the Anthropic request to OpenAI chat/completions
// format and sends it to the local llama-server (which needs no
// authentication). For streaming requests stream_options.include_usage is
// added so token usage arrives in the final chunk.
//
// Upstream failures map to Anthropic errors: transport errors become 502,
// non-200 responses are relayed with their status and a mapped error type.
// On success the response is converted back to Anthropic format — SSE events
// when streaming, a single JSON message otherwise — and token usage is logged
// under -v.
func (h *proxyHandler) forwardToLlama(w http.ResponseWriter, r *http.Request, req map[string]any, stream bool) {
	openAIReq, err := h.convert(req)
	if err != nil {
		convert.WriteAnthropicError(w, http.StatusInternalServerError, "api_error", strings.TrimSuffix(i18n.TF(i18n.MsgConvertErr, err), "\n"))
		return
	}
	openAIReq["stream"] = stream
	if stream {
		openAIReq["stream_options"] = map[string]any{"include_usage": true}
	}

	target := h.llamaServerURL + "/v1/chat/completions"
	body, _ := json.Marshal(openAIReq)
	proxyReq, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, target, bytes.NewReader(body))
	proxyReq.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(proxyReq)
	if err != nil {
		convert.WriteAnthropicError(w, http.StatusBadGateway, "api_error", strings.TrimSuffix(i18n.TF(i18n.MsgUpstreamErr, err), "\n"))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		convert.WriteAnthropicError(w, resp.StatusCode, convert.AnthropicErrorType(resp.StatusCode),
			strings.TrimSuffix(i18n.TF(i18n.MsgLlamaUpstream, string(b)), "\n"))
		return
	}
	if stream {
		in, out := convert.ConvertOpenAIStreamToAnthropic(w, resp, "llama-server")
		h.logf(i18n.T(i18n.MsgProxyUsage), in, out)
		return
	}
	convertOpenAIResponseToAnthropic(w, resp, "llama-server")
}

// ---- Route to external OpenAI-compatible service ----

// forwardToOpenAI converts the Anthropic request to OpenAI chat/completions
// format and forwards it to the configured external provider
// (cfg.OpenAIBaseURL + /chat/completions), authenticating with
// Authorization: Bearer cfg.OpenAIKey — or with no Authorization header when
// the provider was configured without a key (cfg.OpenAIKeyless). Like forwardToLlama it requests usage
// accounting for streams, maps transport/upstream failures to Anthropic error
// responses, and converts successful responses — streaming or not — back to
// the Anthropic format.
func (h *proxyHandler) forwardToOpenAI(w http.ResponseWriter, r *http.Request, req map[string]any, stream bool) {
	openAIReq, err := h.convert(req)
	if err != nil {
		convert.WriteAnthropicError(w, http.StatusInternalServerError, "api_error", strings.TrimSuffix(i18n.TF(i18n.MsgConvertErr, err), "\n"))
		return
	}
	openAIReq["stream"] = stream
	if stream {
		openAIReq["stream_options"] = map[string]any{"include_usage": true}
	}
	target := strings.TrimRight(h.cfg.OpenAIBaseURL, "/") + "/chat/completions"
	body, _ := json.Marshal(openAIReq)
	proxyReq, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, target, bytes.NewReader(body))
	proxyReq.Header.Set("Content-Type", "application/json")
	// The key is the only credential this client knows, and an unauthenticated
	// provider (cfg.OpenAIKeyless) must see NO Authorization header at all —
	// not an empty one and not "Bearer ": exactly the request a bare
	// `curl -H 'Content-Type: application/json' …/chat/completions` makes.
	if h.cfg.OpenAIKey != "" {
		proxyReq.Header.Set("Authorization", "Bearer "+h.cfg.OpenAIKey)
	}

	resp, err := h.client.Do(proxyReq)
	if err != nil {
		convert.WriteAnthropicError(w, http.StatusBadGateway, "api_error", strings.TrimSuffix(i18n.TF(i18n.MsgUpstreamOpenai, err), "\n"))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		convert.WriteAnthropicError(w, resp.StatusCode, convert.AnthropicErrorType(resp.StatusCode),
			strings.TrimSuffix(i18n.TF(i18n.MsgOpenaiUpstream, string(b)), "\n"))
		return
	}
	if stream {
		in, out := convert.ConvertOpenAIStreamToAnthropic(w, resp, h.cfg.OpenAIModel)
		h.logf(i18n.T(i18n.MsgProxyUsage), in, out)
		return
	}
	convertOpenAIResponseToAnthropic(w, resp, h.cfg.OpenAIModel)
}

// ---- Other (/v1/complete, etc.) ----

// handleOther is the catch-all route for everything that is not
// /v1/messages or the /v1/models collection:
//
//   - */count_tokens — answered with an approximate token count
//     (body size / 4, minimum 1); Claude Code only needs a plausible number
//     to render its context meter, not an exact tokenizer result;
//   - GET /v1/models/<id> — echoes a minimal single-model object so lookups
//     of a known ID succeed;
//   - anything else — 404 not_found_error.
func (h *proxyHandler) handleOther(w http.ResponseWriter, r *http.Request) {
	if !h.externalMode() {
		if h.switcher == nil {
			h.passthrough(w, r)
			return
		}
		h.handleOtherLocal(w, r)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/count_tokens") {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		model, _ := req["model"].(string)

		approx := len(body) / 4
		if approx < 1 {
			approx = 1
		}
		_ = model
		convert.WriteJSON(w, http.StatusOK, map[string]any{"input_tokens": approx})
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/models/") && r.Method == http.MethodGet {
		id := strings.TrimPrefix(r.URL.Path, "/v1/models/")
		convert.WriteJSON(w, http.StatusOK, map[string]any{
			"id": id, "type": "model", "display_name": id,
		})
		return
	}

	convert.WriteAnthropicError(w, http.StatusNotFound, "not_found_error", "not implemented: "+r.URL.Path)
}
