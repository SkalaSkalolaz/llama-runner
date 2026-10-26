package main

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
	"sync"
	"syscall"
	"time"
)

// isTerminalFile reports whether f is connected to a terminal (character device).
func isTerminalFile(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// pathContainsSegment reports whether seg is among the :--separated path segments.
func pathContainsSegment(path, seg string) bool {
	for _, p := range strings.Split(path, ":") {
		if p == seg {
			return true
		}
	}
	return false
}

// openTTY tries to open /dev/tty. Returns nil if unavailable
// (Windows, no controlling terminal, /dev/tty trimmed in a container).
func openTTY() *os.File {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	return tty
}

// shouldRedirectClaudeToTTY true if Claude Code's stdio must be redirected
// to /dev/tty: at least one of the passed streams is a non-TTY *os.File (meaning
// there was a redirect/pipe in production). If a non-*os.File is passed (tests with bytes.Buffer),
// return false to avoid opening a real terminal from tests.
func shouldRedirectClaudeToTTY(stdout, stderr io.Writer) bool {
	sf, ok1 := stdout.(*os.File)
	ef, ok2 := stderr.(*os.File)
	if !ok1 || !ok2 {
		return false
	}
	return !isTerminalFile(sf) || !isTerminalFile(ef)
}

// resolvedPaths — all paths verified for existence.
type resolvedPaths struct {
	ServerBin string
	Model     string
	Mmproj    string
	Draft     string
	Template  string
}

// resolvePaths checks files and returns paths. The second result is
// an exit code if launch should be aborted (0 = all ok).
func resolvePaths(cfg *Config, home string, stdout, stderr io.Writer) (resolvedPaths, int) {
	var p resolvedPaths

	serverPath, err := filepath.Abs(expandHome(cfg.ServerBin, home))
	if err != nil {
		fmt.Fprintf(stderr, "ошибка: не удалось определить абсолютный путь llama-server: %v\n", err)
		return p, 2
	}
	p.ServerBin = serverPath

	modelPath, err := filepath.Abs(expandHome(cfg.Model, home))
	if err != nil {
		fmt.Fprintf(stderr, "ошибка: не удалось определить абсолютный путь модели: %v\n", err)
		return p, 2
	}
	p.Model = modelPath

	if _, err := os.Stat(p.ServerBin); err != nil {
		fmt.Fprintf(stderr, "llama-server не найден по пути %q: %v\n", p.ServerBin, err)
		return p, 1
	}
	if _, err := os.Stat(p.Model); err != nil {
		fmt.Fprintf(stderr, "модель не найдена по пути %q: %v\n", p.Model, err)
		return p, 1
	}

	templateRaw := resolveChatTemplate(cfg.ChatTemplate, p.Model, cfg.ChatTemplateFallback)
	if cfg.Verbose && cfg.ChatTemplate == "auto" && templateRaw != "" {
		fmt.Fprintf(stdout, "Автоопределение шаблона: qwen → %s\n", templateRaw)
	}
	templatePath, terr := validateTemplatePath(templateRaw, home, cfg.ChatTemplateSet)
	if terr != nil {
		fmt.Fprintf(stderr, "ошибка: %v\n", terr)
		return p, 2
	}
	if templateRaw != "" && templatePath == "" && !cfg.ChatTemplateSet {
		fmt.Fprintf(stderr, "предупреждение: автоматически выбранный шаблон %q недоступен; использую шаблон из GGUF\n", templateRaw)
	}
	p.Template = templatePath

	if cfg.Mmproj != "" {
		mmprojPath, err := filepath.Abs(expandHome(cfg.Mmproj, home))
		if err != nil {
			fmt.Fprintf(stderr, "ошибка: не удалось определить абсолютный путь mmproj: %v\n", err)
			return p, 2
		}
		if _, err := os.Stat(mmprojPath); err != nil {
			fmt.Fprintf(stderr, "mmproj не найден по пути %q: %v\n", mmprojPath, err)
			return p, 1
		}
		p.Mmproj = mmprojPath
	}

	if cfg.SpecDraftModel != "" {
		draftPath, err := filepath.Abs(expandHome(cfg.SpecDraftModel, home))
		if err != nil {
			fmt.Fprintf(stderr, "ошибка: не удалось определить абсолютный путь spec-draft-model: %v\n", err)
			return p, 2
		}
		if _, err := os.Stat(draftPath); err != nil {
			fmt.Fprintf(stderr, "spec-draft-model не найден по пути %q: %v\n", draftPath, err)
			return p, 1
		}
		p.Draft = draftPath
	}

	return p, 0
}

// shutdown centralizes stopping child processes.
// Signals:
//   - claude   — to the process directly (red zone #1: interactive stdin,
//     non-foreground pgid = SIGTTIN → hung UI);
//   - server   — to the group (Setpgid:true at launch);
//   - mcp      — to the group (Setpgid:true).
//
// If server/mcp are started without Setpgid (e.g. in old tests),
// fallback — direct Process.Signal.
type shutdown struct {
	mu     sync.Mutex
	claude *exec.Cmd
	server *exec.Cmd
	mcp    *exec.Cmd
}

func newShutdown() *shutdown { return &shutdown{} }

func (s *shutdown) SetClaude(cmd *exec.Cmd) {
	s.mu.Lock()
	s.claude = cmd
	s.mu.Unlock()
}

func (s *shutdown) SetServer(cmd *exec.Cmd) {
	s.mu.Lock()
	s.server = cmd
	s.mu.Unlock()
}

func (s *shutdown) SetMCP(cmd *exec.Cmd) {
	s.mu.Lock()
	s.mcp = cmd
	s.mu.Unlock()
}

// signal sends sig to claude directly, server/mcp — to their groups.
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

func (s *shutdown) StopAll()                    { s.signal(syscall.SIGINT) }
func (s *shutdown) Escalate(sig syscall.Signal) { s.signal(sig) }

// handleSignals catches Ctrl-C/Term and stops processes with escalation:
// SIGINT (immediately) → SIGTERM (after timeout) → SIGKILL (after another timeout).
// Repeated signal accelerates escalation (not lost — defect #12).
// done is closed on cancellation from run() (via defer close(done)).
func handleSignals(sigCh <-chan os.Signal, done <-chan struct{}, shut *shutdown,
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
	fmt.Fprintf(stdout, "\nПолучен сигнал %v — останавливаем дочерние процессы...\n", sig)
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
			fmt.Fprintf(stdout, "\nПовторный сигнал %v — эскалация без ожидания\n", s)
		case <-time.After(timeout):
			fmt.Fprintf(stdout, "\nЭскалация: %s\n", esc.name)
		}
		shut.Escalate(esc.sig)
	}
}

