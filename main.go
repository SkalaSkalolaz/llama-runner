package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
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

	// Проверим, что бинарник существует
	if _, err := os.Stat(serverPath); err != nil {
		fmt.Fprintf(os.Stderr, "llama-server не найден по пути %q: %v\n", serverPath, err)
		os.Exit(1)
	}

	// Проверим, что модель существует
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
	// ------- Создаём команду -------
	cmd := exec.Command(serverPath, args...)

	// Рабочая директория процесса llama-server
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

	// Пробрасываем стандартные потоки
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if *verbose {
		fmt.Printf("Рабочая директория: %s\n", cmd.Dir)
		fmt.Printf("Команда: %s %s\n\n", serverPath, strings.Join(args, " "))
	} else {
		fmt.Printf("Запуск llama-server на %s:%s ...\n", *host, *port)
	}

	// ------- NEW: запускаем mcp-search-server, если включён --webui-mcp-proxy -------
	var mcpCmd *exec.Cmd
	if *mcpProxy {
		mcpCmd = exec.Command("mcp-search-server", "--addr", ":8090")
		// SEARCH_ENABLED=true передаём как переменную окружения, а не как аргумент
		mcpCmd.Env = append(os.Environ(), "SEARCH_ENABLED=true")
		mcpCmd.Stdout = os.Stdout
		mcpCmd.Stderr = os.Stderr
		// mcpCmd.Stdin = os.Stdin // обычно не требуется; если нужно — раскомментируйте
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

	// ------- Обработка сигналов -------
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		fmt.Printf("\nПолучен сигнал %v — останавливаем llama-server...\n", sig)
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGINT)
		}
		if mcpCmd != nil && mcpCmd.Process != nil {
			_ = mcpCmd.Process.Signal(syscall.SIGINT)
		}
	}()

	// ------- Ожидание завершения llama-server -------
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
}

// stopMcp корректно останавливает mcp-search-server.
func stopMcp(mcpCmd *exec.Cmd) {
	if mcpCmd == nil || mcpCmd.Process == nil {
		return
	}

	// Посылаем SIGINT, чтобы дать процессу завершиться штатно
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
func printUsage(w *os.File) {
	fmt.Fprintf(w, `llama-runner — лаунчер для llama-server

Запускает локальный сервер llama.cpp с готовым набором параметров,
корректно разворачивает ~ в путях, пробрасывает логи в терминал
и аккуратно завершает дочерний процесс по Ctrl+C / SIGTERM.

При использовании -webui-mcp-proxy дополнительно запускается
SEARCH_ENABLED=true mcp-search-server --addr :8090.

Использование:
  llama-runner [флаги]

Флаги:
  -bin string
        путь к исполняемому файлу llama-server (можно с ~)
        (по умолчанию "~/.local/bin/llama-server")
  -dir string
        рабочая директория для llama-server (по умолчанию — текущая)
  -model string
        путь к GGUF-модели (можно с ~)
        (по умолчанию "~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf")
  -host string
        адрес, на котором слушает сервер (по умолчанию "127.0.0.1")
  -port string
        порт сервера (по умолчанию "8080")

Параметры llama.cpp:
  -c string
        размер контекста, соответствует -c (по умолчанию "65536")
  -b string
        размер батча, соответствует -b (по умолчанию "2048")
  -ub string
        размер микро-батча, соответствует -ub (по умолчанию "1024")
  -fa string
        flash attention, соответствует -fa (по умолчанию "auto")
  -parallel string
        число параллельных слотов, соответствует --parallel (по умолчанию "1")
  -tools string
        режим инструментов, соответствует --tools (по умолчанию "all")
  -lm-lazy
        включить дисковый mmap с ленивым режимом
        Полезно, чтобы не грузить n-граммовую память в RAM целиком,
        а держать её отображённой на диск (по умолчанию false)
  -jinja
        включить Jinja-шаблоны чата, соответствует --jinja (по умолчанию true)
  -webui-mcp-proxy
        включает --webui-mcp-proxy для llama-server и запускает
        SEARCH_ENABLED=true mcp-search-server --addr :8090
        (по умолчанию false)

Прочее:
  -v    печатать полную команду и рабочую директорию перед запуском
  -h, --help
        показать эту справку и выйти

Примеры:
  # запуск с параметрами по умолчанию
  server

  # запуск с явной рабочей директорией
  server -dir ~/my-workspace

  # сменить порт и модель
  server -port 9090 -model ~/GGUF/other.gguf

  # посмотреть, что будет выполнено, без реального запуска
  server -v -dir ~/work

  # запуск с прокси для MCP-серверов и поисковым MCP-сервером
  server -webui-mcp-proxy

Совет:
  Если сервер должен быть доступен из локальной сети, укажите
  -host 0.0.0.0. Значение 127.0.0.1 принимает соединения только
  с этой же машины.
`)
}