package app

import (
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
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
	// Bin is the absolute llama-server binary path.
	Bin string
	// Args is the complete command line (without the binary).
	Args []string
	// WorkDir is the absolute working directory for the process
	// ("" means: inherit the launcher's cwd).
	WorkDir string
	// Log receives the process stdout/stderr; nil means inherit the
	// manager's streams.
	Log io.Writer
	// HealthURL is polled by WaitReady (normally <base>/health).
	HealthURL string
	// ReadyTimeout bounds the health wait.
	ReadyTimeout time.Duration
	// Verbose enables the launcher's -v diagnostics around the start.
	Verbose bool
}

// SpecForConfig turns a resolved configuration — the root config, or a copy
// of it with a profile merged in — into a ServerSpec: paths are verified and
// the llama-server command line is built exactly the way run() used to do it
// inline. User-facing diagnostics go to stdout/stderr as before.
//
// The int return follows run's exit-code convention: 0 on success, and the
// ResolvePaths/SetWorkDir severities (1 — a required file is missing,
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
	return ServerSpec{
		Bin: paths.ServerBin,
		Args: cli.BuildServerArgs(cfg, cli.ServerPaths{
			Model: paths.Model, Mmproj: paths.Mmproj,
			Draft: paths.Draft, Template: paths.Template,
		}),
		WorkDir:      workDir,
		HealthURL:    ServerBaseURL(cfg) + "/health",
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

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
	if m.tracked == nil {
		return errors.New("server is not started")
	}
	return proc.WaitForServerProcess(spec.HealthURL, spec.ReadyTimeout, m.tracked)
}

// Current returns the tracked process of the running instance, or nil before
// the first successful Start.
func (m *ServerManager) Current() *proc.Tracked { return m.tracked }

// CurrentCmd returns the *exec.Cmd of the running instance (nil before the
// first Start). A swapping caller keeps it to stop the old process once the
// replacement is healthy.
func (m *ServerManager) CurrentCmd() *exec.Cmd { return m.current }