func run(args []string, stdout, stderr io.Writer, runner CommandRunner) int {
	cfg, err := parseFlags(args, stderr)
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
		fmt.Fprintln(stderr, "предупреждение: -openai/-key/-model-openai игнорируются в режиме -server")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "не удалось получить домашний каталог: %v\n", err)
		return 1
	}

	cfg.Home = home

	// We pick up the external provider's key ONLY if the external provider
	// is actually requested. Otherwise a developer with OPENAI_API_KEY in the environment
	// would get "-openai is required when using -key" on a local
	// launch (including `-update`), even though OpenAI isn't needed at all.
	if cfg.OpenAIBaseURL != "" || cfg.OpenAIModel != "" || cfg.OpenAIKey != "" {
		resolvedKey, keyErr := resolveOpenAIKey(cfg.OpenAIKey, os.Getenv, home, stderr)
		if keyErr != nil {
			fmt.Fprintf(stderr, "ошибка: %v\n", keyErr)
			return 2
		}
		cfg.OpenAIKey = resolvedKey
	}

	if err := validateConfig(cfg); err != nil {
		fmt.Fprintf(stderr, "ошибка: %v\n", err)
		return 2
	}
	if cfg.DoctorFlag {
		return runDoctor(cfg, home, runner, stdout, stderr)
	}
	if cfg.UpdateFlag {
		srcAbs := expandHome(cfg.LlamaSrc, home)
		installAbs := expandHome(cfg.InstallPrefix, home)
		if err := updateLlamaCPP(runner, srcAbs, installAbs, cfg.CudaArch,
			cfg.WithCuda, cfg.Verbose, stdout); err != nil {
			fmt.Fprintf(stderr, "ошибка обновления llama.cpp: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "llama.cpp успешно обновлён и установлен.")
		return 0
	}

	// --- Paths ---
	paths, code := resolvePaths(cfg, home, stdout, stderr)
	if code != 0 {
		return code
	}

	// --- Build args ---
	serverArgs := buildServerArgs(cfg, ServerPaths{
		Model: paths.Model, Mmproj: paths.Mmproj,
		Draft: paths.Draft, Template: paths.Template,
	})

	cmd := runner.Command(paths.ServerBin, serverArgs...)
	// Setpgid — only for server and mcp (red zone #1):
	// they don't read the terminal, the group is needed for group kill.
	// For claude — we do NOT set it (see shutdown.SetClaude).
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// --- LD_LIBRARY_PATH for llama-server ---
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	for _, sub := range []string{"lib", "lib64"} {
		cand := filepath.Join(filepath.Dir(paths.ServerBin), "..", sub)
		abs, absErr := filepath.Abs(cand)
		if absErr != nil {
			continue
		}
		abs = filepath.Clean(abs)
		if info, statErr := os.Stat(abs); statErr != nil || !info.IsDir() {
			continue
		}
		oldLP := os.Getenv("LD_LIBRARY_PATH")
		if pathContainsSegment(oldLP, abs) {
			// Already in the environment — don't duplicate (otherwise the log shows
			// ".../lib:/home/.../lib:/usr/local/cuda/lib64:").
			if cfg.Verbose {
				fmt.Fprintf(stdout, "LD_LIBRARY_PATH для llama-server: %s (уже содержит %s)\n", oldLP, abs)
			}
		} else {
			newLP := abs
			if oldLP != "" {
				newLP = abs + ":" + oldLP
			}
			cmd.Env = append(cmd.Env, "LD_LIBRARY_PATH="+newLP)
			if cfg.Verbose {
				fmt.Fprintf(stdout, "LD_LIBRARY_PATH для llama-server: %s\n", newLP)
			}
		}
		break
	}
	// --- Working directory ---
	if cfg.WorkDir != "" {
		absDir, dirErr := filepath.Abs(expandHome(cfg.WorkDir, home))
		if dirErr != nil {
			fmt.Fprintf(stderr, "не удалось определить абсолютный путь рабочей директории: %v\n", dirErr)
			return 1
		}
		if info, statErr := os.Stat(absDir); statErr != nil || !info.IsDir() {
			fmt.Fprintf(stderr, "рабочая директория %q недоступна: %v\n", absDir, statErr)
			return 1
		}
		cmd.Dir = absDir
	} else if wd, wdErr := os.Getwd(); wdErr == nil {
		cmd.Dir = wd
	}

	// --- Log file (Claude Code mode) ---
	// serverLogFile — *os.File for backward compatibility with other
	// code (serverLogFile.Name(), -v output); serverLogWriter — what is
	// actually passed to cmd.Stdout/Stderr. If rotation is disabled
	// (LogMaxMB==0), serverLogWriter == serverLogFile.
	var serverLogFile *os.File
	var serverLogWriter io.Writer
	if !cfg.ServerOnly && cfg.QuietServer {
		logPath := expandHome(cfg.LogFile, home)
		if mkErr := os.MkdirAll(filepath.Dir(logPath), 0o700); mkErr != nil {
			fmt.Fprintf(stderr, "не удалось создать каталог для логов %q: %v\n", filepath.Dir(logPath), mkErr)
			return 1
		}
		if cfg.LogMaxMB > 0 {
			rw, rwErr := newRotatingWriter(logPath, int64(cfg.LogMaxMB)*1024*1024, cfg.LogKeep, 0o600)
			if rwErr != nil {
				fmt.Fprintf(stderr, "не удалось открыть ротируемый лог-файл %q: %v\n", logPath, rwErr)
				return 1
			}
			defer rw.Close()
			serverLogWriter = rw
		} else {
			f, openErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
			if openErr != nil {
				fmt.Fprintf(stderr, "не удалось открыть лог-файл %q: %v\n", logPath, openErr)
				return 1
			}
			if chErr := f.Chmod(0o600); chErr != nil {
				_ = f.Close()
				fmt.Fprintf(stderr, "не удалось установить права доступа к лог-файлу %q: %v\n", logPath, chErr)
				return 2
			}
			serverLogFile = f
			serverLogWriter = f
			defer f.Close()
		}
		cmd.Stdout = serverLogWriter
		cmd.Stderr = serverLogWriter
	} else {
		cmd.Stdout = stdout
		cmd.Stderr = stderr
	}

	cmd.Stdin = os.Stdin

	if cfg.Verbose {
		fmt.Fprintf(stdout, "Рабочая директория: %s\n", cmd.Dir)
		fmt.Fprintf(stdout, "Команда: %s %s\n\n", paths.ServerBin, strings.Join(serverArgs, " "))
		if serverLogFile != nil {
			fmt.Fprintf(stdout, "Логи llama-server: %s\n", serverLogFile.Name())
		}
	} else {
		fmt.Fprintf(stdout, "Запуск llama-server на %s:%s ...\n", cfg.Host, cfg.Port)
		if serverLogFile != nil {
			fmt.Fprintf(stdout, "Логи llama-server: %s\n", serverLogFile.Name())
		}
	}

	// --- mcp-search-server ---
	var mcpCmd *exec.Cmd
	if cfg.McpProxy {
		mcpCmd = runner.Command("mcp-search-server", "--addr", cfg.McpAddr)
		mcpCmd.Env = append(os.Environ(), "SEARCH_ENABLED=true")
		// Setpgid for mcp — same as for llama-server (red zone #1:
		// mcp terminal doesn't read, group is needed for group kill).
		mcpCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		if serverLogWriter != nil {
			mcpCmd.Stdout = serverLogWriter
			mcpCmd.Stderr = serverLogWriter
		} else {
			mcpCmd.Stdout = stdout
			mcpCmd.Stderr = stderr
		}
		if cmd.Dir != "" {
			mcpCmd.Dir = cmd.Dir
		}
		if cfg.Verbose {
			fmt.Fprintf(stdout, "MCP-сервер: SEARCH_ENABLED=true mcp-search-server --addr %s\n\n", cfg.McpAddr)
		}
		if startErr := mcpCmd.Start(); startErr != nil {
			fmt.Fprintf(stderr, "ошибка запуска mcp-search-server: %v\n", startErr)
			// mcpCmd.Process == nil (Start failed), server not yet started,
			// nothing to stop.
			return 1
		}
		defer stopMcp(mcpCmd)
	}

	// --- Launch llama-server ---
	if startErr := cmd.Start(); startErr != nil {
		fmt.Fprintf(stderr, "ошибка запуска сервера: %v\n", startErr)
		if mcpCmd != nil && mcpCmd.Process != nil {
			_ = mcpCmd.Process.Signal(syscall.SIGINT)
			_ = mcpCmd.Wait()
		}
		return 1
	}
	serverProcess := trackProcess(cmd)

	// --- Centralized shutdown ---
	shut := newShutdown()
	shut.SetServer(cmd)
	if mcpCmd != nil {
		shut.SetMCP(mcpCmd)
	}

	var claudeProcess *trackedProcess
	cleanup := func() {
		shut.StopAll()
		if claudeProcess != nil {
			<-claudeProcess.done
		}
		_ = serverProcess.Wait()
	}
	defer cleanup()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	sigDone := make(chan struct{})
	defer close(sigDone)
	go handleSignals(sigCh, sigDone, shut, stdout, 15*time.Second)

	// =================================================================
	// MODE 1: -server — llama-server only
	// =================================================================
	if cfg.ServerOnly {
		waitErr := serverProcess.Wait()
		if waitErr != nil {
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				stopMcp(mcpCmd)
				return exitErr.ExitCode()
			}
			fmt.Fprintf(stderr, "сервер завершился с ошибкой: %v\n", waitErr)
			stopMcp(mcpCmd)
			return 1
		}
		fmt.Fprintln(stdout, "llama-server завершён.")
		stopMcp(mcpCmd)
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

	fmt.Fprintf(stdout, "Ожидание готовности сервера (%s), таймаут %v ...\n", healthURL, cfg.ReadyTimeout)
	if waitErr := waitForServerProcess(healthURL, cfg.ReadyTimeout, serverProcess); waitErr != nil {
		fmt.Fprintf(stderr, "ошибка: %v\n", waitErr)
		return 1
	}
	fmt.Fprintln(stdout, "llama-server готов. Запуск Claude Code...")

	// --- Local proxy for switching between llama-server and OpenAI ---
	var proxySrv *http.Server
	if cfg.OpenAIBaseURL != "" && cfg.OpenAIKey != "" && cfg.OpenAIModel != "" {
		llamaTargetURL := "http://" + net.JoinHostPort(strings.Trim(healthHost, "[]"), cfg.Port)
		proxyAddr := cfg.ProxyAddr
		if proxyAddr == "" {
			proxyAddr = "127.0.0.1:8082"
		}

		if shouldWarnProxyExposure(proxyAddr, cfg) {
			fmt.Fprintf(stderr, "предупреждение: прокси слушает не-loopback адрес %s без аутентификации — "+
				"любой в сети может использовать ваш платный ключ. Задайте -proxy-token (и тот же токен "+
				"в Claude Code) или явный -proxy-allow-remote.\n", proxyAddr)
		}
		fmt.Fprintf(stdout, "Запуск прокси на %s (llama → %s, openai → %s)\n",
			proxyAddr, llamaTargetURL, cfg.OpenAIBaseURL)

		var proxyLogWriter io.Writer
		if serverLogWriter != nil {
			proxyLogWriter = serverLogWriter
		} else {
			proxyLogPath := expandHome("~/.cache/llama-runner/proxy.log", home)
			if mkErr := os.MkdirAll(filepath.Dir(proxyLogPath), 0o700); mkErr == nil {
				if pf, openErr := os.OpenFile(proxyLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); openErr == nil {
					proxyLogWriter = pf
					defer func() { _ = pf.Close() }()
					if cfg.Verbose {
						fmt.Fprintf(stdout, "Логи прокси: %s\n", proxyLogPath)
					}
				}
			}
		}
		if cfg.Verbose && proxyLogWriter == nil {
			fmt.Fprintln(stdout,
				"предупреждение: не удалось открыть лог-файл для [proxy] — логи прокси отключены, чтобы не портить TUI Claude Code")
		}

		srv, err := StartProxyServer(proxyAddr, llamaTargetURL, cfg, proxyLogWriter)
		if err != nil {
			fmt.Fprintf(stderr, "ошибка запуска прокси: %v\n", err)
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
		if err := waitForServer(proxyHealthURL, 5*time.Second); err != nil {
			fmt.Fprintf(stderr, "прокси не поднялся: %v\n", err)
			return 1
		}

		baseURL = "http://" + proxyAddr
	}
	// --- MCP config for Claude Code ---
	var mcpConfigFile string
	if cfg.McpProxy {
		mcpHost, mcpPort, splitErr := net.SplitHostPort(cfg.McpAddr)
		if splitErr != nil {
			fmt.Fprintf(stderr, "некорректный -mcp-addr %q: %v\n", cfg.McpAddr, splitErr)
			return 2
		}
		mcpURL := "http://" + net.JoinHostPort(strings.Trim(mcpHost, "[]"), mcpPort)
		mcpHealthURL := mcpURL + "/health"
		mcpEndpointURL := mcpURL + "/mcp"

		fmt.Fprintf(stdout, "Ожидание готовности MCP-сервера (%s) ...\n", mcpHealthURL)
		if mcpWaitErr := waitForServer(mcpHealthURL, 15*time.Second); mcpWaitErr != nil {
			fmt.Fprintf(stderr,
				"предупреждение: mcp-search-server не готов: %v\n"+
					"Claude Code будет запущен без web_search.\n", mcpWaitErr)
		} else {
			requestedConfigPath := expandHome(cfg.McpConfigOut, home)
			path, werr := writeMCPConfig(requestedConfigPath, mcpEndpointURL)
			if werr != nil {
				fmt.Fprintf(stderr, "предупреждение: не удалось создать MCP-конфиг: %v\n", werr)
			} else {
				mcpConfigFile = path
				if requestedConfigPath == "" {
					defer os.Remove(mcpConfigFile)
				}
				if cfg.Verbose {
					fmt.Fprintf(stdout, "MCP-конфиг: %s (url=%s, type=http)\n", path, mcpEndpointURL)
				}
			}
		}
	}

	// --- Locate claude ---
	claudePath := expandHome(cfg.ClaudeBin, home)
	if resolved, lookErr := runner.LookPath(claudePath); lookErr == nil {
		claudePath = resolved
	} else if !filepath.IsAbs(claudePath) {
		fmt.Fprintf(stderr, "Claude Code не найден (%q): %v\n", cfg.ClaudeBin, lookErr)
		return 1
	}

	// --- Env and args for claude ---
	claudeEnv := buildClaudeEnv(cfg, baseURL, os.Environ())
	claudeArgs := buildClaudeArgs(cfg, mcpConfigFile)

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
				fmt.Fprintln(stdout, "Claude Code: stdio перенаправлен в /dev/tty (родительские stdout/stderr — не TTY)")
			}
		} else {
			fmt.Fprintln(stderr, "предупреждение: stdout/stderr не TTY, /dev/tty недоступен — Claude Code может не запуститься интерактивно")
		}
	}

	cc := runner.Command(claudePath, claudeArgs...)
	cc.Env = claudeEnv
	if cfg.Verbose && len(claudeArgs) > 0 {
		fmt.Fprintf(stdout, "Claude Code args: %v\n", claudeArgs)
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
		absDir, dirErr := filepath.Abs(expandHome(cfg.ClaudeDir, home))
		if dirErr != nil {
			fmt.Fprintf(stderr, "не удалось определить абсолютный путь claude-dir: %v\n", dirErr)
			return 1
		}
		if info, statErr := os.Stat(absDir); statErr != nil || !info.IsDir() {
			fmt.Fprintf(stderr, "claude-dir %q недоступна: %v\n", absDir, statErr)
			return 1
		}
		cc.Dir = absDir
	} else if wd, wdErr := os.Getwd(); wdErr == nil {
		cc.Dir = wd
	}

	if cfg.Verbose {
		fmt.Fprintf(stdout, "Claude Code: %s (cwd=%s)\n", claudePath, cc.Dir)
		fmt.Fprintf(stdout, "  ANTHROPIC_BASE_URL=%s\n", baseURL)
		if cfg.ProxyToken != "" {
			fmt.Fprintln(stdout, "  ANTHROPIC_AUTH_TOKEN=<configured via -proxy-token, hidden>")
		}
		// Print the actually set ANTHROPIC_* from claudeEnv, not cfg.Alias:
		// in hybrid mode all three DEFAULT_* point to the external model, not the
		// local alias — otherwise -v lies.
		for _, want := range []string{
			"ANTHROPIC_DEFAULT_SONNET_MODEL",
			"ANTHROPIC_DEFAULT_HAIKU_MODEL",
			"ANTHROPIC_DEFAULT_OPUS_MODEL",
			"ANTHROPIC_CUSTOM_MODEL_OPTION",
		} {
			for _, kv := range claudeEnv {
				if strings.HasPrefix(kv, want+"=") {
					fmt.Fprintf(stdout, "  %s\n", kv)
					break
				}
			}
		}
		fmt.Fprintln(stdout)
	}

	if startErr := cc.Start(); startErr != nil {
		fmt.Fprintf(stderr, "ошибка запуска Claude Code: %v\n", startErr)
		return 1
	}
	shut.SetClaude(cc)
	claudeProcess = trackProcess(cc)

	ccErr := claudeProcess.Wait()
	if ccErr != nil {
		if exitErr, ok := ccErr.(*exec.ExitError); ok {
			fmt.Fprintf(stdout, "Claude Code завершён с кодом %d. Останавливаем llama-server...\n", exitErr.ExitCode())
		} else {
			fmt.Fprintf(stderr, "Claude Code завершился с ошибкой: %v\n", ccErr)
		}
	} else {
		fmt.Fprintln(stdout, "Claude Code завершён. Останавливаем llama-server...")
	}

	fmt.Fprintln(stdout, "Готово.")
	return 0

}

