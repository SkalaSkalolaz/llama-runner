package proxy

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

// requestToken extracts the client-supplied API token from the request
// headers, checking Authorization: Bearer first and then x-api-key.
// Different Claude Code versions use one or the other scheme, so the proxy
// accepts both. Returns "" when no token is present in either header.
func requestToken(r *http.Request) string {
	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
		return strings.TrimPrefix(a, "Bearer ")
	}
	if k := r.Header.Get("x-api-key"); k != "" {
		return k
	}
	return ""
}

// isLoopbackHost reports whether the host part of a host:port listener
// address refers to the local machine: the literal "localhost", or any IP
// literal in the loopback ranges (127.0.0.0/8, ::1). A value that cannot be
// split into host and port, or an empty host, counts as non-loopback so the
// caller errs on the side of warning. Used by the "proxy exposed to the
// network" safety check.
func isLoopbackHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil || host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ShouldWarnProxyExposure reports whether starting the proxy is worth a
// security warning: it listens on a non-loopback address, no authentication
// token is configured, and the user has not opted in with
// -proxy-allow-remote. In that combination anyone on the network could use
// the proxy — and through it the paid external provider key.
//
// This only drives a stderr warning; it is deliberately not a hard failure,
// since binding a reachable address may be exactly what the user wants.
func ShouldWarnProxyExposure(proxyAddr string, cfg *config.Config) bool {
	if cfg == nil || isLoopbackHost(proxyAddr) {
		return false
	}
	return cfg.ProxyToken == "" && !cfg.ProxyAllowRemote
}

// newProxyClient builds the http.Client used for all upstream requests.
//
// Client.Timeout is 0 on purpose: a whole-request timeout would cut long
// generations off mid-stream after a fixed period (defect #8). Protection
// against a dead upstream is instead applied where it belongs —
// Transport.ResponseHeaderTimeout of 30 seconds, which bounds only the wait
// for response headers while leaving the stream body open as long as needed.
// Abandoning an in-flight request is driven by cancellation of the incoming
// request context (NewRequestWithContext with r.Context() in
// forwardToLlama/forwardToOpenAI).
//
// The transport is a Clone of http.DefaultTransport with that single field
// changed, preserving environment proxy settings, HTTP/2 and keep-alives.
func newProxyClient() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{Timeout: 0, Transport: t}
}
