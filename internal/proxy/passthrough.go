package proxy

import (
	"net/http"
	"strings"

	"github.com/SkalaSkalolaz/llama-runner/internal/convert"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// This file implements the local-only proxy mode: no external provider is
// configured, so every request is forwarded to llama-server verbatim and the
// response is streamed back unchanged. Claude Code used to talk to
// llama-server directly (its native Anthropic API); the passthrough keeps
// that behaviour byte-for-byte — no request or response conversion happens
// here — while putting the launcher's proxy in front of the server, which is
// where later work (switching models while running) will hook in.

// hopByHopHeaders are removed when copying headers in either direction:
// they describe a single transport-level connection and must not survive a
// proxy hop (RFC 7230 §6.1). Content-Length is dropped on responses because
// the body is re-streamed chunk by chunk.
var hopByHopHeaders = []string{
	"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
	"Te", "Trailer", "Transfer-Encoding", "Upgrade",
}

// copyHeadersExcept copies src into dst skipping the hop-by-hop headers and
// any name listed in extra (case-insensitive).
func copyHeadersExcept(dst, src http.Header, extra ...string) {
	skip := make(map[string]bool, len(hopByHopHeaders)+len(extra))
	for _, name := range hopByHopHeaders {
		skip[strings.ToLower(name)] = true
	}
	for _, name := range extra {
		skip[strings.ToLower(name)] = true
	}
	for k, vs := range src {
		if skip[strings.ToLower(k)] {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

// passthrough forwards the incoming request to llamaServerURL with the same
// method, path, query, headers and body, then copies the upstream status,
// headers and body back to the client, flushing every chunk so SSE streams
// arrive incrementally (the same bytes Claude Code would have received
// talking to llama-server directly).
//
// The body is size-capped with the same limit as the converted routes.
// Transport failures map to a 502 Anthropic error, mirroring forwardToLlama.
func (h *proxyHandler) passthrough(w http.ResponseWriter, r *http.Request) {
	target := h.llamaServerURL + r.URL.Path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		convert.WriteAnthropicError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
	copyHeadersExcept(req.Header, r.Header)

	resp, err := h.local().Do(req)
	if err != nil {
		convert.WriteAnthropicError(w, http.StatusBadGateway, "api_error",
			strings.TrimSuffix(i18n.TF(i18n.MsgUpstreamErr, err), "\n"))
		return
	}
	defer resp.Body.Close()

	streamUpstream(w, resp)
	h.logf("passthrough: %s %s → %s (без изменений)", r.Method, r.URL.Path, target)
}

// streamUpstream copies the upstream status, non-hop headers and body to w,
// flushing every chunk so SSE streams arrive incrementally. Shared by the
// passthrough and the switched local route.
func streamUpstream(w http.ResponseWriter, resp *http.Response) {
	copyHeadersExcept(w.Header(), resp.Header, "Content-Length")
	w.WriteHeader(resp.StatusCode)

	flusher := newCtrlFlusher(w)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return // клиент ушёл — обрыв виден апстриму по отменёному ctx
			}
			flusher.flush()
		}
		if readErr != nil {
			return
		}
	}
}

// ctrlFlusher flushes w through http.ResponseController, which finds the
// Flusher even behind responseWriter wrappers. A server without flushing
// support simply never flushes early — the body still arrives in full.
type ctrlFlusher struct{ w http.ResponseWriter }

func newCtrlFlusher(w http.ResponseWriter) ctrlFlusher { return ctrlFlusher{w: w} }

func (f ctrlFlusher) flush() {
	rc := http.NewResponseController(f.w)
	// Errors are not actionable here: a failed flush means the connection
	// cannot flush early, not that the response is wrong; the write loop
	// surfaces any real breakage on the next Write.
	_ = rc.Flush()
}
