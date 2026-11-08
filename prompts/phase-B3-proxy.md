Phase B.3 — Proxy integration: in-flight drain, swap on model change

Wire the Swapper into the proxy handler. When a request names a profile
different from the current one, swap before passing through. Track in-flight
requests so swaps can drain cleanly.
Files you may edit

    internal/proxy/proxy.go (+ up to 25 lines)

    internal/proxy/handlers.go (+ up to 50 lines)

    internal/proxy/passthrough.go (+ up to 15 lines)

    internal/proxy/handlers_test.go (extend, + up to 80 lines)

    internal/proxy/passthrough_test.go (extend, + up to 40 lines)

    internal/i18n/messages.go (+ up to 5 keys)

Tasks

B.3.1 — Add swapper to proxyHandler

Field: swapper Swapper (nil-safe everywhere).

B.3.2 — Wrap handlers with in-flight tracking

In newProxyMux, wrap each handler:

tracked := func(h http.HandlerFunc) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
if h.swapper != nil {
h.swapper.IncInflight()
defer h.swapper.DecInflight()
}
h(w, r)
}
}

Apply to /v1/messages only (longest requests); other routes are short.

B.3.3 — Swap on model change

In handleMessages, after parsing model and before the isLocal check:

if h.swapper != nil && h.externalMode() {
aliases := h.swapper.CatalogAliases()
for _, a := range aliases {
if strings.EqualFold(strings.TrimSpace(model), a) &&
a != h.swapper.CurrentAlias() {
newURL, err := h.swapper.SwapTo(r.Context(), a)
if err != nil {
convert.WriteAnthropicError(w, http.StatusBadGateway,
"api_error", fmt.Sprintf("swap to %s failed: %v", a, err))
return
}
h.llamaServerURL = newURL // guarded by h.mu — add one
break
}
}
}

Note: h.llamaServerURL becomes mutable. Wrap in h.mu sync.RWMutex.
passthrough and forwardToOpenAI read it under h.mu.RLock().

B.3.4 — Tests

    TestProxy_Swapper_SwapOnModelChange — mock Swapper, request with alias B
    while current A -> SwapTo(B) called.

    TestProxy_Swapper_SameAlias_NoSwap — alias current -> no call.

    TestProxy_Swapper_UnknownAlias_NoSwap — model not in catalog -> forward
    normally (to external or passthrough).

    TestProxy_Swapper_SwapFails_Returns502 — SwapTo error -> 502.

    TestProxy_Swapper_NilSwapper_Unchanged — existing tests pass with
    swapper == nil.

Acceptance

    All existing handlers_test.go, passthrough_test.go tests pass without edits.

    New tests pass with -race.