package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var version = "1.3"

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

	// Дополнительные параметры llama.cpp (передаются, только если заданы)
	ngl := flag.String("ngl", "", "число слоёв модели на GPU (-ngl), например 999. Не передаётся, если пусто")
	mmproj := flag.String("mmproj", "", "путь к мультимодальному проектору (--mmproj, можно с ~). Не передаётся, если пусто")
	temp := flag.String("temp", "", "температура сэмплинга (--temp). Не передаётся, если пусто")
	topP := flag.String("top-p", "", "top-p сэмплинг (--top-p). Не передаётся, если пусто")
	cacheTypeK := flag.String("cache-type-k", "", "тип K-кэша (--cache-type-k), напр. q8_0. Не передаётся, если пусто")
	cacheTypeV := flag.String("cache-type-v", "", "тип V-кэша (--cache-type-v), напр. q8_0. Не передаётся, если пусто")
    // Управление диагностикой и подбором памяти llama.cpp
    fitMode := flag.String("fit", "", "режим подбора параметров под память (--fit on|off). Не передаётся, если пусто")
    logVerbosity := flag.String("lv", "", "уровень детализации логов llama.cpp (-lv N, 0..5). Не передаётся, если пусто")
	// Спекулятивное декодирование (draft-модель)
	specType := flag.String("spec-type", "", "тип спекулятивного декодирования (--spec-type), напр. draft-dflash. Не передаётся, если пусто")
	specDraftModel := flag.String("spec-draft-model", "", "путь к драфтер-модели (--spec-draft-model, можно с ~). Не передаётся, если пусто")
	specDraftNMax := flag.String("spec-draft-n-max", "", "макс. число черновых токенов драфтера (--spec-draft-n-max). Не передаётся, если пусто")
	specDraftNGL := flag.String("spec-draft-ngl", "", "число слоёв драфтера на GPU (--spec-draft-ngl). Не передаётся, если пусто")

	// Флаги обновления llama.cpp (режим -update)
	updateFlag := flag.Bool("update", false,
		"обновить llama.cpp: git pull + cmake configure/build/install, затем выйти (без запуска сервера)")
	llamaSrc := flag.String("llama-src", "~/src/llama.cpp",
		"путь к исходникам llama.cpp (для -update, можно с ~)")
	installPrefix := flag.String("install-prefix", "~/.local",
		"префикс установки для -DCMAKE_INSTALL_PREFIX (для -update, можно с ~)")
	withCuda := flag.Bool("cuda", true,
		"собирать с CUDA (-DGGML_CUDA=ON) (для -update)")
	cudaArch := flag.String("cuda-arch", "61",
		"архитектура CUDA для -DCMAKE_CUDA_ARCHITECTURES (для -update). \"\" — не передавать флаг (CMake выберет default)")

	// Флаги, связанные с Claude Code
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
	chatTemplate := flag.String("chat-template", "auto",
		`шаблон чата (можно с ~). "auto" (по умолчанию) — если имя файла модели содержит "qwen" (без учёта регистра), использовать ~/GGUF/qwen-fixed-chat-template.jinja, иначе шаблон не передаётся вовсе. Явный "" — не передавать шаблон. Явный путь — использовать его.`)
	logFile := flag.String("log-file", "~/.cache/llama-runner/llama-server.log",
		"файл для логов llama-server и mcp-search-server в режиме Claude Code (можно с ~; права 0600)")
	quietServer := flag.Bool("quiet-server", true,
		"не выводить логи llama-server в терминал при работе с Claude Code (пишет в -log-file)")
	mcpAddr := flag.String("mcp-addr", "127.0.0.1:8090",
		"адрес mcp-search-server в формате host:port")
	mcpConfigOut := flag.String("mcp-config", "",
		"куда записать временный MCP-конфиг для Claude Code. По умолчанию — во временный каталог ОС")
	mcpStrict := flag.Bool("mcp-strict", false,
		"передать Claude Code --strict-mcp-config: использовать только MCP из этого лаунчера, игнорировать ~/.claude.json")
	versionFlag := flag.Bool("version", false, "показать версию и выйти")

	// ------- Кастомная справка -------
	flag.Usage = func() {
		printUsage(os.Stderr)
	}
	flag.Parse()

	if *versionFlag {
		fmt.Println(version)
		os.Exit(0)
	}

	if len(os.Args) == 2 && (os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "-help") {
		printUsage(os.Stdout)
		os.Exit(0)
	}

	// Если запуск не в режиме -server и -alias не задан явно, подставляем "local"
	aliasSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "alias" {
			aliasSet = true
		}
	})
	if !aliasSet && !*serverOnly {
		*alias = "local"
	}

	// ------- Разворачиваем ~ -------
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "не удалось получить домашний каталог: %v\n", err)
		os.Exit(1)
	}

	// Проверяем параметры до запуска внешних процессов.
	if *readyTimeout <= 0 {
		fatalf("-ready-timeout должен быть больше нуля")
	}
	if strings.TrimSpace(*host) == "" {
		fatalf("-host не может быть пустым")
	}
	portNum, parseErr := strconv.Atoi(*port)
	if parseErr != nil || portNum < 1 || portNum > 65535 {
		fatalf("некорректный -port %q: ожидается число от 1 до 65535", *port)
	}
	if strings.TrimSpace(*mcpAddr) == "" {
		fatalf("-mcp-addr не может быть пустым")
	}
	if *mcpProxy {
		if _, _, err := net.SplitHostPort(*mcpAddr); err != nil {
			fatalf("некорректный -mcp-addr %q: используйте host:port", *mcpAddr)
		}
	}
	chatTemplateExplicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "chat-template" {
			chatTemplateExplicit = true
		}
	})

	// =================================================================
	// РЕЖИМ -update: обновляем и пересобираем llama.cpp, затем выходим.
	// =================================================================
	if *updateFlag {
		srcAbs := expandHome(*llamaSrc, home)
		installAbs := expandHome(*installPrefix, home)
		if err := updateLlamaCPP(srcAbs, installAbs, *cudaArch, *withCuda, *verbose); err != nil {
			fmt.Fprintf(os.Stderr, "ошибка обновления llama.cpp: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("llama.cpp успешно обновлён и установлен.")
		return
	}

	// ------- Проверяем пути -------
	serverPath, err := filepath.Abs(expandHome(*serverBin, home))
	if err != nil {
		fatalf("не удалось определить абсолютный путь llama-server: %v", err)
	}
	modelPath, err := filepath.Abs(expandHome(*model, home))
	if err != nil {
		fatalf("не удалось определить абсолютный путь модели: %v", err)
	}

	if _, err := os.Stat(serverPath); err != nil {
		fmt.Fprintf(os.Stderr, "llama-server не найден по пути %q: %v\n", serverPath, err)
		os.Exit(1)
	}
	if _, err := os.Stat(modelPath); err != nil {
		fmt.Fprintf(os.Stderr, "модель не найдена по пути %q: %v\n", modelPath, err)
		os.Exit(1)
	}

	// ------- Автоопределение chat-template -------
	// "auto": Qwen (по имени файла) → дефолтный qwen-шаблон, иначе — без шаблона.
	templateToUse := *chatTemplate
	if templateToUse == "auto" {
		modelBase := strings.ToLower(filepath.Base(modelPath))
		if strings.Contains(modelBase, "qwen") {
			templateToUse = "~/GGUF/qwen-fixed-chat-template.jinja"
			if *verbose {
				fmt.Printf("Автоопределение шаблона: имя модели содержит \"qwen\" → %s\n", templateToUse)
			}
		} else {
			templateToUse = ""
			if *verbose {
				fmt.Printf("Автоопределение шаблона: модель не Qwen → шаблон не передаётся (--chat-template \"\")\n")
			}
		}
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
	if *alias != "" {
		args = append(args, "--alias", *alias)
	}
	if *ngl != "" {
		args = append(args, "-ngl", *ngl)
	}
	if *temp != "" {
		args = append(args, "--temp", *temp)
	}
	if *topP != "" {
		args = append(args, "--top-p", *topP)
	}
	if *cacheTypeK != "" {
		args = append(args, "--cache-type-k", *cacheTypeK)
	}
	if *cacheTypeV != "" {
		args = append(args, "--cache-type-v", *cacheTypeV)
	}
	if *fitMode != "" {
		args = append(args, "--fit", *fitMode)
	}
	if *logVerbosity != "" {
		args = append(args, "-lv", *logVerbosity)
	}
	if *mmproj != "" {
		mmprojPath, pathErr := filepath.Abs(expandHome(*mmproj, home))
		if pathErr != nil {
			fatalf("не удалось определить абсолютный путь mmproj: %v", pathErr)
		}
		if _, err := os.Stat(mmprojPath); err != nil {
			fmt.Fprintf(os.Stderr, "mmproj не найден по пути %q: %v\n", mmprojPath, err)
			os.Exit(1)
		}
		args = append(args, "--mmproj", mmprojPath)
	}
	if *specType != "" {
		args = append(args, "--spec-type", *specType)
	}
	if *specDraftNMax != "" {
		args = append(args, "--spec-draft-n-max", *specDraftNMax)
	}
	if *specDraftNGL != "" {
		args = append(args, "--spec-draft-ngl", *specDraftNGL)
	}
	if *specDraftModel != "" {
		draftPath, pathErr := filepath.Abs(expandHome(*specDraftModel, home))
		if pathErr != nil {
			fatalf("не удалось определить абсолютный путь spec-draft-model: %v", pathErr)
		}
		if _, err := os.Stat(draftPath); err != nil {
			fmt.Fprintf(os.Stderr, "spec-draft-model не найден по пути %q: %v\n", draftPath, err)
			os.Exit(1)
		}
		args = append(args, "--spec-draft-model", draftPath)
	}
	// chat-template: templateToUse уже развёрнут из "auto" выше.
	if templateToUse != "" {
		templatePath, pathErr := filepath.Abs(expandHome(templateToUse, home))
		if pathErr != nil {
			fatalf("не удалось определить абсолютный путь chat-template: %v", pathErr)
		}
		info, statErr := os.Stat(templatePath)
		if statErr != nil || !info.Mode().IsRegular() {
			if chatTemplateExplicit {
				fatalf("указанный -chat-template недоступен или не является обычным файлом: %q (%v)", templatePath, statErr)
			}
			fmt.Fprintf(os.Stderr, "предупреждение: автоматически выбранный шаблон %q недоступен (%v); использую шаблон из GGUF, если он есть\n", templatePath, statErr)
		} else {
			args = append(args, "--chat-template-file", templatePath)
			if *verbose {
				fmt.Printf("Chat template: %s\n", templatePath)
			}
		}
	}

	// ------- Создаём команду -------
	cmd := exec.Command(serverPath, args...)
    // Подкладываем соседний lib-каталог для llama-server, чтобы он находил
    // свои .so (libggml*, backend-библиотеки) даже если ldconfig-кэш устарел
    // или ~/.local/lib не прописан в /etc/ld.so.conf.d.
    if cmd.Env == nil {
        cmd.Env = os.Environ()
    }
    for _, sub := range []string{"lib", "lib64"} {
        cand := filepath.Join(filepath.Dir(serverPath), "..", sub)
        abs, err := filepath.Abs(cand)
        if err != nil {
            continue
        }
        if info, err := os.Stat(abs); err != nil || !info.IsDir() {
            continue
        }
        oldLP := os.Getenv("LD_LIBRARY_PATH")
        newLP := abs
        if oldLP != "" {
            newLP = abs + ":" + oldLP
        }
        cmd.Env = append(cmd.Env, "LD_LIBRARY_PATH="+newLP)
        if *verbose {
            fmt.Printf("LD_LIBRARY_PATH для llama-server: %s\n", newLP)
        }
        break
    }
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

	var serverLogFile *os.File
	if !*serverOnly && *quietServer {
		logPath := expandHome(*logFile, home)
		if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
			fmt.Fprintf(os.Stderr, "не удалось создать каталог для логов %q: %v\n", filepath.Dir(logPath), err)
			os.Exit(1)
		}
		f, lerr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if lerr != nil {
			fmt.Fprintf(os.Stderr, "не удалось открыть лог-файл %q: %v\n", logPath, lerr)
			os.Exit(1)
		}
		if err := f.Chmod(0o600); err != nil {
			_ = f.Close()
			fatalf("не удалось установить права доступа к лог-файлу %q: %v", logPath, err)
		}
		serverLogFile = f
		defer serverLogFile.Close()
		cmd.Stdout = f
		cmd.Stderr = f
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	cmd.Stdin = os.Stdin

	if *verbose {
		fmt.Printf("Рабочая директория: %s\n", cmd.Dir)
		fmt.Printf("Команда: %s %s\n\n", serverPath, strings.Join(args, " "))
		if serverLogFile != nil {
			fmt.Printf("Логи llama-server: %s\n", serverLogFile.Name())
		}
	} else {
		fmt.Printf("Запуск llama-server на %s:%s ...\n", *host, *port)
		if serverLogFile != nil {
			fmt.Printf("Логи llama-server: %s\n", serverLogFile.Name())
		}
	}

	// ------- mcp-search-server -------
	var mcpCmd *exec.Cmd
	if *mcpProxy {
		mcpCmd = exec.Command("mcp-search-server", "--addr", *mcpAddr)
		mcpCmd.Env = append(os.Environ(), "SEARCH_ENABLED=true")
		if serverLogFile != nil {
			mcpCmd.Stdout = serverLogFile
			mcpCmd.Stderr = serverLogFile
		} else {
			mcpCmd.Stdout = os.Stdout
			mcpCmd.Stderr = os.Stderr
		}
		if cmd.Dir != "" {
			mcpCmd.Dir = cmd.Dir
		}
		if *verbose {
			fmt.Printf("MCP-сервер: SEARCH_ENABLED=true mcp-search-server --addr %s\n\n", *mcpAddr)
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
	serverProcess := trackProcess(cmd)

	// ------- Менеджер дочерних процессов для сигналов -------
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
	// РЕЖИМ 1: -server — только llama-server
	// =================================================================
	if *serverOnly {
		err = serverProcess.Wait()
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
	healthHost := *host
	if healthHost == "" || healthHost == "0.0.0.0" || healthHost == "::" {
		healthHost = "127.0.0.1"
	}
	baseURL := "http://" + net.JoinHostPort(strings.Trim(healthHost, "[]"), *port)
	healthURL := baseURL + "/health"

	fmt.Printf("Ожидание готовности сервера (%s), таймаут %v ...\n", healthURL, *readyTimeout)
	if err := waitForServerProcess(healthURL, *readyTimeout, serverProcess); err != nil {
		fmt.Fprintf(os.Stderr, "ошибка: %v\n", err)
		stopChildren()
		_ = serverProcess.Wait()
		stopMcp(mcpCmd)
		os.Exit(1)
	}
	fmt.Println("llama-server готов. Запуск Claude Code...")
    var (
        mcpEndpointURL string
        mcpConfigFile  string
    )
	if *mcpProxy {
		mcpHost, mcpPort, splitErr := net.SplitHostPort(*mcpAddr)
		if splitErr != nil {
			fatalf("некорректный -mcp-addr %q: %v", *mcpAddr, splitErr)
		}

        mcpURL := "http://" + net.JoinHostPort(strings.Trim(mcpHost, "[]"), mcpPort)
        mcpHealthURL := mcpURL + "/health"
        // Claude Code подключается через Streamable HTTP, а не SSE.
        mcpEndpointURL = mcpURL + "/mcp"

		fmt.Printf("Ожидание готовности MCP-сервера (%s) ...\n", mcpHealthURL)
		if err := waitForServer(mcpHealthURL, 15*time.Second); err != nil {
			fmt.Fprintf(os.Stderr,
				"предупреждение: mcp-search-server не готов: %v\n"+
					"Claude Code будет запущен без web_search.\n", err)
		} else {
			requestedConfigPath := expandHome(*mcpConfigOut, home)
            path, werr := writeMCPConfig(requestedConfigPath, mcpEndpointURL)
			if werr != nil {
				fmt.Fprintf(os.Stderr,
					"предупреждение: не удалось создать MCP-конфиг: %v\n", werr)
			} else {
				mcpConfigFile = path
				// Удаляем только временный файл, созданный лаунчером.
				if requestedConfigPath == "" {
					defer os.Remove(mcpConfigFile)
				}
                if *verbose {
                    fmt.Printf("MCP-конфиг: %s (url=%s, type=http)\n", path, mcpEndpointURL)
                }
            }
		}
	}

	claudePath := expandHome(*claudeBin, home)
	if resolved, lerr := exec.LookPath(claudePath); lerr == nil {
		claudePath = resolved
	} else if !filepath.IsAbs(claudePath) {
		fmt.Fprintf(os.Stderr, "Claude Code не найден (%q): %v\n", *claudeBin, lerr)
		stopChildren()
		_ = serverProcess.Wait()
		stopMcp(mcpCmd)
		os.Exit(1)
	}

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

	claudeArgs := []string{}
	if mcpConfigFile != "" {
		claudeArgs = append(claudeArgs, "--mcp-config", mcpConfigFile)
		if *mcpStrict {
			claudeArgs = append(claudeArgs, "--strict-mcp-config")
		}
	}

	cc := exec.Command(claudePath, claudeArgs...)
	cc.Env = claudeEnv
	if *verbose && len(claudeArgs) > 0 {
		fmt.Printf("Claude Code args: %v\n", claudeArgs)
	}
	cc.Stdin = os.Stdin
	cc.Stdout = os.Stdout
	cc.Stderr = os.Stderr

	if *claudeDir != "" {
		absDir, derr := filepath.Abs(expandHome(*claudeDir, home))
		if derr != nil {
			fmt.Fprintf(os.Stderr, "не удалось определить абсолютный путь claude-dir: %v\n", derr)
			stopChildren()
			_ = serverProcess.Wait()
			stopMcp(mcpCmd)
			os.Exit(1)
		}
		if info, serr := os.Stat(absDir); serr != nil || !info.IsDir() {
			fmt.Fprintf(os.Stderr, "claude-dir %q недоступна: %v\n", absDir, serr)
			stopChildren()
			_ = serverProcess.Wait()
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
		_ = serverProcess.Wait()
		stopMcp(mcpCmd)
		os.Exit(1)
	}
	setClaudeCmd(cc)

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

	if cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGINT)
	}
	_ = serverProcess.Wait()
	stopMcp(mcpCmd)
	fmt.Println("Готово.")
}

// updateLlamaCPP обновляет исходники llama.cpp и пересобирает/переустанавливает их.
func updateLlamaCPP(srcPath, installPrefix, cudaArch string, withCuda, verbose bool) error {
	var err error
	srcPath, err = filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("не удалось определить абсолютный путь исходников: %w", err)
	}
	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("исходники llama.cpp не найдены по пути %q: %w", srcPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("путь %q не является каталогом", srcPath)
	}

	upstreamCheck := exec.Command("git", "-C", srcPath,
		"rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err := upstreamCheck.Run(); err != nil {
		fmt.Println("  (у текущей ветки нет upstream — git pull пропущен, используем текущий код)")
	} else {
		pullArgs := []string{"-C", srcPath, "pull", "--ff-only"}
		if verbose {
			fmt.Printf("  git %s\n", strings.Join(pullArgs, " "))
		}
		pull := exec.Command("git", pullArgs...)
		pull.Stdout = os.Stdout
		pull.Stderr = os.Stderr
		if err := pull.Run(); err != nil {
			return fmt.Errorf("git pull: %w", err)
		}
	}


	installAbs, err := filepath.Abs(installPrefix)
	if err != nil {
		return fmt.Errorf("не удалось определить абсолютный install-prefix: %w", err)
	}
	cmakeArgs := []string{
		"-S", srcPath,
		"-B", filepath.Join(srcPath, "build"),
		"-DCMAKE_BUILD_TYPE=Release",
		"-DCMAKE_INSTALL_PREFIX=" + installAbs,
	}
	if withCuda {
		cmakeArgs = append(cmakeArgs, "-DGGML_CUDA=ON")
		if cudaArch != "" {
			cmakeArgs = append(cmakeArgs, "-DCMAKE_CUDA_ARCHITECTURES="+cudaArch)
		} else {
			cmakeArgs = append(cmakeArgs, "-U", "CMAKE_CUDA_ARCHITECTURES")
		}
	} else {
		// Явно выключаем CUDA: одного отсутствия флага недостаточно,
		// если в существующем build-каталоге сохранён старый cache.
		cmakeArgs = append(cmakeArgs, "-DGGML_CUDA=OFF", "-U", "CMAKE_CUDA_ARCHITECTURES")
	}
	if verbose {
		fmt.Printf("  cmake %s\n", strings.Join(cmakeArgs, " "))
	}
	configure := exec.Command("cmake", cmakeArgs...)
	configure.Dir = srcPath
	configure.Stdout = os.Stdout
	configure.Stderr = os.Stderr
	if err := configure.Run(); err != nil {
		return fmt.Errorf("cmake configure: %w", err)
	}

	jobs := runtime.NumCPU()
	fmt.Printf("Сборка (%d потоков)...\n", jobs)
	buildArgs := []string{"--build", filepath.Join(srcPath, "build"), "--config", "Release", "-j", fmt.Sprintf("%d", jobs)}
	if verbose {
		fmt.Printf("  cmake %s\n", strings.Join(buildArgs, " "))
	}
	build := exec.Command("cmake", buildArgs...)
	build.Dir = srcPath
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("cmake build: %w", err)
	}

	fmt.Printf("Установка в %s...\n", installPrefix)
	installArgs := []string{"--install", filepath.Join(srcPath, "build")}
	if verbose {
		fmt.Printf("  cmake %s\n", strings.Join(installArgs, " "))
	}
	install := exec.Command("cmake", installArgs...)
	install.Dir = srcPath
	install.Stdout = os.Stdout
	install.Stderr = os.Stderr
	if err := install.Run(); err != nil {
		return fmt.Errorf("cmake install: %w", err)
	}

	return nil
}

// trackedProcess централизует Wait: для каждого дочернего процесса Wait вызывается ровно один раз.
type trackedProcess struct {
	done chan struct{}
	err  error
}

func trackProcess(cmd *exec.Cmd) *trackedProcess {
	p := &trackedProcess{done: make(chan struct{})}
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()
	return p
}

func (p *trackedProcess) Wait() error {
	<-p.done
	return p.err
}

// waitForServerProcess прекращает ожидание готовности, если процесс сервера завершился раньше.
func waitForServerProcess(url string, timeout time.Duration, process *trackedProcess) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	client := &http.Client{Timeout: 3 * time.Second}
	var lastErr error
	check := func() (bool, error) {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			return false, nil
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true, nil
		}
		lastErr = fmt.Errorf("статус %d", resp.StatusCode)
		return false, nil
	}
	for {
		select {
		case <-process.done:
			return fmt.Errorf("llama-server завершился до готовности: %v", process.Wait())
		default:
		}
		ready, _ := check()
		if ready {
			return nil
		}
		select {
		case <-process.done:
			return fmt.Errorf("llama-server завершился до готовности: %v", process.Wait())
		case <-deadline.C:
			if lastErr == nil {
				lastErr = fmt.Errorf("таймаут")
			}
			return fmt.Errorf("сервер не стал готов за %v: %v", timeout, lastErr)
		case <-ticker.C:
		}
	}
}

