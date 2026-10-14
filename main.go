package main

import (
	"flag"
	"fmt"
	"net/http" // NEW
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync" // NEW
	"syscall"
	"time"
)

func main() {
	// ------- Флаги командной строки -------
	workDir := flag.String("dir", "", "рабочая директория для llama-server (по умолчанию — текущая)")
	host := flag.String("host", "127.0.0.1", "адрес, на котором слушает сервер")
	port := flag.String("port", "8080", "порт сервера")
	model := flag.String("model", "~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf", "путь к GGUF-модели (можно с ~)")
	serverBin := flag.String("bin", "~/.local/bin/llama-server", "путь к исполняемому файлу llama-server (можно с ~)")
	ctxSize := flag.String("c", "65536", "размер контекста (-c)")
	batchSize := flag.String("b", "2048", "размер батча (-b)")
	ubatchSize := flag.String("ub", "1024", "размер микро-батча (-ub)")
	flashAttn := flag.String("fa", "auto", "flash attention (-fa)")
	parallel := flag.String("parallel", "1", "число параллельных слотов (--parallel)")
	tools := flag.String("tools", "all", "режим инструментов (--tools)")
	jinja := flag.Bool("jinja", true, "включить Jinja-шаблоны чата (--jinja)")
	mcpProxy := flag.Bool("webui-mcp-proxy", false, "включить CORS-прокси в WebUI и запустить mcp-search-server (--webui-mcp-proxy)")
	lazyMmap := flag.Bool("lm-lazy", false, "включить mmap-память с ленивым режимом (-lm mmap --lazy-mode on)")
	verbose := flag.Bool("v", false, "печатать команду перед запуском")

	// NEW: флаги, связанные с Claude Code
	serverOnly := flag.Bool("server", false,
		"запустить ТОЛЬКО llama-server (старое поведение). Без этого флага после старта сервера автоматически запускается Claude Code")
	claudeBin := flag.String("claude-bin", "claude",
		"путь к исполняемому файлу Claude Code (можно с ~)")
	claudeDir := flag.String("claude-dir", "",
		"рабочая директория для Claude Code (по умолчанию — текущая)")
	alias := flag.String("alias", "",
		"алиас модели: передаётся llama-server как --alias и в ANTHROPIC_DEFAULT_*_MODEL. Если пусто — не передаётся")
	readyTimeout := flag.Duration("ready-timeout", 180*time.Second,
		"сколько ждать готовности llama-server по /health перед запуском Claude Code")

	// ------- Кастомная справка -------
	flag.Usage = func() {
		printUsage(os.Stderr)
	}
	flag.Parse()

	if len(os.Args) == 2 && (os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "-help") {
		printUsage(os.Stdout)
		os.Exit(0)
	}

	// ------- Разворачиваем ~ в путях -------
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "не удалось получить домашний каталог: %v\n", err)
		os.Exit(1)
	}

	serverPath := expandHome(*serverBin, home)
	modelPath := expandHome(*model, home)

	if _, err := os.Stat(serverPath); err != nil {
		fmt.Fprintf(os.Stderr, "llama-server не найден по пути %q: %v\n", serverPath, err)
		os.Exit(1)
	}
	if _, err := os.Stat(modelPath); err != nil {
		fmt.Fprintf(os.Stderr, "модель не найдена по пути %q: %v\n", modelPath, err)
		os.Exit(1)
	}

	// ------- Собираем аргументы для llama-server -------
	args := []string{
		"-m", modelPath,
		"--host", *host,
		"--port", *port,
		"--tools", *tools,
		"-c", *ctxSize,
		"-b", *batchSize,
		"-ub", *ubatchSize,
		"-fa", *flashAttn,
		"--parallel", *parallel,
	}
	if *jinja {
		args = append(args, "--jinja")
	}
	if *mcpProxy {
		args = append(args, "--webui-mcp-proxy")
	}
	if *lazyMmap {
		args = append(args, "-lm", "mmap", "--lazy-mode", "on")
	}
	// NEW: алиас модели
	if *alias != "" {
		args = append(args, "--alias", *alias)
	}

	// ------- Создаём команду -------
	cmd := exec.Command(serverPath, args...)

	if *workDir != "" {
		absDir, err := filepath.Abs(expandHome(*workDir, home))
		if err != nil {
			fmt.Fprintf(os.Stderr, "не удалось определить абсолютный путь рабочей директории: %v\n", err)
			os.Exit(1)
		}
		if info, err := os.Stat(absDir); err != nil || !info.IsDir() {
			fmt.Fprintf(os.Stderr, "рабочая директория %q недоступна: %v\n", absDir, err)
			os.Exit(1)
		}
		cmd.Dir = absDir
	} else {
		if wd, err := os.Getwd(); err == nil {
			cmd.Dir = wd
		}
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if *verbose {
		fmt.Printf("Рабочая директория: %s\n", cmd.Dir)
		fmt.Printf("Команда: %s %s\n\n", serverPath, strings.Join(args, " "))
	} else {
		fmt.Printf("Запуск llama-server на %s:%s ...\n", *host, *port)
	}

	// ------- Запуск mcp-search-server при --webui-mcp-proxy -------
	var mcpCmd *exec.Cmd
	if *mcpProxy {
		mcpCmd = exec.Command("mcp-search-server", "--addr", ":8090")
		mcpCmd.Env = append(os.Environ(), "SEARCH_ENABLED=true")
		mcpCmd.Stdout = os.Stdout
		mcpCmd.Stderr = os.Stderr
		if cmd.Dir != "" {
			mcpCmd.Dir = cmd.Dir
		}
		if *verbose {
			fmt.Printf("MCP-сервер: SEARCH_ENABLED=true mcp-search-server --addr :8090\n\n")
		}
		if err := mcpCmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "ошибка запуска mcp-search-server: %v\n", err)
			os.Exit(1)
		}
	}

	// ------- Запуск llama-server -------
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "ошибка запуска сервера: %v\n", err)
		if mcpCmd != nil && mcpCmd.Process != nil {
			_ = mcpCmd.Process.Signal(syscall.SIGINT)
			_ = mcpCmd.Wait()
		}
		os.Exit(1)
	}

	// ------- CHANGED: единый менеджер дочерних процессов для сигналов -------
	// claudeCmd назначается позже (только в режиме "сервер + Claude"),
	// поэтому защищаем его мьютексом.
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
		fmt.Printf("\nПолучен сигнал %v — останавливаем дочерние процессы...\n", sig)
		stopChildren()
	}()

	// =================================================================
	// РЕЖИМ 1: -server — только llama-server (старое поведение)
	// =================================================================
	if *serverOnly {
		err = cmd.Wait()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				stopMcp(mcpCmd)
				os.Exit(exitErr.ExitCode())
			}
			fmt.Fprintf(os.Stderr, "сервер завершился с ошибкой: %v\n", err)
			stopMcp(mcpCmd)
			os.Exit(1)
		}
		fmt.Println("llama-server завершён.")
		stopMcp(mcpCmd)
		return
	}

	// =================================================================
	// РЕЖИМ 2 (по умолчанию): llama-server + Claude Code
	// =================================================================

	// 0.0.0.0 / :: нельзя использовать для HTTP-запросов — приводим к 127.0.0.1
	healthHost := *host
	if healthHost == "" || healthHost == "0.0.0.0" || healthHost == "::" {
		healthHost = "127.0.0.1"
	}
	baseURL := fmt.Sprintf("http://%s:%s", healthHost, *port)
	healthURL := baseURL + "/health"

	fmt.Printf("Ожидание готовности сервера (%s), таймаут %v ...\n", healthURL, *readyTimeout)
	if err := waitForServer(healthURL, *readyTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "ошибка: %v\n", err)
		stopChildren()
		_ = cmd.Wait()
		stopMcp(mcpCmd)
		os.Exit(1)
	}
	fmt.Println("llama-server готов. Запуск Claude Code...")

	// --- Путь к claude ---
	claudePath := expandHome(*claudeBin, home)
	if resolved, lerr := exec.LookPath(claudePath); lerr == nil {
		claudePath = resolved
	} else if !filepath.IsAbs(claudePath) {
		fmt.Fprintf(os.Stderr, "Claude Code не найден (%q): %v\n", *claudeBin, lerr)
		stopChildren()
		_ = cmd.Wait()
		stopMcp(mcpCmd)
		os.Exit(1)
	}

	// --- Окружение для Claude Code ---
	claudeEnv := append(os.Environ(),
		"ANTHROPIC_BASE_URL="+baseURL,
		"ANTHROPIC_AUTH_TOKEN=local-llama",
		"ANTHROPIC_API_KEY=local-llama",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
	)
	if *alias != "" {
		claudeEnv = append(claudeEnv,
			"ANTHROPIC_DEFAULT_SONNET_MODEL="+*alias,
			"ANTHROPIC_DEFAULT_HAIKU_MODEL="+*alias,
			"ANTHROPIC_DEFAULT_OPUS_MODEL="+*alias,
		)
	}

	// --- Создаём процесс Claude Code ---
	cc := exec.Command(claudePath)
	cc.Env = claudeEnv
	cc.Stdin = os.Stdin
	cc.Stdout = os.Stdout
	cc.Stderr = os.Stderr

	// Рабочая директория для Claude Code
	if *claudeDir != "" {
		absDir, derr := filepath.Abs(expandHome(*claudeDir, home))
		if derr != nil {
			fmt.Fprintf(os.Stderr, "не удалось определить абсолютный путь claude-dir: %v\n", derr)
			stopChildren()
			_ = cmd.Wait()
			stopMcp(mcpCmd)
			os.Exit(1)
		}
		if info, serr := os.Stat(absDir); serr != nil || !info.IsDir() {
			fmt.Fprintf(os.Stderr, "claude-dir %q недоступна: %v\n", absDir, serr)
			stopChildren()
			_ = cmd.Wait()
			stopMcp(mcpCmd)
			os.Exit(1)
		}
		cc.Dir = absDir
	} else if wd, werr := os.Getwd(); werr == nil {
		cc.Dir = wd
	}

	if *verbose {
		fmt.Printf("Claude Code: %s (cwd=%s)\n", claudePath, cc.Dir)
		fmt.Printf("  ANTHROPIC_BASE_URL=%s\n", baseURL)
		if *alias != "" {
			fmt.Printf("  ANTHROPIC_DEFAULT_SONNET_MODEL=%s\n", *alias)
		}
		fmt.Println()
	}

	if err := cc.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "ошибка запуска Claude Code: %v\n", err)
		stopChildren()
		_ = cmd.Wait()
		stopMcp(mcpCmd)
		os.Exit(1)
	}
	setClaudeCmd(cc)

	// --- Ждём завершения Claude Code ---
	ccErr := cc.Wait()
	if ccErr != nil {
		if exitErr, ok := ccErr.(*exec.ExitError); ok {
			fmt.Printf("Claude Code завершён с кодом %d. Останавливаем llama-server...\n", exitErr.ExitCode())
		} else {
			fmt.Fprintf(os.Stderr, "Claude Code завершился с ошибкой: %v\n", ccErr)
		}
	} else {
		fmt.Println("Claude Code завершён. Останавливаем llama-server...")
	}

	// --- Гасим llama-server ---
	if cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGINT)
	}
	_ = cmd.Wait()
	stopMcp(mcpCmd)
	fmt.Println("Готово.")
}

