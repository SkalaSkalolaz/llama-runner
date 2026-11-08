Phase B.2 — Swapper interface and profileSwapper

Define the interface between the proxy and the server manager, and
implement the real swapper.
Files you may edit

    internal/proxy/swap.go (new, up to 100 lines)

    internal/proxy/swap_test.go (new, up to 80 lines)

    internal/app/swapper.go (new, up to 180 lines)

    internal/app/swapper_test.go (new, up to 200 lines)

Tasks

B.2.1 — proxy.Swapper interface

In internal/proxy/swap.go:

type Swapper interface {
// SwapTo switches the active llama-server to the profile whose alias
// is given. Returns the new base URL and nil on success. The old
// server is stopped before the new one starts; on error, no server
// is running.
SwapTo(ctx context.Context, alias string) (baseURL string, err error)

// CurrentAlias returns the alias of the currently active server.
CurrentAlias() string

// CatalogAliases returns all known aliases (for validation of
// incoming model names). Empty slice if no catalog (non-auto mode).
CatalogAliases() []string

// WaitInflight blocks until in-flight request count drops to zero
// or ctx is cancelled.
WaitInflight(ctx context.Context) error
}

// NoopSwapper is used outside -auto mode. All methods return errors or no-op.
type NoopSwapper struct{}

NoopSwapper behaviour:

    SwapTo -> error "swapping not supported (auto mode is off)".

    CurrentAlias -> "".

    CatalogAliases -> nil.

    WaitInflight -> nil immediately.

B.2.2 — profileSwapper

In internal/app/swapper.go:

type profileSwapper struct {
mgr ServerManager
profiles map[string]config.Config // alias -> merged config
home string
stdout io.Writer
stderr io.Writer

mu sync.Mutex
currentAlias string
inflight int64 // atomic
idle *sync.Cond
}

SwapTo(ctx, alias):

    Lock.

    If alias == currentAlias -> return current base URL, nil.

    WaitInflight(ctx) with 30s timeout (if exceeded -> error).

    Get profileCfg := profiles[alias]; missing -> error.

    spec, code := SpecForConfig(profileCfg, home, stdout, stderr);
    code != 0 -> error.

    newSpec, err := mgr.SwapTo(spec); err -> error, set currentAlias="".

    Update currentAlias = alias.

    Return ServerBaseURL(profileCfg), nil.

IncInflight() / DecInflight() — public methods the proxy calls around
each request.

WaitInflight(ctx) — waits on idle condvar until inflight == 0.

B.2.3 — Tests

    TestNoopSwapper_AllMethods — errors/no-ops.

    TestProfileSwapper_SwapTo_Success — two profiles via recordingRunner, swap works.

    TestProfileSwapper_SwapTo_SameAlias_Noop — no restart.

    TestProfileSwapper_SwapTo_UnknownAlias — error.

    TestProfileSwapper_WaitInflight — inc/dec counter, wait returns.

    TestProfileSwapper_WaitInflight_Timeout — ctx expires -> error.

Acceptance

    go test -race ./internal/proxy/... ./internal/app/... ok.

    No existing test changes needed.