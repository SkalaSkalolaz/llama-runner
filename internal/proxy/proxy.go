package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/convert"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// localModelName is the model ID that routes to the local llama-server. Any
// other model name received by the proxy is forwarded to the external
// OpenAI-compatible provider.
const localModelName = "llama-server"

// Options bundles everything Start needs.
type Options struct {
	// ProxyAddr is the listen address (host:port) for Claude Code.
	ProxyAddr string
	// LlamaServerURL is the base URL (no trailing slash) of the local
	// llama-server.
	LlamaServerURL string
	// Cfg supplies provider settings, proxy token and verbosity.
	Cfg *config.Config
	// LogWriter receives [proxy]-prefixed diagnostics under -v.
	LogWriter io.Writer
}

// Start binds an HTTP listener on opts.ProxyAddr and starts serving the proxy
// in a background goroutine, returning the *http.Server so the caller can shut
// it down gracefully.
//
// All incoming requests pass through authMiddleware: with cfg.ProxyToken set,
// every request must present the token; with it empty, no authentication is
// performed.
//
// opts.LogWriter is the sink for [proxy] diagnostic lines emitted under -v —
// normally os.Stderr when output is piped/redirected, or the open
// llama-server log file in interactive mode (to avoid corrupting the TUI).
// nil disables proxy logging.
func Start(opts Options) (*http.Server, error) {
	h := &proxyHandler{
		llamaServerURL: strings.TrimRight(opts.LlamaServerURL, "/"),
		cfg:            opts.Cfg,
		client:         newProxyClient(),
		localClient:    newLocalProxyClient(),
		logWriter:      opts.LogWriter,
	}

	mux := newProxyMux(h)

	ln, err := net.Listen("tcp", opts.ProxyAddr)
	if err != nil {
		return nil, fmt.Errorf("не удалось занять %s: %w", opts.ProxyAddr, err)
	}
	srv := &http.Server{Handler: h.authMiddleware(mux)}
	go func() {
		_ = srv.Serve(ln) // http.ErrServerClosed при Shutdown — нормально
	}()
	return srv, nil
}

// maxRequestBodyBytes is the hard limit on incoming request bodies, enforced
// with http.MaxBytesReader. Real Claude Code payloads stay below 10 MB; the
// 100 MB ceiling adds a large safety margin for base64-encoded images and
// very long conversation histories while still capping pathological input.
const maxRequestBodyBytes int64 = 100 * 1024 * 1024

// proxyHandler implements the Anthropic Messages API facade that routes
// traffic between Claude Code and either the local llama-server or an
// external OpenAI-compatible provider, converting requests and responses
// between the two API dialects.
type proxyHandler struct {
	// llamaServerURL is the base URL (no trailing slash) of the local
	// llama-server used for requests whose model is localModelName.
	llamaServerURL string
	// cfg supplies the external provider settings, proxy token and verbosity.
	cfg *config.Config
	// client performs upstream requests to the EXTERNAL provider without a
	// whole-request timeout, with a short ResponseHeaderTimeout (see
	// newProxyClient).
	client *http.Client
	// localClient performs requests to the LOCAL llama-server: same shape as
	// client but with a much longer ResponseHeaderTimeout to accommodate
	// slow prefill (see newLocalProxyClient). May be nil when a handler was
	// constructed directly in a test — callers use h.local() to get a
	// non-nil client either way.
	localClient *http.Client
	// logWriter receives [proxy]-prefixed diagnostic lines when -v is set;
	// nil disables logging entirely.
	logWriter io.Writer
}

// externalMode reports whether the proxy runs in hybrid mode: an external
// OpenAI-compatible provider is fully configured, so requests are routed by
// model name and converted between the API dialects. Without it the proxy is
// a local-only passthrough: every request goes to llama-server unchanged
// (see passthrough.go).
func (h *proxyHandler) externalMode() bool {
	return h.cfg != nil && h.cfg.OpenAIProviderConfigured()
}

// local returns the http.Client used for requests to llama-server. In
// production it is localClient (long ResponseHeaderTimeout, see
// newLocalProxyClient); in tests that construct proxyHandler directly with
// only client set, it falls back to client so those tests keep working.
func (h *proxyHandler) local() *http.Client {
	if h.localClient != nil {
		return h.localClient
	}
	return h.client
}

// newProxyMux assembles the proxy's routing table:
//
//	/v1/messages                 → handleMessages (the core endpoint)
//	/v1/models                   → handleModels
//	/v1/messages/count_tokens    → handleOther (approximate count)
//	/v1/models/<id>              → handleOther (single model echo)
//	/                            → handleOther (404 for everything else)
//
// It is factored out of Start so that end-to-end tests can serve
// the exact same wiring (httptest.NewServer over authMiddleware+mux) without
// binding a real port.
func newProxyMux(h *proxyHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/messages", h.handleMessages)
	mux.HandleFunc("/v1/models", h.handleModels)
	mux.HandleFunc("/", h.handleOther)
	mux.HandleFunc("/v1/messages/count_tokens", h.handleOther)
	mux.HandleFunc("/v1/models/", h.handleOther)
	return mux
}

// authMiddleware wraps next with bearer-token authentication. When
// -proxy-token is configured, every request must present a matching token
// (see requestToken); mismatches and missing tokens are rejected with
// 401 authentication_error before any routing happens.
//
// With an empty token the request chain is returned unwrapped — no
// authentication at all. This preserves backward compatibility: the default
// localhost-only launch keeps working without any credentials.
func (h *proxyHandler) authMiddleware(next http.Handler) http.Handler {
	if h.cfg == nil || h.cfg.ProxyToken == "" {
		return next
	}
	want := h.cfg.ProxyToken
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestToken(r) != want {
			h.logf(i18n.T(i18n.MsgProxyAuthReject), r.Method, r.URL.Path)
			convert.WriteAnthropicError(w, http.StatusUnauthorized,
				"authentication_error", "invalid or missing API token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// logf writes a [proxy]-prefixed diagnostic line to h.logWriter, but only
// when -v is set (or drops it silently otherwise, including when no writer
// was configured).
//
// Routing the lines through logWriter rather than os.Stderr directly keeps
// Claude Code's interactive TUI clean: with -quiet-server on a TTY the sink
// is the llama-server log file, and when output is piped it is stderr —
// either way never into the live terminal UI.
func (h *proxyHandler) logf(format string, args ...any) {
	if h.cfg == nil || !h.cfg.Verbose || h.logWriter == nil {
		return
	}
	fmt.Fprintf(h.logWriter, "[proxy] "+format+"\n", args...)
}

// convert runs the Anthropic→OpenAI request conversion with options derived
// from cfg (image blocks are only mapped when a vision projector is
// configured via -mmproj) and logs the result under -v: a one-line summary of
// the converted request, a preview of each role=tool message, and a note
// listing dropped content blocks (thinking has no OpenAI equivalent; image
// needs -mmproj). The conversion statistics make silent data loss visible.
func (h *proxyHandler) convert(req map[string]any) (map[string]any, error) {
	opts := convert.ConvertOptions{ImagesToOpenAI: h.cfg != nil && h.cfg.Mmproj != ""}
	out, st, err := convert.ConvertRequest(req, opts)
	if err != nil {
		return out, err
	}
	h.logf(i18n.T(i18n.MsgProxyUpstream), convert.RequestSummary(out))
	for _, line := range convert.ToolMessagesPreview(out) {
		h.logf("%s", line)
	}
	if st.Dropped() > 0 {
		h.logf(i18n.T(i18n.MsgProxyDropped), st.Summary())
	}
	return out, err
}
