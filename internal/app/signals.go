package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// shutdown is the single place that knows how to stop the three kinds of
// child processes, because each needs a different signalling strategy:
//
//   - claude — signalled on its PID directly, never via a process group:
//     Claude Code reads the terminal interactively, and putting it in its own
//     group (non-foreground pgid) makes terminal reads raise SIGTTIN and hang
//     the UI (red zone #1);
//   - server — signalled to its process group (started with Setpgid), so any
//     helper processes llama-server spawns die with it;
//   - mcp    — signalled to its process group as well (Setpgid).
//
// When server/mcp were started without Setpgid (older tests do this), the
// signal falls back to the process directly. All setters and signal
// operations are mutex-guarded, so the signal-handler goroutine and run()
// can touch the struct concurrently.
type shutdown struct {
	mu     sync.Mutex
	claude *exec.Cmd
	server *exec.Cmd
	mcp    *exec.Cmd
}

// NewShutdown returns an empty shutdown coordinator with no children
// registered yet.
func NewShutdown() *shutdown { return &shutdown{} }

// SetClaude registers the Claude Code process. It must be called right after
// Start so a signal arriving immediately afterwards still reaches it.
func (s *shutdown) SetClaude(cmd *exec.Cmd) {
	s.mu.Lock()
	s.claude = cmd
	s.mu.Unlock()
}

// SetServer registers the llama-server process.
func (s *shutdown) SetServer(cmd *exec.Cmd) {
	s.mu.Lock()
	s.server = cmd
	s.mu.Unlock()
}

// SetMCP registers the mcp-search-server process.
func (s *shutdown) SetMCP(cmd *exec.Cmd) {
	s.mu.Lock()
	s.mcp = cmd
	s.mu.Unlock()
}

// signal sends sig to every registered child: claude on its PID directly,
// server and mcp to their process groups when they were started with
// Setpgid and to the PID otherwise (see the type doc for why). Unregistered
// or not-yet-started processes are skipped; signalling errors (e.g. an
// already-exited child) are ignored.
func (s *shutdown) signal(sig syscall.Signal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claude != nil && s.claude.Process != nil {
		_ = s.claude.Process.Signal(sig)
	}
	if s.server != nil && s.server.Process != nil {
		if s.server.SysProcAttr != nil && s.server.SysProcAttr.Setpgid {
			_ = syscall.Kill(-s.server.Process.Pid, sig)
		} else {
			_ = s.server.Process.Signal(sig)
		}
	}
	if s.mcp != nil && s.mcp.Process != nil {
		if s.mcp.SysProcAttr != nil && s.mcp.SysProcAttr.Setpgid {
			_ = syscall.Kill(-s.mcp.Process.Pid, sig)
		} else {
			_ = s.mcp.Process.Signal(sig)
		}
	}
}

// StopAll sends SIGINT to all registered children — the first, graceful
// stop attempt.
func (s *shutdown) StopAll() { s.signal(syscall.SIGINT) }

// Escalate re-signals all children with a harder signal (SIGTERM, SIGKILL)
// when a previous one did not stop them in time.
func (s *shutdown) Escalate(sig syscall.Signal) { s.signal(sig) }

// HandleSignals runs the Ctrl-C/Terminate shutdown dance, in its own
// goroutine, escalating until the children are gone:
//
//	first signal  → SIGINT to everything;
//	no exit within timeout (or a repeated signal, which skips the wait)
//	              → SIGTERM;
//	still nothing (or another signal) → SIGKILL.
//
// Repeated signals are never lost or merely echoed — each one pulls the
// escalation forward immediately (defect #12). Progress is announced on
// stdout. The goroutine returns without acting when done is closed (run()
// finished normally and no longer needs it) or when sigCh is closed.
func HandleSignals(sigCh <-chan os.Signal, done <-chan struct{}, shut *shutdown,
	stdout io.Writer, timeout time.Duration) {

	var sig os.Signal
	select {
	case <-done:
		return
	case s, ok := <-sigCh:
		if !ok {
			return
		}
		sig = s
	}
	fmt.Fprint(stdout, i18n.TF(i18n.MsgSignalReceived, sig))
	shut.StopAll()

	for _, esc := range []struct {
		sig  syscall.Signal
		name string
	}{
		{syscall.SIGTERM, "SIGTERM"},
		{syscall.SIGKILL, "SIGKILL"},
	} {
		select {
		case <-done:
			return
		case s, ok := <-sigCh:
			if !ok {
				return
			}
			fmt.Fprint(stdout, i18n.TF(i18n.MsgSignalRepeatEscalate, s))
		case <-time.After(timeout):
			fmt.Fprint(stdout, i18n.TF(i18n.MsgEscalation, esc.name))
		}
		shut.Escalate(esc.sig)
	}
}
