package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// parseFlags public wrapper: resolves the real home directory.
func parseFlags(args []string, stderr io.Writer) (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("не удалось получить домашний каталог: %w", err)
	}
	return parseFlagsWithHome(args, stderr, home)
}

// parseFlagsWithHome testable version with explicit home: not dependent on
// the real ~/.config/llama-runner/config.json.
//
// Priority: CLI > file > built-in defaults. Env is used only for the key,
// separately in resolveOpenAIKey (M3).
func parseFlagsWithHome(args []string, stderr io.Writer, home string) (*Config, error) {
	// 1. Early search for -config and -p: they define the defaults source.
	configPath, configSet := findFlag(args, "config")
	profileName, _ := findFlag(args, "p")
	// 2. Read config file (if not explicitly disabled with -config "").
	// configExplicit — path set explicitly via -config (not ""), file is required.
	configExplicit := configSet && configPath != ""
	var fc *fileConfig
	if !configSet || configPath != "" {
		effectivePath := configPath
		if !configSet {
			effectivePath = defaultConfigPath(home)
		}
		var loadErr error
		fc, loadErr = loadConfigFile(effectivePath, home)
		if loadErr != nil {
			return nil, loadErr
		}
		// Explicit -config but no file — error. For the default
		// path, a missing file is normal (most users have no config).
		if fc == nil && configExplicit {
			return nil, fmt.Errorf("конфиг-файл %q не найден", expandHome(effectivePath, home))
		}
	}

	// -p set but config not loaded (no file / -config "") —
	// nothing to apply the profile from. Cannot ignore silently: doctor
	// would then lie about "profile: coder", and run() uses defaults.
	if profileName != "" && fc == nil {
		return nil, fmt.Errorf("профиль %q не найден: конфиг-файл отсутствует", profileName)
	}

	// 3. Assemble Config: built-in defaults → root file → profile.

	cfg := defaultConfig()
	if fc != nil {
		if err := applyFileConfig(cfg, *fc); err != nil {
			return nil, err
		}
		// File set a value — this is also "explicit", otherwise run() replaces
		// alias with "local" and validateTemplatePath won't count
		// template as explicit.
		if fc.Alias != nil {
			cfg.AliasSet = true
		}
		if fc.ChatTemplate != nil {
			cfg.ChatTemplateSet = true
		}
		if profileName != "" {
			p, ok := fc.Profiles[profileName]
			if !ok {
				return nil, fmt.Errorf("профиль %q не найден в конфиге", profileName)
			}
			if err := applyFileConfig(cfg, p); err != nil {
				return nil, err
			}
			if p.Alias != nil {
				cfg.AliasSet = true
			}
			if p.ChatTemplate != nil {
				cfg.ChatTemplateSet = true
			}
		}
	}
	cfg.ConfigPath = configPath
	cfg.Profile = profileName

	// 4. Flags. cfg values already contain file-or-default;
	//    flag.StringVar(ptr, name, default, ...) — default = current value,
	//    CLI overrides it if the flag is set.
	fs := flag.NewFlagSet("llama-runner", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { printUsage(stderr) }

	fs.StringVar(&cfg.WorkDir, "dir", cfg.WorkDir, "рабочая директория...")
	fs.StringVar(&cfg.Host, "host", cfg.Host, "адрес...")
	fs.StringVar(&cfg.Port, "port", cfg.Port, "порт...")
	fs.StringVar(&cfg.Model, "model", cfg.Model, "путь к GGUF...")
	fs.StringVar(&cfg.ServerBin, "bin", cfg.ServerBin, "путь к llama-server...")
	fs.StringVar(&cfg.CtxSize, "c", cfg.CtxSize, "размер контекста (-c)")
	fs.StringVar(&cfg.BatchSize, "b", cfg.BatchSize, "размер батча (-b)")
	fs.StringVar(&cfg.UbatchSize, "ub", cfg.UbatchSize, "размер микро-батча (-ub)")
	fs.StringVar(&cfg.FlashAttn, "fa", cfg.FlashAttn, "flash attention (-fa)")
	fs.StringVar(&cfg.Parallel, "parallel", cfg.Parallel, "число параллельных слотов")
	fs.StringVar(&cfg.Tools, "tools", cfg.Tools, "режим инструментов")
	fs.BoolVar(&cfg.Jinja, "jinja", cfg.Jinja, "включить Jinja-шаблоны")
	fs.BoolVar(&cfg.McpProxy, "webui-mcp-proxy", cfg.McpProxy, "включить CORS-прокси")
	fs.BoolVar(&cfg.LazyMmap, "lm-lazy", cfg.LazyMmap, "mmap с ленивым режимом")
	fs.BoolVar(&cfg.Verbose, "v", cfg.Verbose, "печатать команду")

	fs.StringVar(&cfg.NGL, "ngl", cfg.NGL, "число слоёв на GPU")
	fs.StringVar(&cfg.Mmproj, "mmproj", cfg.Mmproj, "путь к mmproj")
	fs.StringVar(&cfg.Temp, "temp", cfg.Temp, "температура")
	fs.StringVar(&cfg.TopP, "top-p", cfg.TopP, "top-p")
	fs.StringVar(&cfg.CacheTypeK, "cache-type-k", cfg.CacheTypeK, "тип K-кэша")
	fs.StringVar(&cfg.CacheTypeV, "cache-type-v", cfg.CacheTypeV, "тип V-кэша")
	fs.StringVar(&cfg.FitMode, "fit", cfg.FitMode, "режим --fit")
	fs.StringVar(&cfg.LogVerbosity, "lv", cfg.LogVerbosity, "уровень логов (-lv)")
	fs.StringVar(&cfg.Reasoning, "reasoning", cfg.Reasoning,
		"режим размышлений: on|off|auto (пусто — значение llama-server по умолчанию)")
	fs.StringVar(&cfg.ReasoningBudget, "reasoning-budget", cfg.ReasoningBudget,
		"бюджет токенов на размышления: -1 (без лимита), 0 (отключить), N (положительное число)")
	fs.StringVar(&cfg.SpecType, "spec-type", cfg.SpecType, "тип spec-декодирования")
	fs.StringVar(&cfg.SpecDraftModel, "spec-draft-model", cfg.SpecDraftModel, "путь к драфтеру")
	fs.StringVar(&cfg.SpecDraftNMax, "spec-draft-n-max", cfg.SpecDraftNMax, "n-max драфтера")
	fs.StringVar(&cfg.SpecDraftNGL, "spec-draft-ngl", cfg.SpecDraftNGL, "ngl драфтера")

	fs.BoolVar(&cfg.UpdateFlag, "update", false, "обновить llama.cpp")
	fs.StringVar(&cfg.LlamaSrc, "llama-src", cfg.LlamaSrc, "путь к исходникам")
	fs.StringVar(&cfg.InstallPrefix, "install-prefix", cfg.InstallPrefix, "префикс установки")
	fs.BoolVar(&cfg.WithCuda, "cuda", cfg.WithCuda, "собирать с CUDA")
	fs.StringVar(&cfg.CudaArch, "cuda-arch", cfg.CudaArch, "архитектура CUDA")

	fs.BoolVar(&cfg.ServerOnly, "server", false, "только llama-server")
	fs.StringVar(&cfg.ClaudeBin, "claude-bin", cfg.ClaudeBin, "путь к claude")
	fs.StringVar(&cfg.ClaudeDir, "claude-dir", cfg.ClaudeDir, "рабочая директория claude")
	fs.StringVar(&cfg.Alias, "alias", cfg.Alias, "алиас модели")
	fs.DurationVar(&cfg.ReadyTimeout, "ready-timeout", cfg.ReadyTimeout, "таймаут готовности")
	fs.StringVar(&cfg.ChatTemplate, "chat-template", cfg.ChatTemplate, "шаблон чата")
	fs.StringVar(&cfg.LogFile, "log-file", cfg.LogFile, "лог-файл")
	fs.BoolVar(&cfg.QuietServer, "quiet-server", cfg.QuietServer, "уводить логи в файл")
	fs.IntVar(&cfg.LogMaxMB, "log-max-mb", cfg.LogMaxMB,
		"макс. размер лога llama-server в МБ перед ротацией (0 — не ротировать)")
	fs.IntVar(&cfg.LogKeep, "log-keep", cfg.LogKeep,
		"сколько архивов лога хранить (0 — без архивов)")
	fs.StringVar(&cfg.McpAddr, "mcp-addr", cfg.McpAddr, "адрес mcp-search-server")
	fs.StringVar(&cfg.McpConfigOut, "mcp-config", cfg.McpConfigOut, "куда записать MCP-конфиг")
	fs.BoolVar(&cfg.McpStrict, "mcp-strict", cfg.McpStrict, "strict-mcp-config")

	fs.BoolVar(&cfg.VersionFlag, "version", false, "показать версию")

	fs.StringVar(&cfg.OpenAIBaseURL, "openai", cfg.OpenAIBaseURL, "URL OpenAI-совместимого API")
	fs.StringVar(&cfg.OpenAIKey, "key", cfg.OpenAIKey, "API-ключ для внешнего провайдера")
	fs.StringVar(&cfg.OpenAIModel, "model-openai", cfg.OpenAIModel, "Название модели у внешнего провайдера")
	fs.StringVar(&cfg.ProxyAddr, "proxy-addr", cfg.ProxyAddr, "адрес локального прокси (гибридный режим)")
	fs.StringVar(&cfg.ProxyToken, "proxy-token", cfg.ProxyToken,
		"токен для аутентификации входящих запросов прокси (пусто — без проверки)")
	fs.BoolVar(&cfg.ProxyAllowRemote, "proxy-allow-remote", cfg.ProxyAllowRemote,
		"разрешить не-loopback -proxy-addr без предупреждения")

	// M6: config, profile, doctor.
	fs.StringVar(&cfg.ConfigPath, "config", cfg.ConfigPath,
		"путь к конфиг-файлу (\"\" — не читать)")
	fs.StringVar(&cfg.Profile, "p", cfg.Profile, "профиль из конфиг-файла")
	fs.BoolVar(&cfg.DoctorFlag, "doctor", false,
		"показать разрешённые пути и версию, не запуская процессы")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "chat-template":
			cfg.ChatTemplateSet = true
		case "alias":
			cfg.AliasSet = true
		}
	})
	return cfg, nil
}

