package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/cli"
	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/doctor"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
	"github.com/SkalaSkalolaz/llama-runner/internal/proc"
	"github.com/SkalaSkalolaz/llama-runner/internal/proxy"
	"github.com/SkalaSkalolaz/llama-runner/internal/runner"
	"github.com/SkalaSkalolaz/llama-runner/internal/update"
)

// run is the whole program minus os.Exit: testable end to end with injected
// argument list, output streams and runner.CommandRunner. It returns the process
// exit code (0 success, 1 runtime/launcher failure, 2 bad configuration or
// flags; help and -version return 0).
//
// Flow:
//  1. parse flags (help → 0), apply the default alias, resolve the OpenAI
//     key only when an external provider was actually requested;
//  2. handle the one-shot modes: -doctor and -update exit here;
//  3. validate and resolve all paths, then build and start llama-server in
//     its own process group, with LD_LIBRARY_PATH pointed at the sibling
//     lib/ or lib64/ directory, the chosen working directory, and logs going
//     either to the rotating log file (-quiet-server) or to the inherited
//     streams; optionally start mcp-search-server the same way;
//  4. register everything with the shutdown coordinator and start the
//     escalating signal handler;
//  5. -server mode: wait for the server and exit with its status;
//     default mode: wait for /health, optionally start the local proxy when
//     an external provider is configured (routing llama-server vs OpenAI),
//     wait for the MCP server and write its config, then launch Claude Code
//     with the ANTHROPIC_* environment — reconnecting its stdio to /dev/tty
//     when the parent's streams were redirected. When Claude Code exits,
//     deferred cleanup stops server and MCP, and run returns.
func Run(args []string, stdout, stderr io.Writer, cmdRunner runner.CommandRunner, version string) int {
	cfg, err := cli.ParseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if cfg.VersionFlag {
		fmt.Fprintln(stdout, version)
		return 0
	}
	if !cfg.AliasSet && !cfg.ServerOnly {
		cfg.Alias = "local"
	}

	if cfg.ServerOnly && cfg.OpenAIBaseURL != "" {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgWarnOpenaiIgnored))
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgErrHomeDirShort, err))
		return 1
	}

	cfg.Home = home

	// We pick up the external provider's key ONLY if the external provider
	// is actually requested. Otherwise a developer with OPENAI_API_KEY in the environment
	// would get "-openai is required when using -key" on a local
	// launch (including `-update`), even though OpenAI isn't needed at all.
	if cfg.OpenAIBaseURL != "" || cfg.OpenAIModel != "" || cfg.OpenAIKey != "" {
		resolvedKey, keyErr := cli.ResolveOpenAIKey(cfg.OpenAIKey, os.Getenv, home, stderr)
		if keyErr != nil {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgErrResolved, keyErr))
			return 2
		}
		cfg.OpenAIKey = resolvedKey
	}

	if err := cli.ValidateConfig(cfg); err != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgErrResolved, err))
		return 2
	}
	if cfg.DoctorFlag {
		return doctor.RunDoctor(cfg, home, cmdRunner, version, stdout, stderr)
	}
	if cfg.UpdateFlag {
		srcAbs := config.ExpandHome(cfg.LlamaSrc, home)
		installAbs := config.ExpandHome(cfg.InstallPrefix, home)
		if err := update.UpdateLlamaCPP(cmdRunner, srcAbs, installAbs, cfg.CudaArch,
			cfg.WithCuda, cfg.Verbose, stdout); err != nil {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgErrUpdate, err))
			return 1
		}
		fmt.Fprint(stdout, i18n.TF(i18n.MsgUpdateSuccess))
		return 0
	}

	// --- Paths ---
	paths, code := ResolvePaths(cfg, home, stdout, stderr)
	if code != 0 {
		return code
	}

	// --- Build args ---
	serverArgs := cli.BuildServerArgs(cfg, cli.ServerPaths{
		Model: paths.Model, Mmproj: paths.Mmproj,
		Draft: paths.Draft, Template: paths.Template,
	})

	cmd := cmdRunner.Command(paths.ServerBin, serverArgs...)
	// Setpgid — only for server and mcp (red zone #1):
	// they don't read the terminal, the group is needed for group kill.
	// For claude — we do NOT set it (see shutdown.SetClaude).
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// --- LD_LIBRARY_PATH for llama-server ---
	ApplyLdLibraryPath(cmd, paths.ServerBin, stdout, cfg.Verbose)
	// --- Working directory ---
	wdCode := SetWorkDir(cmd, cfg, home, stderr)
	if wdCode != 0 {
		return wdCode
	}

	// --- Log file (Claude Code mode) ---
	sink, sinkCode := OpenServerLog(cfg, home, stdout, stderr)
	if sinkCode != 0 {
		return sinkCode
	}
	defer sink.Close()
	if sink.Writer != nil {
		cmd.Stdout = sink.Writer
		cmd.Stderr = sink.Writer
	} else {
		cmd.Stdout = stdout
		cmd.Stderr = stderr
	}
	cmd.Stdin = os.Stdin

	if cfg.Verbose {
		fmt.Fprint(stdout, i18n.TF(i18n.MsgWorkDirLine, cmd.Dir))
		fmt.Fprint(stdout, i18n.TF(i18n.MsgCommandLine, paths.ServerBin, strings.Join(serverArgs, " ")), "\n")
		if sink.Name != "" {
			fmt.Fprint(stdout, i18n.TF(i18n.MsgServerLogFile, sink.Name))
		}
	} else {
		fmt.Fprint(stdout, i18n.TF(i18n.MsgStartingServer, cfg.Host, cfg.Port))
		if sink.Name != "" {
			fmt.Fprint(stdout, i18n.TF(i18n.MsgServerLogFile, sink.Name))
		}
	}

	// --- mcp-search-server ---
	var mcpCmd *exec.Cmd
	if cfg.McpProxy {
		mcpCmd = cmdRunner.Command("mcp-search-server", "--addr", cfg.McpAddr)
		mcpCmd.Env = append(os.Environ(), "SEARCH_ENABLED=true")
		// Setpgid for mcp — same as for llama-server (red zone #1:
		// mcp terminal doesn't read, group is needed for group kill).
		mcpCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		if sink.Writer != nil {
			mcpCmd.Stdout = sink.Writer
			mcpCmd.Stderr = sink.Writer
		} else {
			mcpCmd.Stdout = stdout
			mcpCmd.Stderr = stderr
		}
		if cmd.Dir != "" {
			mcpCmd.Dir = cmd.Dir
		}
		if cfg.Verbose {
			fmt.Fprint(stdout, i18n.TF(i18n.MsgMcpCommandLine, cfg.McpAddr), "\n")
		}
		if startErr := mcpCmd.Start(); startErr != nil {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgErrMcpStart, startErr))
			// mcpCmd.Process == nil (Start failed), server not yet started,
			// nothing to stop.
			return 1
		}
		defer proc.StopMcp(mcpCmd)
	}

	// --- Launch llama-server ---
	if startErr := cmd.Start(); startErr != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgErrServerStart, startErr))
		if mcpCmd != nil && mcpCmd.Process != nil {
			_ = mcpCmd.Process.Signal(syscall.SIGINT)
			_ = mcpCmd.Wait()
		}
		return 1
	}
	serverProcess := proc.Track(cmd)

	// --- Centralized shutdown ---
	shut := NewShutdown()
	shut.SetServer(cmd)
	if mcpCmd != nil {
		shut.SetMCP(mcpCmd)
	}

	var claudeProcess *proc.Tracked
	cleanup := func() {
		shut.StopAll()
		if claudeProcess != nil {
			<-claudeProcess.Done
		}
		_ = serverProcess.Wait()
	}
	defer cleanup()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	sigDone := make(chan struct{})
	defer close(sigDone)
	go HandleSignals(sigCh, sigDone, shut, stdout, 15*time.Second)

	// =================================================================
	// MODE 1: -server — llama-server only
	// =================================================================
	if cfg.ServerOnly {
		waitErr := serverProcess.Wait()
		if waitErr != nil {
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				proc.StopMcp(mcpCmd)
				return exitErr.ExitCode()
			}
			fmt.Fprint(stderr, i18n.TF(i18n.MsgServerExitErr, waitErr))
			proc.StopMcp(mcpCmd)
			return 1
		}
		fmt.Fprint(stdout, i18n.TF(i18n.MsgServerDone))
		proc.StopMcp(mcpCmd)
		return 0
	}

	// =================================================================
	// MODE 2 (default): llama-server + Claude Code
	// =================================================================
	healthHost := cfg.Host
	if healthHost == "" || healthHost == "0.0.0.0" || healthHost == "::" {
		healthHost = "127.0.0.1"
	}
	baseURL := "http://" + net.JoinHostPort(strings.Trim(healthHost, "[]"), cfg.Port)
	healthURL := baseURL + "/health"

	fmt.Fprint(stdout, i18n.TF(i18n.MsgWaitingReady, healthURL, cfg.ReadyTimeout))
	if waitErr := proc.WaitForServerProcess(healthURL, cfg.ReadyTimeout, serverProcess); waitErr != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgErrResolved, waitErr))
		return 1
	}
	fmt.Fprint(stdout, i18n.TF(i18n.MsgServerReady))

	// --- Local proxy for switching between llama-server and OpenAI ---
	var proxySrv *http.Server
	if cfg.OpenAIBaseURL != "" && cfg.OpenAIKey != "" && cfg.OpenAIModel != "" {
		llamaTargetURL := "http://" + net.JoinHostPort(strings.Trim(healthHost, "[]"), cfg.Port)
		proxyAddr := cfg.ProxyAddr
		if proxyAddr == "" {
			proxyAddr = "127.0.0.1:8082"
		}

		if proxy.ShouldWarnProxyExposure(proxyAddr, cfg) {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgWarnProxyExposure, proxyAddr))
		}
		fmt.Fprint(stdout, i18n.TF(i18n.MsgStartingProxy, proxyAddr, llamaTargetURL, cfg.OpenAIBaseURL))

		var proxyLogWriter io.Writer
		if sink.Writer != nil {
			proxyLogWriter = sink.Writer
		} else {
			w, pf := OpenProxyLog(home, stdout, cfg.Verbose)
			if pf != nil {
				proxyLogWriter = w
				defer pf.Close()
			}
		}
		if cfg.Verbose && proxyLogWriter == nil {
			fmt.Fprint(stdout, i18n.TF(i18n.MsgWarnProxyLogDisabled))
		}

		srv, err := proxy.StartProxyServer(proxyAddr, llamaTargetURL, cfg, proxyLogWriter)
		if err != nil {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgErrProxyStart, err))
			return 1
		}
		proxySrv = srv
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = proxySrv.Shutdown(ctx)
		}()

		// Wait for proxy readiness (not sleep, but poll)
		proxyHealthURL := "http://" + proxyAddr + "/v1/models"
		if err := proc.WaitForServer(proxyHealthURL, 5*time.Second); err != nil {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgProxyNotUp, err))
			return 1
		}

		baseURL = "http://" + proxyAddr
	}
	// --- MCP config for Claude Code ---
	var mcpConfigFile string
	if cfg.McpProxy {
		mcpHost, mcpPort, splitErr := net.SplitHostPort(cfg.McpAddr)
		if splitErr != nil {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgErrMcpAddrInvalid, cfg.McpAddr, splitErr))
			return 2
		}
		mcpURL := "http://" + net.JoinHostPort(strings.Trim(mcpHost, "[]"), mcpPort)
		mcpHealthURL := mcpURL + "/health"
		mcpEndpointURL := mcpURL + "/mcp"

		fmt.Fprint(stdout, i18n.TF(i18n.MsgWaitingMcpReady, mcpHealthURL))
		if mcpWaitErr := proc.WaitForServer(mcpHealthURL, 15*time.Second); mcpWaitErr != nil {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgWarnMcpNotReady, mcpWaitErr))
		} else {
			requestedConfigPath := config.ExpandHome(cfg.McpConfigOut, home)
			path, werr := proc.WriteMCPConfig(requestedConfigPath, mcpEndpointURL)
			if werr != nil {
				fmt.Fprint(stderr, i18n.TF(i18n.MsgWarnMcpConfigWrite, werr))
			} else {
				mcpConfigFile = path
				if requestedConfigPath == "" {
					defer os.Remove(mcpConfigFile)
				}
				if cfg.Verbose {
					fmt.Fprint(stdout, i18n.TF(i18n.MsgMcpConfigLine, path, mcpEndpointURL))
				}
			}
		}
	}

	// --- Locate claude ---
	claudePath := config.ExpandHome(cfg.ClaudeBin, home)
	if resolved, lookErr := cmdRunner.LookPath(claudePath); lookErr == nil {
		claudePath = resolved
	} else if !filepath.IsAbs(claudePath) {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgClaudeNotFound, cfg.ClaudeBin, lookErr))
		return 1
	}

	// --- Env and args for claude ---
	claudeEnv := cli.BuildClaudeEnv(cfg, baseURL, os.Environ())
	claudeArgs := cli.BuildClaudeArgs(cfg, mcpConfigFile)

	// --- stdio Claude Code: isolate TUI from redirected output ---
	// Claude Code — interactive TUI, needs a TTY. If the parent writes to
	// a pipe/file (e.g. `llama-runner -v 2>&1 | tee log`), claude will go into
	// non-interactive --print and crash without a prompt. In this case we redirect
	// stdio claude to /dev/tty, while keeping diagnostics in the parent's stdout/stderr.
	var ccTTY *os.File
	if shouldRedirectClaudeToTTY(stdout, stderr) {
		if tty := openTTY(); tty != nil {
			ccTTY = tty
			defer func() { _ = ccTTY.Close() }()
			if cfg.Verbose {
				fmt.Fprint(stdout, i18n.TF(i18n.MsgClaudeTtyRedirect))
			}
		} else {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgWarnNoTty))
		}
	}

	cc := cmdRunner.Command(claudePath, claudeArgs...)
	cc.Env = claudeEnv
	if cfg.Verbose && len(claudeArgs) > 0 {
		fmt.Fprint(stdout, i18n.TF(i18n.MsgClaudeArgsLine, claudeArgs))
	}
	if ccTTY != nil {
		cc.Stdin = ccTTY
		cc.Stdout = ccTTY
		cc.Stderr = ccTTY
	} else {
		cc.Stdin = os.Stdin
		cc.Stdout = stdout
		cc.Stderr = stderr
	}

	// --- Claude working directory ---
	if cfg.ClaudeDir != "" {
		absDir, dirErr := filepath.Abs(config.ExpandHome(cfg.ClaudeDir, home))
		if dirErr != nil {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgErrClaudeDirAbs, dirErr))
			return 1
		}
		if info, statErr := os.Stat(absDir); statErr != nil || !info.IsDir() {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgErrClaudeDirUnavail, absDir, statErr))
			return 1
		}
		cc.Dir = absDir
	} else if wd, wdErr := os.Getwd(); wdErr == nil {
		cc.Dir = wd
	}

	if cfg.Verbose {
		PrintClaudeDiagnostics(stdout, cfg, claudePath, cc.Dir, baseURL, claudeEnv)
	}

	if startErr := cc.Start(); startErr != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgErrClaudeStart, startErr))
		return 1
	}
	shut.SetClaude(cc)
	claudeProcess = proc.Track(cc)

	ccErr := claudeProcess.Wait()
	if ccErr != nil {
		if exitErr, ok := ccErr.(*exec.ExitError); ok {
			fmt.Fprint(stdout, i18n.TF(i18n.MsgClaudeExitCode, exitErr.ExitCode()))
		} else {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgClaudeExitErr, ccErr))
		}
	} else {
		fmt.Fprint(stdout, i18n.TF(i18n.MsgClaudeDone))
	}

	fmt.Fprint(stdout, i18n.TF(i18n.MsgDone))
	return 0

}
