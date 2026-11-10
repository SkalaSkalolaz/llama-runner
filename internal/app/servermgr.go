package app

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/cli"
	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/proc"
	"github.com/SkalaSkalolaz/llama-runner/internal/runner"
)

// This module owns one responsibility: the lifecycle of a llama-server child
// process. A ServerSpec is a fully resolved launch description produced from
// a config; the ServerManager starts an instance from a spec, keeps track of
// the running one and registers it with the shutdown coordinator. The same
// pair serves the initial launch and — this is why it was extracted from
// run() — a later relaunch with another profile while Claude Code keeps
// running: switching models means Stop-ing the current instance and Start-ing
// a spec built from the profile's merged config.

// ServerSpec is the resolved, launch-ready description of one llama-server
// instance: the verified binary and command line, the working directory, the
// log target and the health probe to wait on after Start.
type ServerSpec struct {
	Bin     string
	Args    []string
	WorkDir string
	Log     io.Writer
	// BaseURL is the http base URL used to reach the server from the
	// launcher (e.g. "http://127.0.0.1:8080", no trailing slash).
	BaseURL string
	// HealthURL is BaseURL + "/health".
	HealthURL string
	// Port is the TCP port the server listens on, kept separately from
	// Args so SwapTo can rewrite it to a fresh free port without string
	// parsing. Informational for the initial launch.
	Port         string
	ReadyTimeout time.Duration
	Verbose      bool
}

// SpecForConfig turns a resolved configuration — the root config, or a copy
// of it with a profile merged in — into a ServerSpec: paths are verified and
// the llama-server command line is built exactly the way run() used to do it
// inline. User-facing diagnostics go to stdout/stderr as before.
//
// The int return follows run's exit-code convention: 0 on success, and the
// ResolvePaths/resolveServerWorkDir severities (1 — a required file is missing,
// 2 — a path cannot even be resolved) otherwise. A relaunching caller (the
// model switch) treats any non-zero as "this profile cannot start".
func SpecForConfig(cfg *config.Config, home string, stdout, stderr io.Writer) (ServerSpec, int) {
	paths, code := ResolvePaths(cfg, home, stdout, stderr)
	if code != 0 {
		return ServerSpec{}, code
	}
	workDir, wdCode := resolveServerWorkDir(cfg, home, stderr)
	if wdCode != 0 {
		return ServerSpec{}, wdCode
	}
	base := ServerBaseURL(cfg)
	return ServerSpec{
		Bin: paths.ServerBin,
		Args: cli.BuildServerArgs(cfg, cli.ServerPaths{
			Model: paths.Model, Mmproj: paths.Mmproj,
			Draft: paths.Draft, Template: paths.Template,
		}),
		WorkDir:      workDir,
		BaseURL:      base,
		HealthURL:    base + "/health",
		Port:         cfg.Port,
		ReadyTimeout: cfg.ReadyTimeout,
		Verbose:      cfg.Verbose,
	}, 0
}

// ServerBaseURL derives the http base URL for reaching llama-server from the
// launcher side: a wildcard bind host ("", 0.0.0.0, ::) is polled through
// loopback, since the launcher and the server share the machine.
func ServerBaseURL(cfg *config.Config) string {
	host := cfg.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(strings.Trim(host, "[]"), cfg.Port)
}

// ServerManager owns the llama-server child process: each Start replaces the
// tracked current instance, registers the new process with the shutdown
// coordinator immediately, and WaitReady polls the spec's health endpoint.
// The manager is NOT safe for concurrent Start calls — serialization is the
// caller's concern (the model switch will hold a mutex around swaps).
type ServerManager struct {
	runner  runner.CommandRunner
	shut    *shutdown
	stdout  io.Writer
	stderr  io.Writer
	mu      sync.RWMutex
	current *exec.Cmd
	tracked *proc.Tracked
}

// NewServerManager creates a manager that starts processes with cmdRunner,
// registers them with shut and inherits stdout/stderr when a spec names no
// log target.
func NewServerManager(cmdRunner runner.CommandRunner, shut *shutdown, stdout, stderr io.Writer) *ServerManager {
	return &ServerManager{runner: cmdRunner, shut: shut, stdout: stdout, stderr: stderr}
}