// NEW: waitForServer периодически опрашивает /health, пока сервер не ответит 200 OK,
// либо пока не истечёт timeout.
func waitForServer(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 3 * time.Second}
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("статус %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(1 * time.Second)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("таймаут")
	}
	return fmt.Errorf("сервер не стал готов за %v: %v", timeout, lastErr)
}

// stopMcp корректно останавливает mcp-search-server.
func stopMcp(mcpCmd *exec.Cmd) {
	if mcpCmd == nil || mcpCmd.Process == nil {
		return
	}
	_ = mcpCmd.Process.Signal(syscall.SIGINT)

	done := make(chan error, 1)
	go func() {
		done <- mcpCmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			if _, ok := err.(*exec.ExitError); !ok {
				fmt.Fprintf(os.Stderr, "mcp-search-server завершился с ошибкой: %v\n", err)
			}
		}
	case <-time.After(3 * time.Second):
		fmt.Fprintln(os.Stderr, "mcp-search-server не завершился за 3 секунды — убиваем")
		_ = mcpCmd.Process.Kill()
		_ = mcpCmd.Wait()
	}
}

// expandHome заменяет ведущий "~" на домашний каталог пользователя.
func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

// printUsage печатает подробную справку программы.
// CHANGED: добавлены описания новых флагов (-server, -claude-bin, -claude-dir, -alias, -ready-timeout).
func printUsage(w *os.File) {
	fmt.Fprintf(w, `llama-runner — лаунчер для llama-server и Claude Code

По умолчанию запускает локальный llama-server и, дождавшись его готовности,
поднимает Claude Code с нужными переменными окружения (ANTHROPIC_BASE_URL и др.).
Когда Claude Code завершается, llama-server автоматически останавливается.

С флагом -server запускается ТОЛЬКО llama-server (прежнее поведение).

При использовании -webui-mcp-proxy дополнительно запускается
SEARCH_ENABLED=true mcp-search-server --addr :8090.

Использование:
  server [флаги]

Основные флаги:
  -bin string
        путь к llama-server (можно с ~) (по умолчанию "~/.local/bin/llama-server")
  -model string
        путь к GGUF-модели (можно с ~)
  -host string
        адрес сервера (по умолчанию "127.0.0.1")
  -port string
        порт сервера (по умолчанию "8080")
  -dir string
        рабочая директория для llama-server (по умолчанию — текущая)
  -alias string
        алиас модели: --alias для llama-server и ANTHROPIC_DEFAULT_*_MODEL для Claude Code

Флаги Claude Code:
  -server
        запустить ТОЛЬКО llama-server, без Claude Code
  -claude-bin string
        путь к claude (по умолчанию "claude", ищется в PATH)
  -claude-dir string
        рабочая директория для Claude Code (по умолчанию — текущая)
  -ready-timeout duration
        сколько ждать готовности /health перед стартом Claude Code (по умолчанию 180s)

Параметры llama.cpp:
  -c, -b, -ub string
        размер контекста / батча / микро-батча
  -fa string
        flash attention (-fa, по умолчанию "auto")
  -parallel string
        число параллельных слотов (по умолчанию "1")
  -tools string
        режим инструментов (по умолчанию "all")
  -jinja
        включить Jinja-шаблоны чата (по умолчанию true)
  -lm-lazy
        включить mmap с ленивым режимом (-lm mmap --lazy-mode on)
  -webui-mcp-proxy
        включить --webui-mcp-proxy и поднять mcp-search-server

Прочее:
  -v    печатать полную команду и рабочую директорию перед запуском
  -h, --help
        показать эту справку и выйти

Примеры:
  # сервер + Claude Code с дефолтными параметрами
  server

  # сервер + Claude Code, алиас модели и рабочая папка проекта
  server -alias qwen3-coder -claude-dir ~/my-project

  # только сервер (как раньше)
  server -server

  # только сервер на другом порту
  server -server -port 9090

  # посмотреть, что будет выполнено
  server -v -claude-dir ~/work

  # запуск с прокси для MCP-серверов и поисковым MCP-сервером
  server -webui-mcp-proxy

Совет:
  Если сервер должен быть доступен из локальной сети, укажите
  -host 0.0.0.0. Значение 127.0.0.1 принимает соединения только
  с этой же машины.
`)
}