// printUsageRU prints detailed program help in Russian.
func printUsageRU(w io.Writer) {
	fmt.Fprintf(w, `llama-runner — лаунчер для llama-server и Claude Code

По умолчанию запускает локальный llama-server и, дождавшись его готовности,
поднимает Claude Code с нужными переменными окружения (ANTHROPIC_BASE_URL и др.).
Когда Claude Code завершается, llama-server автоматически останавливается.

С флагом -server запускается ТОЛЬКО llama-server (прежнее поведение).
С флагом -update обновляется llama.cpp (git pull + сборка + установка), затем выход.

При использовании -webui-mcp-proxy дополнительно запускается
SEARCH_ENABLED=true mcp-search-server --addr :8090.

Использование:
  llama-runner [флаги]

Основные флаги:
  -bin string
        путь к llama-server (по умолчанию "~/.local/bin/llama-server")
  -model string
        путь к GGUF-модели
  -host string
        адрес сервера (по умолчанию "127.0.0.1")
  -port string
        порт сервера (по умолчанию "8080")
  -alias string
        алиас модели (--alias для llama-server и ANTHROPIC_DEFAULT_*_MODEL)
  -v    печатать команду перед запуском
  -version
        показать версию и выйти
  -h, --help
        показать эту справку и выйти

Параметры llama.cpp:
  -c, -b, -ub string   контекст / батч / микро-батч
  -fa string           flash attention (-fa)
  -parallel string     число параллельных слотов
  -tools string        режим инструментов
  -jinja               Jinja-шаблоны
  -ngl string          слоёв на GPU
  -mmproj string       путь к мультимодальному проектору
  -temp, -top-p string сэмплинг
  -cache-type-k/v string типы KV-кэша
  -fit string          режим --fit (on|off)
  -lv string           уровень логов llama.cpp
  -lm-lazy             mmap с ленивым режимом
  -webui-mcp-proxy     CORS-прокси + mcp-search-server
  -reasoning string    режим размышлений: on|off|auto
  -reasoning-budget N  бюджет токенов на размышления: -1/0/N

Спекулятивное декодирование:
  -spec-type string        тип (draft-mtp, draft-dflash)
  -spec-draft-model string путь к драфтеру
  -spec-draft-n-max string макс. число черновых токенов
  -spec-draft-ngl string   слоёв драфтера на GPU

Claude Code:
  -server              только llama-server, без Claude Code
  -claude-bin string   путь к claude (по умолчанию "claude")
  -claude-dir string   рабочая директория claude
  -ready-timeout dur   таймаут готовности /health (по умолчанию 180s)
  -log-file string     файл логов llama-server (по умолчанию "~/.cache/llama-runner/llama-server.log")
  -quiet-server        уводить логи llama-server в файл (по умолчанию true)
  -log-max-mb int      макс. размер лога llama-server в МБ перед ротацией
                       (по умолчанию 50; 0 — не ротировать)
  -log-keep int        сколько архивов лога хранить (по умолчанию 5; 0 — без архивов)
  -mcp-addr string     адрес mcp-search-server (по умолчанию 127.0.0.1:8090)
  -mcp-config string   путь к MCP-конфигу (по умолчанию временный)
  -mcp-strict          передать --strict-mcp-config
  -chat-template str   шаблон чата ("auto" | "" | путь)
  -openai string       URL OpenAI-совместимого API
  -key string          API-ключ внешнего провайдера
  -model-openai str    имя модели у внешнего провайдера
  -proxy-addr string   адрес локального прокси (по умолчанию 127.0.0.1:8082)
  -proxy-token string  токен для проверки входящих запросов прокси (пусто — без проверки)
  -proxy-allow-remote  разрешить не-loopback -proxy-addr без предупреждения

Конфиг:
  -config string       путь к конфиг-файлу (по умолчанию
                       "~/.config/llama-runner/config.json"; "" — не читать)
  -p string            профиль из конфиг-файла (profiles: {имя: {...}})
  -doctor              показать разрешённые пути и версию, не запуская процессы

Обновление llama.cpp:
  -update
        обновить llama.cpp: git pull --ff-only, cmake configure/build/install,
        затем выйти
  -llama-src string
        путь к исходникам llama.cpp (по умолчанию "~/src/llama.cpp")
  -install-prefix string
        префикс для -DCMAKE_INSTALL_PREFIX (по умолчанию "~/.local")
  -cuda
        собирать с CUDA (по умолчанию true)
  -cuda-arch string
        значение для -DCMAKE_CUDA_ARCHITECTURES (по умолчанию "61")

Примеры:
  # Сервер + Claude Code
  llama-runner

  # Только сервер
  llama-runner -server -port 9090

  # MCP-поиск + Claude Code
  llama-runner -webui-mcp-proxy

  # Профиль из конфиг-файла
  llama-runner -p coder

  # Диагностика без запуска процессов
  llama-runner -doctor

  # Обновить llama.cpp
  llama-runner -update -cuda-arch 61

Совет:
  Для доступа из локальной сети используйте -host 0.0.0.0.
`)
}

