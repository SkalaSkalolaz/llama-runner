package proxy

import (
	"context"
	"errors"
)

// Swapper is the interface between the proxy and whatever owns the local
// llama-server lifecycle. In production it is app.profileSwapper; tests
// inject mocks. A nil Swapper (or NoopSwapper) disables swapping entirely:
// every request is forwarded to the single server, exactly as before.
type Swapper interface {
	// SwapTo switches the active llama-server to the profile whose alias is
	// given, returning the new base URL (no trailing slash). The old server
	// is drained, stopped, and the new one is started and waited-ready
	// before this returns.
	SwapTo(ctx context.Context, alias string) (baseURL string, err error)
	// CurrentAlias is the alias of the currently active server ("" if none).
	CurrentAlias() string
	// CatalogAliases lists every alias the swapper can launch. Used to
	// decide whether an incoming model name triggers a swap.
	CatalogAliases() []string
	// IncInflight / DecInflight bracket the processing of one request whose
	// swap check has already passed — WaitInflight must not wait for it.
	IncInflight()
	DecInflight()
	// WaitInflight blocks until no request is in flight or ctx is done.
	WaitInflight(ctx context.Context) error
}

// NoopSwapper is a no-op implementation for the single-model case.
type NoopSwapper struct{}

func (NoopSwapper) SwapTo(context.Context, string) (string, error) {
	return "", errSwappingDisabled
}
func (NoopSwapper) CurrentAlias() string               { return "" }
func (NoopSwapper) CatalogAliases() []string           { return nil }
func (NoopSwapper) IncInflight()                       {}
func (NoopSwapper) DecInflight()                       {}
func (NoopSwapper) WaitInflight(context.Context) error { return nil }

var errSwappingDisabled = errors.New("swapping disabled")