// waitForServer периодически опрашивает /health, пока сервер не ответит 200 OK.
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
		select {
		case err := <-done:
			if err != nil {
				if _, ok := err.(*exec.ExitError); !ok {
					fmt.Fprintf(os.Stderr, "ошибка ожидания mcp-search-server: %v\n", err)
				}
			}
		case <-time.After(3 * time.Second):
			fmt.Fprintln(os.Stderr, "предупреждение: процесс mcp-search-server не подтвердил завершение после Kill")
		}
	}
}

func writeMCPConfig(path, httpURL string) (string, error) {
	temporary := path == ""
	if temporary {
		f, err := os.CreateTemp("", "llama-runner-mcp-*.json")
		if err != nil {
			return "", err
		}
		path = f.Name()
		if err := f.Close(); err != nil {
			_ = os.Remove(path)
			return "", err
		}
	} else {
		dir := filepath.Dir(path)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return "", err
			}
		}
	}

    cfg := map[string]any{
        "mcpServers": map[string]any{
            "search": map[string]any{
                "type": "http",
                "url":  httpURL,
            },
        },
    }
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		if temporary {
			_ = os.Remove(path)
		}
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		if temporary {
			_ = os.Remove(path)
		}
		return "", err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		if temporary {
			_ = os.Remove(path)
		}
		return "", err
	}
	return path, nil
}