// printUsageEN prints detailed program help in English.
func printUsageEN(w io.Writer) {
	fmt.Fprintf(w, `llama-runner — launcher for llama-server and Claude Code

By default it starts a local llama-server and, after waiting for it to be ready,
launches Claude Code with the required environment variables (ANTHROPIC_BASE_URL, etc.).
When Claude Code finishes, llama-server stops automatically.

With -server, ONLY llama-server starts (legacy behavior).
With -update, llama.cpp is updated (git pull + build + install), then exits.

With -webui-mcp-proxy, SEARCH_ENABLED=true mcp-search-server --addr :8090 is also started.

Usage:
  llama-runner [flags]

Main flags:
  -bin string
        path to llama-server (default "~/.local/bin/llama-server")
  -model string
        path to GGUF model
  -host string
        server address (default "127.0.0.1")
  -port string
        server port (default "8080")
  -alias string
        model alias (--alias for llama-server and ANTHROPIC_DEFAULT_*_MODEL)
  -v    print the command before launch
  -version
        show version and exit
  -h, --help
        show this help and exit

llama.cpp parameters:
  -c, -b, -ub string   context / batch / micro-batch
  -fa string           flash attention (-fa)
  -parallel string     number of parallel slots
  -tools string        tool mode
  -jinja               Jinja templates
  -ngl string          layers on GPU
  -mmproj string       path to multimodal projector
  -temp, -top-p string sampling
  -cache-type-k/v string KV-cache types
  -fit string          --fit mode (on|off)
  -lv string           llama.cpp log verbosity
  -lm-lazy             mmap with lazy mode
  -webui-mcp-proxy     CORS-proxy + mcp-search-server
  -reasoning string    reasoning mode: on|off|auto
  -reasoning-budget N  reasoning token budget: -1 (unlimited), 0 (disabled), N (positive)

Speculative decoding:
  -spec-type string        type (draft-mtp, draft-dflash)
  -spec-draft-model string path to draft model
  -spec-draft-n-max string max draft tokens
  -spec-draft-ngl string   draft layers on GPU

Claude Code:
  -server              only llama-server, no Claude Code
  -claude-bin string   path to claude (default "claude")
  -claude-dir string   working directory for claude
  -ready-timeout dur   readiness timeout for /health (default 180s)
  -log-file string     llama-server log file (default "~/.cache/llama-runner/llama-server.log")
  -quiet-server        redirect llama-server logs to file (default true)
  -log-max-mb int      max log size in MB before rotation
                       (default 50; 0 = no rotation)
  -log-keep int        how many log archives to keep (default 5; 0 = no archives)
  -mcp-addr string     mcp-search-server address (default 127.0.0.1:8090)
  -mcp-config string   path to MCP config (default: temp file)
  -mcp-strict          pass --strict-mcp-config
  -chat-template str   chat template ("auto" | "" | path)
  -openai string       OpenAI-compatible API URL
  -key string          external provider API key
  -model-openai str    model name at external provider
  -proxy-addr string   local proxy address (default 127.0.0.1:8082)
  -proxy-token string  proxy auth token for incoming requests (empty = no check)
  -proxy-allow-remote  allow non-loopback -proxy-addr without warning

Config:
  -config string       config file path (default
                       "~/.config/llama-runner/config.json"; "" = don't read)
  -p string            profile from config file (profiles: {name: {...}})
  -doctor              show resolved paths and version without starting processes

llama.cpp update:
  -update
        update llama.cpp: git pull --ff-only, cmake configure/build/install,
        then exit
  -llama-src string
        path to llama.cpp sources (default "~/src/llama.cpp")
  -install-prefix string
        -DCMAKE_INSTALL_PREFIX prefix (default "~/.local")
  -cuda
        build with CUDA (default true)
  -cuda-arch string
        value for -DCMAKE_CUDA_ARCHITECTURES (default "61")

Examples:
  # Server + Claude Code
  llama-runner

  # Server only
  llama-runner -server -port 9090

  # MCP search + Claude Code
  llama-runner -webui-mcp-proxy

  # Profile from config file
  llama-runner -p coder

  # Diagnose without starting processes
  llama-runner -doctor

  # Update llama.cpp
  llama-runner -update -cuda-arch 61

Tip:
  For local network access, use -host 0.0.0.0.
`)
}
