package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/cli"
	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/proxy"
)

// ModelSwitch changes which GGUF the local llama-server loads while Claude
// Code keeps running — the mechanism the Claude Code /model menu drives. It
// implements proxy.ModelSwitcher: the proxy sees only the model name of the
// next request, and when that name means "another local model", the switch
// happens right there: the current llama-server is stopped (SIGTERM →
// SIGKILL escalation), a new one starts with the chosen profile's fully
// resolved arguments on the same host:port, and the request is forwarded
// once /health passes. Claude Code just experiences a slow first message.
//
// A failed switch rolls back: the half-started instance is stopped and the
// previous spec is started again, so a broken profile cannot leave the
// local route dead.
//
// One switch at a time: the mutex serialises swaps; read-only queries use
// the read lock so model listing never waits on a reload in progress.

const (
	// switchStopTimeout is how long the current server gets to exit
	// gracefully before SIGKILL, mirroring the shutdown coordinator's
	// escalation idea on a smaller scale.
	switchStopTimeout = 15 * time.Second
	// switchKillTimeout bounds the wait after SIGKILL.
	switchKillTimeout = 10 * time.Second
)

// compile-time proof that the launcher's switch satisfies the proxy hook.
var _ proxy.ModelSwitcher = (*ModelSwitch)(nil)

// ModelSwitch is the launcher-side implementation of proxy.ModelSwitcher.
type ModelSwitch struct {
	mu   sync.RWMutex
	mgr  *ServerManager
	base *config.Config // launch config: root section + selected profile + CLI
	home string
	diag io.Writer // diagnostics (SpecForConfig output, switch progress)

	// stopTimeout is a test seam for the stop escalation wait.
	stopTimeout time.Duration

	// origName/origSpec describe the model as it was launched (its resolved
	// alias and the spec that started it); the menu's "local default
	// (running)" entry carries origName, so switching back is switching to
	// this spec.
	origName string
	origSpec ServerSpec
	// curName/curSpec are what the server actually serves right now; curName
	// is also the alias the upstream expects in forwarded requests.
	curName string
	curSpec ServerSpec
}

// NewModelSwitch creates a switch whose current model is the one just
// launched: cfg.Alias (resolved by Run before launch) under launchedSpec.
// diag receives switch progress and SpecForConfig diagnostics — normally the
// proxy log sink, so switching never pollutes Claude Code's TUI.
func NewModelSwitch(cfg *config.Config, launchedSpec ServerSpec, mgr *ServerManager, home string, diag io.Writer) *ModelSwitch {
	if diag == nil {
		diag = io.Discard
	}
	name := cfg.Alias
	return &ModelSwitch{
		mgr: mgr, base: cfg, home: home, diag: diag,
		stopTimeout: switchStopTimeout,
		origName:    name, origSpec: launchedSpec,
		curName: name, curSpec: launchedSpec,
	}
}

// Handles reports whether model names a local model the switch can serve:
// the currently loaded one, the as-launched one (the menu's local default),
// or any profile's display name. Comparison is case- and space-insensitive,
// matching the proxy's routing token convention.
func (s *ModelSwitch) Handles(model string) bool {
	if strings.TrimSpace(model) == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if sameModelName(s.curName, model) || sameModelName(s.origName, model) {
		return true
	}
	for _, p := range s.base.Profiles {
		if sameModelName(cli.ProfileDisplayName(p), model) {
			return true
		}
	}
	return false
}

// ModelNames lists every model identifier the local route can serve, in a
// stable order: the as-launched model first, then the profiles in file
// order, then the current model if it is neither (a switched-in alias).
func (s *ModelSwitch) ModelNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	seen := map[string]bool{}
	add := func(name string) {
		if name == "" || seen[strings.ToLower(name)] {
			return
		}
		seen[strings.ToLower(name)] = true
		out = append(out, name)
	}
	add(s.origName)
	for _, p := range s.base.Profiles {
		add(cli.ProfileDisplayName(p))
	}
	add(s.curName)
	return out
}