// fatalf выводит диагностическое сообщение и завершает программу с кодом 2.
func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ошибка: "+format+"\n", args...)
	os.Exit(2)
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
	fmt.Fprintf(w, `llama-runner — лаунчер для llama-server и Claude Code

По умолчанию запускает локальный llama-server и, дождавшись его готовности,
поднимает Claude Code с нужными переменными окружения (ANTHROPIC_BASE_URL и др.).
Когда Claude Code завершается, llama-server автоматически останавливается.

С флагом -server запускается ТОЛЬКО llama-server (прежнее поведение).
С флагом -update обновляется llama.cpp (git pull + сборка + установка), затем выход.

При использовании -webui-mcp-proxy дополнительно запускается
SEARCH_ENABLED=true mcp-search-server --addr :8090.

Использование:
  server [флаги]

Автоопределение chat-template (по умолчанию -chat-template "auto"):
  - если имя файла модели содержит "qwen" (без учёта регистра) —
    используется ~/GGUF/qwen-fixed-chat-template.jinja;
  - иначе шаблон не передаётся вовсе (эквивалент -chat-template "").
  Явное значение -chat-template (путь или "") имеет приоритет.

Обновление llama.cpp:
  -update
        обновить llama.cpp: git pull --ff-only, cmake configure/build/install,
        затем выйти. Сервер не запускается. Удобно комбинировать:
          server -update && server -server -model ~/GGUF/model.gguf
  -llama-src string
        путь к исходникам llama.cpp (по умолчанию "~/src/llama.cpp")
  -install-prefix string
        префикс для -DCMAKE_INSTALL_PREFIX (по умолчанию "~/.local")
  -cuda
        собирать с CUDA, т.е. -DGGML_CUDA=ON (по умолчанию true).
        Установите -cuda=false для CPU-only сборки.
  -cuda-arch string
        значение для -DCMAKE_CUDA_ARCHITECTURES. По умолчанию "61"
        (Pascal, GTX 1080 Ti). Укажите иную (например "75" для Turing,
        "86" для Ampere). Пустая строка "" — не передавать флаг вовсе
        (CMake возьмёт свой default; закешированное значение сбрасывается).

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
        алиас модели: --alias для llama-server и ANTHROPIC_DEFAULT_*_MODEL для Claude Code.
        Если не указан и не задан -server, по умолчанию используется "local".

Флаги Claude Code:
  -server
        запустить ТОЛЬКО llama-server, без Claude Code
  -claude-bin string
        путь к claude (по умолчанию "claude", ищется в PATH)
  -claude-dir string
        рабочая директория для Claude Code (по умолчанию — текущая)
  -ready-timeout duration
        сколько ждать готовности /health перед стартом Claude Code (по умолчанию 180s)
  -log-file string
        файл для логов llama-server и mcp-search-server в режиме Claude Code
        (по умолчанию "~/.cache/llama-runner/llama-server.log")
  -quiet-server
        уводить логи llama-server в файл, а не в терминал (по умолчанию true)
  -mcp-addr string
        адрес mcp-search-server (host:port, по умолчанию 127.0.0.1:8090)
  -mcp-config string
        путь к MCP-конфигу для Claude Code; заданный файл сохраняется после выхода
  -mcp-strict
        передать Claude Code --strict-mcp-config
  -chat-template string
        шаблон чата (можно с ~). "auto" (по умолчанию) — Qwen: ~/GGUF/qwen-fixed-chat-template.jinja,
        иначе шаблон не передаётся. Явный "" — без шаблона. Явный путь — использовать его.

Параметры llama.cpp:
  -c, -b, -ub string
        размер контекста / батча / микро-батча (по умолчанию 65536/2048/1024)
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

Дополнительные параметры llama.cpp (передаются, только если заданы):
  -ngl string
        число слоёв модели на GPU (-ngl), например 999
  -mmproj string
        путь к мультимодальному проектору (--mmproj)
  -temp string
        температура сэмплинга (--temp)
  -top-p string
        top-p сэмплинг (--top-p)
  -cache-type-k string
        тип K-кэша (--cache-type-k), например q8_0
  -cache-type-v string
        тип V-кэша (--cache-type-v), например q8_0
  -fit string
        режим подбора параметров под доступную память (--fit on|off).
        Пример: -fit off — отключить автоподбор (полезно при отладке
        падений в llama.cpp). Не передаётся, если пусто.
  -lv string
        уровень детализации логов llama.cpp (-lv N, где N=0..5).
        Пример: -lv 4 — максимально подробный лог (нужен для диагностики
        MTP/DFlash: видно n_draft, n_accept, accept_rate).
        Не путать с -v — флагом самого лаунчера.

Спекулятивное декодирование (передаётся, только если задано):
  -spec-type string
        тип спекулятивного декодирования (--spec-type), например draft-dflash
  -spec-draft-model string
        путь к драфтер-модели (--spec-draft-model)
  -spec-draft-n-max string
        макс. число черновых токенов драфтера (--spec-draft-n-max)
  -spec-draft-ngl string
        число слоёв драфтера на GPU (--spec-draft-ngl)

Прочее:
  -v    печатать полную команду и рабочую директорию перед запуском
  -version
        показать версию и выйти
  -h, --help
        показать эту справку и выйти

Примеры:
  # обновить llama.cpp (архитектура по умолчанию — "61") и выйти
  server -update

  # обновить для Turing (sm_75)
  server -update -cuda-arch 75

  # обновить без явной архитектуры (CMake выберет default)
  server -update -cuda-arch ""

  # обновить, затем запустить сервер Nemotron (без LiLiCorr)
  server -update && server -server \
    -model ~/GGUF/NVIDIA-Nemotron-3.5-Bolt-74B-A9B-24GB.gguf \
    -c 16384 -b 2048 -ub 512 -fa on -jinja
    # chat-template можно не указывать — автоопределение само отключит шаблон

  # сервер + Claude Code с дефолтными параметрами
  server

  # только сервер
  server -server

  # Nemotron с LiLiCorr-драфтером
  server -server \
    -model  ~/models/nemotron-bolt/NVIDIA-Nemotron-3.5-Bolt-74B-A9B-24GB.gguf \
    -mmproj ~/models/nemotron-bolt/mmproj-NVIDIA-Nemotron-3.5-Bolt-74B-A9B-24GB.gguf \
    -spec-type draft-dflash \
    -spec-draft-model ~/models/nemotron-bolt/NVIDIA-Nemotron-3.5-Bolt-74B-A9B-24GB-LiLiCorr.gguf \
    -spec-draft-n-max 6 -spec-draft-ngl 999 -ngl 999 \
    -c 98304 -b 2048 -ub 512 \
    -cache-type-k q8_0 -cache-type-v q8_0 \
    -temp 1.0 -top-p 0.95 -fa on -jinja

Совет:
  Если сервер должен быть доступен из локальной сети, укажите
  -host 0.0.0.0. Значение 127.0.0.1 принимает соединения только
  с этой же машины.
`)
}