// validateConfig validates parameters before starting processes.
func validateConfig(cfg *Config) error {
	if cfg.ReadyTimeout <= 0 {
		return fmt.Errorf("-ready-timeout должен быть больше нуля")
	}
	if strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("-host не может быть пустым")
	}
	portNum, err := strconv.Atoi(cfg.Port)
	if err != nil || portNum < 1 || portNum > 65535 {
		return fmt.Errorf("некорректный -port %q: ожидается число от 1 до 65535", cfg.Port)
	}
	if strings.TrimSpace(cfg.McpAddr) == "" {
		return fmt.Errorf("-mcp-addr не может быть пустым")
	}
	if cfg.McpProxy {
		if _, _, err := net.SplitHostPort(cfg.McpAddr); err != nil {
			return fmt.Errorf("некорректный -mcp-addr %q: используйте host:port", cfg.McpAddr)
		}
	}

	if cfg.Reasoning != "" {
		switch cfg.Reasoning {
		case "on", "off", "auto":
		default:
			return fmt.Errorf("invalid -reasoning %q: expect on, off or auto", cfg.Reasoning)
		}
	}
	if cfg.ReasoningBudget != "" {
		n, err := strconv.Atoi(cfg.ReasoningBudget)
		if err != nil || n < -1 {
			return fmt.Errorf("invalid -reasoning-budget %q: expect -1, 0 or a positive number", cfg.ReasoningBudget)
		}
	}
	if cfg.OpenAIBaseURL != "" || cfg.OpenAIKey != "" || cfg.OpenAIModel != "" {
		if cfg.OpenAIBaseURL == "" {
			return fmt.Errorf("-openai обязателен при использовании -key или -model-openai")
		}
		if cfg.OpenAIKey == "" {
			return fmt.Errorf("-key обязателен при использовании -openai")
		}
		if cfg.OpenAIModel == "" {
			return fmt.Errorf("-model-openai обязателен при использовании -openai")
		}
		if _, err := url.ParseRequestURI(cfg.OpenAIBaseURL); err != nil {
			return fmt.Errorf("некорректный -openai URL %q: %v", cfg.OpenAIBaseURL, err)
		}
		if cfg.ProxyAddr != "" {
			if _, _, err := net.SplitHostPort(cfg.ProxyAddr); err != nil {
				return fmt.Errorf("некорректный -proxy-addr %q: используйте host:port", cfg.ProxyAddr)
			}
		}
	}
	return nil
}

