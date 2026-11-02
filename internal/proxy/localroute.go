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

// This file owns the local route of the proxy while a ModelSwitcher is
// wired. Without a switcher the local route is a verbatim passthrough (see
// passthrough.go); with one, a request naming a configured local model is
// first handed to the switcher — which may stop the current llama-server and
// start it with another profile — and then forwarded with its model field
// rewritten to the alias the (new) server actually reports. Everything here
// stays Anthropic-format: no dialect conversion happens on the local route.

// handleMessagesLocal is the local-mode POST /v1/messages: read the body,
// ask the switcher to make the server serve the named model (when the name
// is one it knows), rewrite the model field to the loaded alias and forward.
// A body that is not JSON, or a model the switcher does not claim, is
// forwarded untouched — same behaviour as the plain passthrough.
func (h *proxyHandler) handleMessagesLocal(w http.ResponseWriter, r *http.Request) {
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
		h.forwardLocal(w, r, body)
		return
	}
	model, _ := req["model"].(string)

	if h.switcher.Handles(model) {
		alias, swErr := h.switcher.EnsureLoaded(r.Context(), model)
		if swErr != nil {
			h.logf("переключение модели %q не удалось: %v", model, swErr)
			convert.WriteAnthropicError(w, http.StatusBadGateway, "api_error",
				strings.TrimSuffix(i18n.TF(i18n.MsgModelSwitchFailed, swErr), "\n"))
			return
		}
		if !strings.EqualFold(model, alias) {
			req["model"] = alias
			if body, err = json.Marshal(req); err != nil {
				convert.WriteAnthropicError(w, http.StatusInternalServerError, "api_error", err.Error())
				return
			}
			h.logf("модель запроса переписана %q → %q", model, alias)
		}
	}
	h.forwardLocal(w, r, body)
}

// forwardLocal sends the (possibly rewritten) body to the local llama-server
// under the original path/query and streams the response back.
func (h *proxyHandler) forwardLocal(w http.ResponseWriter, r *http.Request, body []byte) {
	target := h.llamaServerURL + r.URL.Path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, bytes.NewReader(body))
	if err != nil {
		convert.WriteAnthropicError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
	copyHeadersExcept(req.Header, r.Header, "Content-Length")
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		convert.WriteAnthropicError(w, http.StatusBadGateway, "api_error",
			strings.TrimSuffix(i18n.TF(i18n.MsgUpstreamErr, err), "\n"))
		return
	}
	defer resp.Body.Close()
	streamUpstream(w, resp)
	h.logf("local: %s %s → %s", r.Method, r.URL.Path, target)
}

// handleModelsLocal advertises the model names the local route can serve —
// the menu identifiers from the switcher — so Claude Code's model checks see
// exactly the names it may send. Served by the proxy itself because the
// llama-server behind it only knows the one model it currently loads.
func (h *proxyHandler) handleModelsLocal(w http.ResponseWriter) {
	now := time.Now().UTC().Format(time.RFC3339)

	names := h.switcher.ModelNames()
	data := make([]modelEntry, 0, len(names))
	for _, name := range names {
		data = append(data, modelEntry{ID: name, Type: "model", DisplayName: name, CreatedAt: now})
	}
	convert.WriteJSON(w, http.StatusOK, map[string]any{
		"data":     data,
		"has_more": false,
	})
}

// handleOtherLocal is the local-mode catch-all while a switcher is wired:
// single-model lookups are answered by the proxy (the names live in the
// menu, not necessarily in the running server), everything else — token
// counting and any native llama-server endpoint — goes upstream unchanged.
func (h *proxyHandler) handleOtherLocal(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/v1/models/") && r.Method == http.MethodGet {
		id := strings.TrimPrefix(r.URL.Path, "/v1/models/")
		convert.WriteJSON(w, http.StatusOK, map[string]any{
			"id": id, "type": "model", "display_name": id,
		})
		return
	}
	h.passthrough(w, r)
}