// EnsureLoaded makes the local llama-server serve model and returns the
// alias the upstream expects. Calling it with the already-loaded model is a
// no-op. A switch stops the current server, starts the target profile's
// fully resolved spec on the same address and waits for /health — up to the
// profile's readyTimeout, which is why the first message after a switch is
// slow. On any failure the previous instance is restored and the error
// returned.
//
// The ctx is intentionally not consulted: aborting half-way through a
// restart would leave the local route in an undefined state, so once a
// switch begins it runs to completion (success or rollback).
func (s *ModelSwitch) EnsureLoaded(_ context.Context, model string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sameModelName(s.curName, model) {
		return s.curName, nil
	}

	target, spec, err := s.resolveSpec(model)
	if err != nil {
		return "", err
	}

	s.logf("переключение локальной модели %q → %q (перезагрузка сервера)", s.curName, target)
	prevName, prevSpec := s.curName, s.curSpec

	if err := s.stopCurrent(); err != nil {
		return "", fmt.Errorf("не удалось остановить текущий llama-server: %w", err)
	}
	if err := s.mgr.Start(spec); err != nil {
		s.rollback(prevName, prevSpec)
		return "", fmt.Errorf("запуск профиля не удался: %w", err)
	}
	if err := s.mgr.WaitReady(spec); err != nil {
		s.rollback(prevName, prevSpec)
		return "", fmt.Errorf("llama-server с моделью %s не стал здоров: %w", target, err)
	}

	s.curName, s.curSpec = target, spec
	s.logf("локальная модель: %s (готова)", target)
	return s.curName, nil
}

// resolveSpec maps a menu model name to the spec that would serve it: the
// as-launched spec for the local default, or a fresh spec built from the
// profile's overlay applied onto the launch config (so per-profile context
// size, chat template and everything else are honoured). The profile's
// display name becomes the server alias, matching what the menu shows and
// what the proxy rewrites requests to.
func (s *ModelSwitch) resolveSpec(model string) (string, ServerSpec, error) {
	if sameModelName(s.origName, model) {
		return s.origName, s.origSpec, nil
	}
	for _, p := range s.base.Profiles {
		if !sameModelName(cli.ProfileDisplayName(p), model) {
			continue
		}
		name := cli.ProfileDisplayName(p)
		newCfg, err := config.ApplyProfileOverlay(s.base, p.Name)
		if err != nil {
			return "", ServerSpec{}, err
		}
		newCfg.Alias = name
		spec, code := SpecForConfig(newCfg, s.home, s.diag, s.diag)
		if code != 0 {
			return "", ServerSpec{}, fmt.Errorf("профиль %q нельзя запустить (код %d, подробности в логе прокси)", p.Name, code)
		}
		// SpecForConfig builds a bare spec; the log sink is
		// attached to the launched one by run(). It must travel
		// over, or Start would inherit the launcher's
		// stdout/stderr — the terminal Claude Code's TUI renders
		// into — and every server log line would corrupt the
		// interface.
		spec.Log = s.origSpec.Log
		return name, spec, nil
	}
	return "", ServerSpec{}, fmt.Errorf("неизвестная локальная модель %q", model)
}

// rollback restores the instance that served requests before a failed
// switch. Failures are logged, not returned: the caller already has the
// primary error, and this is best-effort damage control.
func (s *ModelSwitch) rollback(prevName string, prevSpec ServerSpec) {
	if err := s.stopCurrent(); err != nil {
		s.logf("не удалось остановить недоподнятый экземпляр: %v", err)
	}
	if err := s.mgr.Start(prevSpec); err != nil {
		s.logf("откат на модель %q: запуск не удался: %v", prevName, err)
		return
	}
	if err := s.mgr.WaitReady(prevSpec); err != nil {
		s.logf("откат на модель %q: сервер не стал здоров: %v", prevName, err)
		return
	}
	s.logf("откат выполнен: локальная модель снова %q", prevName)
}

// stopCurrent shuts the tracked llama-server down with SIGTERM escalating
// to SIGKILL, and returns only after the process is gone — the replacement
// binds the same port, and an exited process releases it.
func (s *ModelSwitch) stopCurrent() error {
	cmd, tracked := s.mgr.CurrentCmd(), s.mgr.Current()
	if cmd == nil || cmd.Process == nil || tracked == nil {
		return nil
	}
	signalServerGroup(cmd, syscall.SIGTERM)
	select {
	case <-tracked.Done:
		return nil
	case <-time.After(s.stopTimeout):
	}
	s.logf("сервер не вышел за %v — SIGKILL", s.stopTimeout)
	signalServerGroup(cmd, syscall.SIGKILL)
	select {
	case <-tracked.Done:
		return nil
	case <-time.After(switchKillTimeout):
		return errors.New("llama-server не завершился даже после SIGKILL")
	}
}

// signalServerGroup signals the server's process group when it was started
// with Setpgid (red zone #1 — same rule as the shutdown coordinator), and
// the process directly otherwise.
func signalServerGroup(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd.SysProcAttr != nil && cmd.SysProcAttr.Setpgid {
		_ = syscall.Kill(-cmd.Process.Pid, sig)
		return
	}
	_ = cmd.Process.Signal(sig)
}

func (s *ModelSwitch) logf(format string, args ...any) {
	fmt.Fprintf(s.diag, "[switch] "+format+"\n", args...)
}

// sameModelName compares menu model names case- and surrounding-space
// insensitively, the way the proxy matches its routing tokens.
func sameModelName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