// resolveChatTemplate selects the final template path (may be "").
// fallback is the auto-mode path, extracted to config (M6), so it is not
// hardcoded. Auto-detect by `qwen` substring stays the default.
func resolveChatTemplate(raw, modelPath, fallback string) string {
	if raw != "auto" {
		return raw
	}
	if strings.Contains(strings.ToLower(filepath.Base(modelPath)), "qwen") {
		return fallback
	}
	return ""
}

// validateTemplatePath checks that path is a regular file.
// explicit==true and file missing → error; otherwise ("" , nil).
func validateTemplatePath(raw, home string, explicit bool) (string, error) {
	if raw == "" {
		return "", nil
	}
	p, err := filepath.Abs(expandHome(raw, home))
	if err != nil {
		return "", fmt.Errorf("не удалось определить абсолютный путь chat-template: %w", err)
	}
	info, statErr := os.Stat(p)
	if statErr != nil || !info.Mode().IsRegular() {
		if explicit {
			return "", fmt.Errorf("указанный -chat-template недоступен или не является обычным файлом: %q (%v)", p, statErr)
		}
		return "", nil
	}
	return p, nil
}

type ServerPaths struct {
	Model    string
	Mmproj   string
	Draft    string
	Template string
}

// buildServerArgs builds args for llama-server. All paths are already verified.
func buildServerArgs(cfg *Config, paths ServerPaths) []string {
	args := []string{
		"-m", paths.Model,
		"--host", cfg.Host,
		"--port", cfg.Port,
		"--tools", cfg.Tools,
		"-c", cfg.CtxSize,
		"-b", cfg.BatchSize,
		"-ub", cfg.UbatchSize,
		"-fa", cfg.FlashAttn,
		"--parallel", cfg.Parallel,
	}
	if cfg.Jinja {
		args = append(args, "--jinja")
	}
	if cfg.McpProxy {
		args = append(args, "--webui-mcp-proxy")
	}
	if cfg.LazyMmap {
		args = append(args, "-lm", "mmap", "--lazy-mode", "on")
	}
	if cfg.Alias != "" {
		args = append(args, "--alias", cfg.Alias)
	}
	if cfg.NGL != "" {
		args = append(args, "-ngl", cfg.NGL)
	}
	if cfg.Temp != "" {
		args = append(args, "--temp", cfg.Temp)
	}
	if cfg.TopP != "" {
		args = append(args, "--top-p", cfg.TopP)
	}
	if cfg.CacheTypeK != "" {
		args = append(args, "--cache-type-k", cfg.CacheTypeK)
	}
	if cfg.CacheTypeV != "" {
		args = append(args, "--cache-type-v", cfg.CacheTypeV)
	}
	if cfg.FitMode != "" {
		args = append(args, "--fit", cfg.FitMode)
	}
	if cfg.LogVerbosity != "" {
		args = append(args, "-lv", cfg.LogVerbosity)
	}
	if cfg.Reasoning != "" {
		args = append(args, "--reasoning", cfg.Reasoning)
	}
	if cfg.ReasoningBudget != "" {
		args = append(args, "--reasoning-budget", cfg.ReasoningBudget)
	}
	if paths.Mmproj != "" {
		args = append(args, "--mmproj", paths.Mmproj)
	}

	if cfg.SpecType != "" {
		args = append(args, "--spec-type", cfg.SpecType)
	}
	if cfg.SpecDraftNMax != "" {
		args = append(args, "--spec-draft-n-max", cfg.SpecDraftNMax)
	}
	if cfg.SpecDraftNGL != "" {
		args = append(args, "--spec-draft-ngl", cfg.SpecDraftNGL)
	}
	if paths.Draft != "" {
		args = append(args, "--spec-draft-model", paths.Draft)
	}
	if paths.Template != "" {
		args = append(args, "--chat-template-file", paths.Template)
	}
	return args
}