// Start launches the instance described by spec: its own process group
// (Setpgid — red zone #1, see the shutdown type: the server never reads the
// terminal, and the group lets the coordinator take any helper processes
// down with it), LD_LIBRARY_PATH pointed at the sibling lib/ directory, the
// spec's working directory, and logs per spec. After a successful Start the
// new instance is the current one and is registered with the shutdown
// coordinator right away, so a signal arriving before the health wait still
// reaches it.
func (m *ServerManager) Start(spec ServerSpec) error {
	cmd := m.runner.Command(spec.Bin, spec.Args...)
	setProcAttr(cmd)
	ApplyLdLibraryPath(cmd, spec.Bin, m.stdout, spec.Verbose)
	if spec.WorkDir != "" {
		cmd.Dir = spec.WorkDir
	}
	if spec.Log != nil {
		cmd.Stdout = spec.Log
		cmd.Stderr = spec.Log
	} else {
		cmd.Stdout = m.stdout
		cmd.Stderr = m.stderr
	}
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return err
	}
	m.current = cmd
	m.tracked = proc.Track(cmd)
	m.shut.SetServer(cmd)
	return nil
}

// WaitReady polls spec.HealthURL until the server answers, the process
// exits, or ReadyTimeout elapses.
func (m *ServerManager) WaitReady(spec ServerSpec) error {
	m.mu.RLock()
	tracked := m.tracked
	m.mu.RUnlock()
	if tracked == nil {
		return errors.New("server is not started")
	}
	return proc.WaitForServerProcess(spec.HealthURL, spec.ReadyTimeout, tracked)
}

func (m *ServerManager) Current() *proc.Tracked {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tracked
}

func (m *ServerManager) CurrentCmd() *exec.Cmd {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Stop terminates the current llama-server: SIGTERM to its process group,
// wait up to 5s for a clean exit, escalate to SIGKILL, then clear the
// tracked state. Safe before the first Start and idempotent after a
// successful Stop.
func (m *ServerManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cmd, tracked := m.current, m.tracked
	if cmd == nil || cmd.Process == nil {
		m.current, m.tracked = nil, nil
		m.shut.SetServer(nil)
		return nil
	}
	signalChild(cmd, syscall.SIGTERM)
	if tracked != nil {
		select {
		case <-tracked.Done:
		case <-time.After(15 * time.Second):
			signalChild(cmd, syscall.SIGKILL)
			select {
			case <-tracked.Done:
			case <-time.After(2 * time.Second):
			}
		}
	}
	m.current, m.tracked = nil, nil
	m.shut.SetServer(nil)
	return nil
}

// StopAndStart replaces the current server with one built from spec on a
// fresh free port (avoids TIME_WAIT races on the old port) and returns the
// spec that is now running. Does NOT wait for readiness; see SwapTo.
func (m *ServerManager) StopAndStart(spec ServerSpec) (ServerSpec, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return ServerSpec{}, fmt.Errorf("allocate port: %w", err)
	}
	newPort := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close()

	newSpec := rewriteSpecPort(spec, newPort)
	if err := m.Stop(); err != nil {
		return ServerSpec{}, fmt.Errorf("stop old server: %w", err)
	}
	if err := m.Start(newSpec); err != nil {
		return ServerSpec{}, fmt.Errorf("start new server: %w", err)
	}
	return newSpec, nil
}

// SwapTo replaces the current server with one built from spec and waits
// for it to become ready. On any error the old server is already stopped
// and no new server is guaranteed to be running.
func (m *ServerManager) SwapTo(spec ServerSpec) (ServerSpec, error) {
	newSpec, err := m.StopAndStart(spec)
	if err != nil {
		return ServerSpec{}, err
	}
	if err := m.WaitReady(newSpec); err != nil {
		_ = m.Stop()
		return ServerSpec{}, fmt.Errorf("wait ready: %w", err)
	}
	return newSpec, nil
}

// rewriteSpecPort returns a copy of spec with --port rewritten to newPort
// and BaseURL/HealthURL recomputed against it.
func rewriteSpecPort(spec ServerSpec, newPort string) ServerSpec {
	args := make([]string, len(spec.Args))
	copy(args, spec.Args)
	for i := 1; i < len(args); i++ {
		if args[i-1] == "--port" {
			args[i] = newPort
		}
	}
	host := "127.0.0.1"
	if u, err := url.Parse(spec.BaseURL); err == nil {
		if h := u.Hostname(); h != "" && h != "0.0.0.0" && h != "::" {
			host = h
		}
	}
	base := "http://" + net.JoinHostPort(host, newPort)
	spec.Args = args
	spec.Port = newPort
	spec.BaseURL = base
	spec.HealthURL = base + "/health"
	return spec
}
