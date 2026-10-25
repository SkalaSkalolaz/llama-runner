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

// isTerminalFile сообщает, подключён ли f к терминалу (символьному устройству).
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

// pathContainsSegment сообщает, есть ли seg среди :-разделённых сегментов path.
func pathContainsSegment(path, seg string) bool {
	for _, p := range strings.Split(path, ":") {
		if p == seg {
			return true
		}
	}
	return false
}

// openTTY пытается открыть /dev/tty. Возвращает nil, если недоступен
// (Windows, нет контролирующего терминала, /dev/tty урезан в контейнере).
func openTTY() *os.File {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	return tty
}

// shouldRedirectClaudeToTTY — true, если stdio Claude Code надо перенаправить
// в /dev/tty: хотя бы один из переданных потоков — не-TTY *os.File (значит,
// в проде был редирект/пайп). Если передан не *os.File (тесты с bytes.Buffer),
// возвращаем false, чтобы не открывать реальный терминал из тестов.
func shouldRedirectClaudeToTTY(stdout, stderr io.Writer) bool {
	sf, ok1 := stdout.(*os.File)
	ef, ok2 := stderr.(*os.File)
	if !ok1 || !ok2 {
		return false
	}
	return !isTerminalFile(sf) || !isTerminalFile(ef)
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

	resolvedKey, keyErr := resolveOpenAIKey(cfg.OpenAIKey, os.Getenv, home, stderr)
	if keyErr != nil {
		fmt.Fprintf(stderr, "ошибка: %v\n", keyErr)
		return 2
	}
	cfg.OpenAIKey = resolvedKey

	if err := validateConfig(cfg); err != nil {
		fmt.Fprintf(stderr, "ошибка: %v\n", err)
		return 2
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

	// --- Пути ---
	serverPath, err := filepath.Abs(expandHome(cfg.ServerBin, home))
	if err != nil {
		fmt.Fprintf(stderr, "ошибка: не удалось определить абсолютный путь llama-server: %v\n", err)
		return 2
	}
	modelPath, err := filepath.Abs(expandHome(cfg.Model, home))
	if err != nil {
		fmt.Fprintf(stderr, "ошибка: не удалось определить абсолютный путь модели: %v\n", err)
		return 2
	}
	if _, err := os.Stat(serverPath); err != nil {
		fmt.Fprintf(stderr, "llama-server не найден по пути %q: %v\n", serverPath, err)
		return 1
	}
	if _, err := os.Stat(modelPath); err != nil {
		fmt.Fprintf(stderr, "модель не найдена по пути %q: %v\n", modelPath, err)
		return 1
	}

	// --- Chat-template ---
	templateRaw := resolveChatTemplate(cfg.ChatTemplate, modelPath)
	if cfg.Verbose && cfg.ChatTemplate == "auto" && templateRaw != "" {
		fmt.Fprintf(stdout, "Автоопределение шаблона: qwen → %s\n", templateRaw)
	}
	templatePath, err := validateTemplatePath(templateRaw, home, cfg.ChatTemplateSet)
	if err != nil {
		fmt.Fprintf(stderr, "ошибка: %v\n", err)
		return 2
	}
	if templateRaw != "" && templatePath == "" && !cfg.ChatTemplateSet {
		fmt.Fprintf(stderr, "предупреждение: автоматически выбранный шаблон %q недоступен; использую шаблон из GGUF\n", templateRaw)
	}

	// --- Mmproj / Draft ---
	var mmprojPath, draftPath string
	if cfg.Mmproj != "" {
		mmprojPath, err = filepath.Abs(expandHome(cfg.Mmproj, home))
		if err != nil {
			fmt.Fprintf(stderr, "ошибка: не удалось определить абсолютный путь mmproj: %v\n", err)
			return 2
		}
		if _, err := os.Stat(mmprojPath); err != nil {
			fmt.Fprintf(stderr, "mmproj не найден по пути %q: %v\n", mmprojPath, err)
			return 1
		}
	}
	if cfg.SpecDraftModel != "" {
		draftPath, err = filepath.Abs(expandHome(cfg.SpecDraftModel, home))
		if err != nil {
			fmt.Fprintf(stderr, "ошибка: не удалось определить абсолютный путь spec-draft-model: %v\n", err)
			return 2
		}
		if _, err := os.Stat(draftPath); err != nil {
			fmt.Fprintf(stderr, "spec-draft-model не найден по пути %q: %v\n", draftPath, err)
			return 1
		}
	}

	// --- Сборка args ---
	serverArgs := buildServerArgs(cfg, ServerPaths{
		Model: modelPath, Mmproj: mmprojPath,
		Draft: draftPath, Template: templatePath,
	})

	cmd := runner.Command(serverPath, serverArgs...)

	// --- LD_LIBRARY_PATH для llama-server ---
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	for _, sub := range []string{"lib", "lib64"} {
		cand := filepath.Join(filepath.Dir(serverPath), "..", sub)
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
			// Уже есть в окружении — не дублируем (иначе в логе появляется
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
	// --- Рабочая директория ---
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

	// --- Лог-файл (в режиме Claude Code) ---
	var serverLogFile *os.File
	if !cfg.ServerOnly && cfg.QuietServer {
		logPath := expandHome(cfg.LogFile, home)
		if mkErr := os.MkdirAll(filepath.Dir(logPath), 0o700); mkErr != nil {
			fmt.Fprintf(stderr, "не удалось создать каталог для логов %q: %v\n", filepath.Dir(logPath), mkErr)
			return 1
		}
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
		defer serverLogFile.Close()
		cmd.Stdout = f
		cmd.Stderr = f
	} else {
		cmd.Stdout = stdout
		cmd.Stderr = stderr
	}
	cmd.Stdin = os.Stdin

	if cfg.Verbose {
		fmt.Fprintf(stdout, "Рабочая директория: %s\n", cmd.Dir)
		fmt.Fprintf(stdout, "Команда: %s %s\n\n", serverPath, strings.Join(serverArgs, " "))
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
		if serverLogFile != nil {
			mcpCmd.Stdout = serverLogFile
			mcpCmd.Stderr = serverLogFile
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
			return 1
		}
	}

	// --- Запуск llama-server ---
	if startErr := cmd.Start(); startErr != nil {
		fmt.Fprintf(stderr, "ошибка запуска сервера: %v\n", startErr)
		if mcpCmd != nil && mcpCmd.Process != nil {
			_ = mcpCmd.Process.Signal(syscall.SIGINT)
			_ = mcpCmd.Wait()
		}
		return 1
	}
	serverProcess := trackProcess(cmd)

	// --- Менеджер дочерних процессов для сигналов ---
	var (
		mu        sync.Mutex
		claudeCmd *exec.Cmd
	)
	setClaudeCmd := func(c *exec.Cmd) {
		mu.Lock()
		claudeCmd = c
		mu.Unlock()
	}
	stopChildren := func() {
		mu.Lock()
		cc := claudeCmd
		mu.Unlock()

		if cc != nil && cc.Process != nil {
			_ = cc.Process.Signal(syscall.SIGINT)
		}
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGINT)
		}
		if mcpCmd != nil && mcpCmd.Process != nil {
			_ = mcpCmd.Process.Signal(syscall.SIGINT)
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		fmt.Fprintf(stdout, "\nПолучен сигнал %v — останавливаем дочерние процессы...\n", sig)
		stopChildren()
	}()

	// =================================================================
	// РЕЖИМ 1: -server — только llama-server
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
	// РЕЖИМ 2 (по умолчанию): llama-server + Claude Code
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
		stopChildren()
		_ = serverProcess.Wait()
		stopMcp(mcpCmd)
		return 1
	}
	fmt.Fprintln(stdout, "llama-server готов. Запуск Claude Code...")

	// --- Локальный прокси для переключения между llama-server и OpenAI ---
	var proxySrv *http.Server
	if cfg.OpenAIBaseURL != "" && cfg.OpenAIKey != "" && cfg.OpenAIModel != "" {
		llamaTargetURL := "http://" + net.JoinHostPort(strings.Trim(healthHost, "[]"), cfg.Port)
		proxyAddr := cfg.ProxyAddr
		if proxyAddr == "" {
			proxyAddr = "127.0.0.1:8082"
		}

		fmt.Fprintf(stdout, "Запуск прокси на %s (llama → %s, openai → %s)\n",
			proxyAddr, llamaTargetURL, cfg.OpenAIBaseURL)

		var proxyLogWriter io.Writer
		if serverLogFile != nil {
			proxyLogWriter = serverLogFile
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
			stopChildren()
			_ = serverProcess.Wait()
			stopMcp(mcpCmd)
			return 1
		}
		proxySrv = srv
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = proxySrv.Shutdown(ctx)
		}()

		// Ждём готовности прокси (не sleep, а poll)
		proxyHealthURL := "http://" + proxyAddr + "/v1/models"
		if err := waitForServer(proxyHealthURL, 5*time.Second); err != nil {
			fmt.Fprintf(stderr, "прокси не поднялся: %v\n", err)
			stopChildren()
			_ = serverProcess.Wait()
			stopMcp(mcpCmd)
			return 1
		}

		baseURL = "http://" + proxyAddr
	}
	// --- MCP-конфиг для Claude Code ---
	var mcpConfigFile string
	if cfg.McpProxy {
		mcpHost, mcpPort, splitErr := net.SplitHostPort(cfg.McpAddr)
		if splitErr != nil {
			fmt.Fprintf(stderr, "некорректный -mcp-addr %q: %v\n", cfg.McpAddr, splitErr)
			stopChildren()
			_ = serverProcess.Wait()
			stopMcp(mcpCmd)
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

	// --- Поиск claude ---
	claudePath := expandHome(cfg.ClaudeBin, home)
	if resolved, lookErr := runner.LookPath(claudePath); lookErr == nil {
		claudePath = resolved
	} else if !filepath.IsAbs(claudePath) {
		fmt.Fprintf(stderr, "Claude Code не найден (%q): %v\n", cfg.ClaudeBin, lookErr)
		stopChildren()
		_ = serverProcess.Wait()
		stopMcp(mcpCmd)
		return 1
	}

	// --- Env и args для claude ---
	claudeEnv := buildClaudeEnv(cfg, baseURL, os.Environ())
	claudeArgs := buildClaudeArgs(cfg, mcpConfigFile)

	// --- stdio Claude Code: изолируем TUI от перенаправленного вывода ---
	// Claude Code — интерактивный TUI, ему нужен TTY. Если родитель пишет в
	// пайп/файл (например, `llama-runner -v 2>&1 | tee log`), claude уйдёт в
	// неинтерактивный --print и упадёт без промпта. В этом случае перенаправляем
	// stdio claude в /dev/tty, а диагностику оставляем в родительском stdout/stderr.
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

	// --- Рабочая директория claude ---
	if cfg.ClaudeDir != "" {
		absDir, dirErr := filepath.Abs(expandHome(cfg.ClaudeDir, home))
		if dirErr != nil {
			fmt.Fprintf(stderr, "не удалось определить абсолютный путь claude-dir: %v\n", dirErr)
			stopChildren()
			_ = serverProcess.Wait()
			stopMcp(mcpCmd)
			return 1
		}
		if info, statErr := os.Stat(absDir); statErr != nil || !info.IsDir() {
			fmt.Fprintf(stderr, "claude-dir %q недоступна: %v\n", absDir, statErr)
			stopChildren()
			_ = serverProcess.Wait()
			stopMcp(mcpCmd)
			return 1
		}
		cc.Dir = absDir
	} else if wd, wdErr := os.Getwd(); wdErr == nil {
		cc.Dir = wd
	}

	if cfg.Verbose {
		fmt.Fprintf(stdout, "Claude Code: %s (cwd=%s)\n", claudePath, cc.Dir)
		fmt.Fprintf(stdout, "  ANTHROPIC_BASE_URL=%s\n", baseURL)
		// Печатаем фактически установленные ANTHROPIC_* из claudeEnv, а не cfg.Alias:
		// в гибридном режиме все три DEFAULT_* ведут на внешнюю модель, а не на
		// локальный alias — иначе -v врёт.
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
		stopChildren()
		_ = serverProcess.Wait()
		stopMcp(mcpCmd)
		return 1
	}
	setClaudeCmd(cc)

	ccErr := cc.Wait()
	if ccErr != nil {
		if exitErr, ok := ccErr.(*exec.ExitError); ok {
			fmt.Fprintf(stdout, "Claude Code завершён с кодом %d. Останавливаем llama-server...\n", exitErr.ExitCode())
		} else {
			fmt.Fprintf(stderr, "Claude Code завершился с ошибкой: %v\n", ccErr)
		}
	} else {
		fmt.Fprintln(stdout, "Claude Code завершён. Останавливаем llama-server...")
	}

	if cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGINT)
	}
	_ = serverProcess.Wait()
	stopMcp(mcpCmd)
	fmt.Fprintln(stdout, "Готово.")
	return 0
}

// printUsage печатает подробную справку программы.
func printUsage(w io.Writer) {
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
  -mcp-addr string     адрес mcp-search-server (по умолчанию 127.0.0.1:8090)
  -mcp-config string   путь к MCP-конфигу (по умолчанию временный)
  -mcp-strict          передать --strict-mcp-config
  -chat-template str   шаблон чата ("auto" | "" | путь)
  -openai string       URL OpenAI-совместимого API
  -key string          API-ключ внешнего провайдера
  -model-openai str    имя модели у внешнего провайдера
  -proxy-addr string   адрес локального прокси (по умолчанию 127.0.0.1:8082)

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

  # Обновить llama.cpp
  llama-runner -update -cuda-arch 61

Совет:
  Для доступа из локальной сети используйте -host 0.0.0.0.
`)
}