// buildClaudeEnv assembles env for Claude Code (does not mutate baseEnv).
func buildClaudeEnv(cfg *Config, baseURL string, baseEnv []string) []string {
	env := append([]string{}, baseEnv...)

	// Proxy token: -proxy-token if set, otherwise historical
	// local-llama. We put it in both fields — different versions of Claude Code send
	// different ones: Authorization: Bearer (AUTH_TOKEN) or x-api-key (API_KEY).
	// The proxy accepts both variants.
	authToken := "local-llama"
	if cfg.ProxyToken != "" {
		authToken = cfg.ProxyToken
	}
	env = append(env,
		"ANTHROPIC_BASE_URL="+baseURL,
		"ANTHROPIC_AUTH_TOKEN="+authToken,
		"ANTHROPIC_API_KEY="+authToken,
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
	)
	// Local model name: cfg.Alias (already "local" by default from run())
	localModel := cfg.Alias
	if localModel == "" {
		localModel = "local"
	}
	if cfg.OpenAIBaseURL != "" && cfg.OpenAIModel != "" {
		// In hybrid mode all standard aliases point to the external provider.
		env = append(env,
			"ANTHROPIC_DEFAULT_SONNET_MODEL="+cfg.OpenAIModel,
			"ANTHROPIC_DEFAULT_HAIKU_MODEL="+cfg.OpenAIModel,
			"ANTHROPIC_DEFAULT_OPUS_MODEL="+cfg.OpenAIModel,
			// llama-server added as the sole custom option in the /model menu.
			"ANTHROPIC_CUSTOM_MODEL_OPTION=llama-server",
			"ANTHROPIC_CUSTOM_MODEL_OPTION_NAME=llama-server (local)",
			"ANTHROPIC_CUSTOM_MODEL_OPTION_DESCRIPTION=Route to local llama-server",
		)
	} else if cfg.Alias != "" {
		env = append(env,
			"ANTHROPIC_DEFAULT_SONNET_MODEL="+cfg.Alias,
			"ANTHROPIC_DEFAULT_HAIKU_MODEL="+cfg.Alias,
			"ANTHROPIC_DEFAULT_OPUS_MODEL="+cfg.Alias,
		)
	}
	return env
}

