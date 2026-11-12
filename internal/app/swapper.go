package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/proxy"
)

// profileSwapper implements proxy.Swapper on top of ServerManager. It holds
// the merged config of every catalog profile, the current alias, and an
// in-flight counter used to drain requests before a swap. All swaps are
// serialized by mu: a second caller sees currentAlias already changed and
// returns immediately.
type profileSwapper struct {
	mgr      *ServerManager
	profiles map[string]*config.Config
	home     string
	logSink  io.Writer
	stdout   io.Writer
	stderr   io.Writer

	mu           sync.Mutex
	currentAlias string
	stopped      bool

	inflight atomic.Int64
}

// NewProfileSwapper builds a swapper for the given catalog. currentAlias
// must match the alias the server is currently running with (cfg.Alias
// after ResolveAlias).
func NewProfileSwapper(mgr *ServerManager, profiles map[string]*config.Config,
	currentAlias, home string, logSink, stdout, stderr io.Writer) *profileSwapper {
	return &profileSwapper{
		mgr: mgr, profiles: profiles,
		currentAlias: currentAlias, home: home,
		logSink: logSink, stdout: stdout, stderr: stderr,
	}
}

func (s *profileSwapper) CurrentAlias() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentAlias
}

func (s *profileSwapper) CatalogAliases() []string {
	out := make([]string, 0, len(s.profiles))
	for a := range s.profiles {
		out = append(out, a)
	}
	return out
}

// CurrentURL returns the base URL of the running server, or "" when no
// server is loaded. It derives the URL from the profile's own host:port
// through ServerBaseURL, so the answer matches what ServerManager started
// the process with — including the fresh port StopAndStart allocated.
//
// The mutex is the same one SwapTo holds for its whole duration (drain,
// stop, start, health wait — up to minutes). A proxy request arriving
// during a swap therefore blocks until the swap completes and then sees
// the new URL; blocking is deliberate, sending the request to a
// half-torn-down server is worse.
func (s *profileSwapper) CurrentURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentAlias == "" {
		return ""
	}
	cfg, ok := s.profiles[s.currentAlias]
	if !ok || cfg == nil {
		return ""
	}
	return ServerBaseURL(cfg)
}

func (s *profileSwapper) IncInflight() { s.inflight.Add(1) }
func (s *profileSwapper) DecInflight() { s.inflight.Add(-1) }

// WaitInflight polls the counter until zero or ctx done. 50ms is a fine
// granularity for a swap (which waits on human timescales anyway).
func (s *profileSwapper) WaitInflight(ctx context.Context) error {
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	for {
		if s.inflight.Load() == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// Stop permanently disables swapping and kills the running server. Called
// from run()'s cleanup so a swap in flight cannot race with process exit.
// Nil-safe: a swapper built without a manager (tests, defensive
// construction) still flips the stopped flag so later SwapTo calls fail
// cleanly instead of racing.
func (s *profileSwapper) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	if s.mgr != nil {
		_ = s.mgr.Stop()
	}
	s.currentAlias = ""
}

func (s *profileSwapper) SwapTo(ctx context.Context, alias string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return "", errors.New("swapper stopped")
	}
	alias = strings.TrimSpace(alias)
	if strings.EqualFold(alias, s.currentAlias) {
		if cfg, ok := s.profiles[alias]; ok {
			return ServerBaseURL(cfg), nil
		}
		return "", fmt.Errorf("unknown alias %q", alias)
	}
	cfg, ok := s.profiles[alias]
	if !ok {
		return "", fmt.Errorf("unknown alias %q", alias)
	}
	prevAlias := s.currentAlias
	// prevCfg, hasPrev := s.profiles[prevAlias]

	drainCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := s.WaitInflight(drainCtx); err != nil {
		return "", fmt.Errorf("drain: %w", err)
	}

	if s.mgr == nil {
		return "", errors.New("swapper has no server manager")
	}

	spec, code := SpecForConfig(cfg, s.home, s.stdout, s.stderr)
	if code != 0 {
		return "", fmt.Errorf("profile %q cannot launch (code %d)", alias, code)
	}
	spec.Log = s.logSink

	newSpec, err := s.mgr.SwapTo(spec)
	if err != nil {
		if prevCfg, ok := s.profiles[prevAlias]; ok {
			if prevSpec, pcode := SpecForConfig(prevCfg, s.home, s.stdout, s.stderr); pcode == 0 {
				prevSpec.Log = s.logSink
				if _, rerr := s.mgr.SwapTo(prevSpec); rerr == nil {
					s.currentAlias = prevAlias
					return "", fmt.Errorf("swap to %q failed (%v); restored %q", alias, err, prevAlias)
				}
			}
		}
		s.currentAlias = ""
		return "", err
	}

	s.currentAlias = alias
	return newSpec.BaseURL, nil
}

var _ proxy.Swapper = (*profileSwapper)(nil)