// buildClaudeArgs returns arguments for claude.
func buildClaudeArgs(cfg *Config, mcpConfigPath string) []string {
	args := []string{}
	if mcpConfigPath != "" {
		args = append(args, "--mcp-config", mcpConfigPath)
		if cfg.McpStrict {
			args = append(args, "--strict-mcp-config")
		}
	}
	return args
}

// credentialsPath returns where to look for the key if neither flag nor env is set.
func credentialsPath(home string) string {
	return filepath.Join(home, ".config", "llama-runner", "credentials")
}

// resolveOpenAIKey selects the key by priority:
//
//	flag -key > LLAMA_RUNNER_OPENAI_KEY > OPENAI_API_KEY >
//	~/.config/llama-runner/credentials > "".
//
// The file is a simple KEY=VALUE (one pair per line); searches OPENAI_KEY
// or OPENAI_API_KEY. Permissions are checked: if the file is wider than 0600,
// the key is still read but a warning is written to stderr. Unreadable/broken file —
// not fatal (just the next source), but a warning.
//
// env and statFile are injected for tests (real: os.Getenv,
// os.Stat/os.ReadFile).
func resolveOpenAIKey(keyFlag string, env func(string) string, home string, stderr io.Writer) (string, error) {
	if keyFlag != "" {
		return keyFlag, nil
	}
	for _, name := range []string{"LLAMA_RUNNER_OPENAI_KEY", "OPENAI_API_KEY"} {
		if v := strings.TrimSpace(env(name)); v != "" {
			return v, nil
		}
	}
	path := credentialsPath(home)
	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(stderr, "предупреждение: не удалось проверить %q: %v\n", path, err)
		}
		return "", nil
	}
	info, err := os.Stat(path)
	if err == nil && info.Mode().Perm() != 0o600 {
		fmt.Fprintf(stderr, "предупреждение: %q имеет права %o, рекомендуется 0600\n",
			path, info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "предупреждение: не удалось прочитать %q: %v\n", path, err)
		return "", nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if key != "OPENAI_KEY" && key != "OPENAI_API_KEY" {
			continue
		}
		val := strings.TrimSpace(line[eq+1:])
		val = strings.Trim(val, `"'`)
		if val != "" {
			return val, nil
		}
	}
	return "", nil
}
