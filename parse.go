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

// parseFlags resolves the real user home directory and delegates to
// parseFlagsWithHome. It is the production entry point for building a Config
// from command-line arguments.
//
// On flag.ErrHelp (the -h/--help path) it returns the error unchanged; the
// caller is expected to treat it as a successful exit (code 0), since the
// help text has already been printed to stderr by FlagSet.Usage.
func parseFlags(args []string, stderr io.Writer) (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, errorf(msgErrHomeDir, err)
	}
	return parseFlagsWithHome(args, stderr, home)
}

// parseFlagsWithHome builds a Config from command-line arguments using an
// explicit home directory, so tests never touch the real
// ~/.config/llama-runner/config.json.
//
// Layered resolution, later layers overriding earlier ones:
// built-in defaults → config-file root section → selected profile (-p) →
// CLI flags. Environment variables take part only in API-key selection,
// handled separately by resolveOpenAIKey.
//
// Resolution steps:
//  1. -config and -p are located by a pre-scan (findFlag) before FlagSet
//     parsing, because they determine where defaults come from;
//  2. the config file is loaded unless disabled with -config ""; an
//     explicitly named file that does not exist is an error, while a missing
//     default-path file is normal;
//  3. -p without a loaded file is an error — silently ignoring it would make
//     -doctor report a profile that run() never applied;
//  4. alias and chat-template are special: being set by the file counts as
//     "explicit" too (AliasSet/ChatTemplateSet), otherwise run() would
//     overwrite the alias with "local" and a file-provided template would not
//     be validated as explicit.
//
// Returns flag.ErrHelp (after printing localized usage to stderr) when the
// user asks for help, and a descriptive error for bad flag syntax or missing
// configuration.
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
			return nil, errorv(msgErrConfigFileNotFound, expandHome(effectivePath, home))
		}
	}

	// -p set but config not loaded (no file / -config "") —
	// nothing to apply the profile from. Cannot ignore silently: doctor
	// would then lie about "profile: coder", and run() uses defaults.
	if profileName != "" && fc == nil {
		return nil, errorv(msgErrProfileNoConfig, profileName)
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
				return nil, errorv(msgErrProfileNotFound, profileName)
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
	fs.Usage = func() { printUsageForLocale(stderr) }

	fs.StringVar(&cfg.WorkDir, "dir", cfg.WorkDir, T(msgFlagDir))
	fs.StringVar(&cfg.Host, "host", cfg.Host, T(msgFlagHost))
	fs.StringVar(&cfg.Port, "port", cfg.Port, T(msgFlagPort))
	fs.StringVar(&cfg.Model, "model", cfg.Model, T(msgFlagModel))
	fs.StringVar(&cfg.ServerBin, "bin", cfg.ServerBin, T(msgFlagBin))
	fs.StringVar(&cfg.CtxSize, "c", cfg.CtxSize, T(msgFlagC))
	fs.StringVar(&cfg.BatchSize, "b", cfg.BatchSize, T(msgFlagB))
	fs.StringVar(&cfg.UbatchSize, "ub", cfg.UbatchSize, T(msgFlagUb))
	fs.StringVar(&cfg.FlashAttn, "fa", cfg.FlashAttn, T(msgFlagFa))
	fs.StringVar(&cfg.Parallel, "parallel", cfg.Parallel, T(msgFlagParallel))
	fs.StringVar(&cfg.Tools, "tools", cfg.Tools, T(msgFlagTools))
	fs.BoolVar(&cfg.Jinja, "jinja", cfg.Jinja, T(msgFlagJinja))
	fs.BoolVar(&cfg.McpProxy, "webui-mcp-proxy", cfg.McpProxy, T(msgFlagMcpProxy))
	fs.BoolVar(&cfg.LazyMmap, "lm-lazy", cfg.LazyMmap, T(msgFlagLazyMmap))
	fs.BoolVar(&cfg.Verbose, "v", cfg.Verbose, T(msgFlagVerbose))

	fs.StringVar(&cfg.NGL, "ngl", cfg.NGL, T(msgFlagNgl))
	fs.StringVar(&cfg.Mmproj, "mmproj", cfg.Mmproj, T(msgFlagMmproj))
	fs.StringVar(&cfg.Temp, "temp", cfg.Temp, T(msgFlagTemp))
	fs.StringVar(&cfg.TopP, "top-p", cfg.TopP, T(msgFlagTopP))
	fs.StringVar(&cfg.CacheTypeK, "cache-type-k", cfg.CacheTypeK, T(msgFlagCacheK))
	fs.StringVar(&cfg.CacheTypeV, "cache-type-v", cfg.CacheTypeV, T(msgFlagCacheV))
	fs.StringVar(&cfg.FitMode, "fit", cfg.FitMode, T(msgFlagFit))
	fs.StringVar(&cfg.LogVerbosity, "lv", cfg.LogVerbosity, T(msgFlagLv))
	fs.StringVar(&cfg.Reasoning, "reasoning", cfg.Reasoning, T(msgFlagReasoning))
	fs.StringVar(&cfg.ReasoningBudget, "reasoning-budget", cfg.ReasoningBudget, T(msgFlagReasoningBudget))
	fs.StringVar(&cfg.SpecType, "spec-type", cfg.SpecType, T(msgFlagSpecType))
	fs.StringVar(&cfg.SpecDraftModel, "spec-draft-model", cfg.SpecDraftModel, T(msgFlagSpecDraftModel))
	fs.StringVar(&cfg.SpecDraftNMax, "spec-draft-n-max", cfg.SpecDraftNMax, T(msgFlagSpecDraftNMax))
	fs.StringVar(&cfg.SpecDraftNGL, "spec-draft-ngl", cfg.SpecDraftNGL, T(msgFlagSpecDraftNGL))

	fs.BoolVar(&cfg.UpdateFlag, "update", false, T(msgFlagUpdate))
	fs.StringVar(&cfg.LlamaSrc, "llama-src", cfg.LlamaSrc, T(msgFlagLlamaSrc))
	fs.StringVar(&cfg.InstallPrefix, "install-prefix", cfg.InstallPrefix, T(msgFlagInstallPrefix))
	fs.BoolVar(&cfg.WithCuda, "cuda", cfg.WithCuda, T(msgFlagCuda))
	fs.StringVar(&cfg.CudaArch, "cuda-arch", cfg.CudaArch, T(msgFlagCudaArch))

	fs.BoolVar(&cfg.ServerOnly, "server", false, T(msgFlagServer))
	fs.StringVar(&cfg.ClaudeBin, "claude-bin", cfg.ClaudeBin, T(msgFlagClaudeBin))
	fs.StringVar(&cfg.ClaudeDir, "claude-dir", cfg.ClaudeDir, T(msgFlagClaudeDir))
	fs.StringVar(&cfg.Alias, "alias", cfg.Alias, T(msgFlagAlias))
	fs.DurationVar(&cfg.ReadyTimeout, "ready-timeout", cfg.ReadyTimeout, T(msgFlagReadyTimeout))
	fs.StringVar(&cfg.ChatTemplate, "chat-template", cfg.ChatTemplate, T(msgFlagChatTemplate))
	fs.StringVar(&cfg.LogFile, "log-file", cfg.LogFile, T(msgFlagLogFile))
	fs.BoolVar(&cfg.QuietServer, "quiet-server", cfg.QuietServer, T(msgFlagQuietServer))
	fs.IntVar(&cfg.LogMaxMB, "log-max-mb", cfg.LogMaxMB, T(msgFlagLogMaxMB))
	fs.IntVar(&cfg.LogKeep, "log-keep", cfg.LogKeep, T(msgFlagLogKeep))
	fs.StringVar(&cfg.McpAddr, "mcp-addr", cfg.McpAddr, T(msgFlagMcpAddr))
	fs.StringVar(&cfg.McpConfigOut, "mcp-config", cfg.McpConfigOut, T(msgFlagMcpConfig))
	fs.BoolVar(&cfg.McpStrict, "mcp-strict", cfg.McpStrict, T(msgFlagMcpStrict))

	fs.BoolVar(&cfg.VersionFlag, "version", false, T(msgFlagVersion))

	fs.StringVar(&cfg.OpenAIBaseURL, "openai", cfg.OpenAIBaseURL, T(msgFlagOpenai))
	fs.StringVar(&cfg.OpenAIKey, "key", cfg.OpenAIKey, T(msgFlagKey))
	fs.StringVar(&cfg.OpenAIModel, "model-openai", cfg.OpenAIModel, T(msgFlagModelOpenai))
	fs.StringVar(&cfg.ProxyAddr, "proxy-addr", cfg.ProxyAddr, T(msgFlagProxyAddr))
	fs.StringVar(&cfg.ProxyToken, "proxy-token", cfg.ProxyToken, T(msgFlagProxyToken))
	fs.BoolVar(&cfg.ProxyAllowRemote, "proxy-allow-remote", cfg.ProxyAllowRemote, T(msgFlagProxyAllowRem))

	// M6: config, profile, doctor.
	fs.StringVar(&cfg.ConfigPath, "config", cfg.ConfigPath, T(msgFlagConfig))
	fs.StringVar(&cfg.Profile, "p", cfg.Profile, T(msgFlagP))
	fs.BoolVar(&cfg.DoctorFlag, "doctor", false, T(msgFlagDoctor))

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

// validateConfig checks cross-field consistency of a resolved Config before
// any process is started. It returns the first problem found:
//
//   - -ready-timeout must be strictly positive;
//   - -host must be non-blank;
//   - -port must be a number in 1..65535;
//   - -mcp-addr must be non-blank and, when the MCP proxy is enabled, a
//     valid host:port pair;
//   - -reasoning, when set, must be one of on, off, auto;
//   - -reasoning-budget, when set, must be an integer >= -1
//     (-1 = unlimited, 0 = disabled, N = token budget);
//   - the external OpenAI provider is all-or-nothing: supplying any of
//     -openai, -key or -model-openai requires all three, -openai must be a
//     parseable absolute URL, and -proxy-addr (if set) must be a valid
//     host:port pair.
func validateConfig(cfg *Config) error {
	if cfg.ReadyTimeout <= 0 {
		return errorv(msgErrReadyTimeoutPositive)
	}
	if strings.TrimSpace(cfg.Host) == "" {
		return errorv(msgErrHostEmpty)
	}
	portNum, err := strconv.Atoi(cfg.Port)
	if err != nil || portNum < 1 || portNum > 65535 {
		return errorv(msgErrPortBad, cfg.Port)
	}
	if strings.TrimSpace(cfg.McpAddr) == "" {
		return errorv(msgErrMcpAddrEmpty)
	}
	if cfg.McpProxy {
		if _, _, err := net.SplitHostPort(cfg.McpAddr); err != nil {
			return errorv(msgErrMcpAddrHostPort, cfg.McpAddr)
		}
	}

	if cfg.Reasoning != "" {
		switch cfg.Reasoning {
		case "on", "off", "auto":
		default:
			return errorv(msgErrReasoning, cfg.Reasoning)
		}
	}
	if cfg.ReasoningBudget != "" {
		n, err := strconv.Atoi(cfg.ReasoningBudget)
		if err != nil || n < -1 {
			return errorv(msgErrReasoningBudget, cfg.ReasoningBudget)
		}
	}
	if cfg.OpenAIBaseURL != "" || cfg.OpenAIKey != "" || cfg.OpenAIModel != "" {
		if cfg.OpenAIBaseURL == "" {
			return errorv(msgErrOpenaiRequired)
		}
		if cfg.OpenAIKey == "" {
			return errorv(msgErrKeyRequired)
		}
		if cfg.OpenAIModel == "" {
			return errorv(msgErrModelOpenaiRequired)
		}
		if _, err := url.ParseRequestURI(cfg.OpenAIBaseURL); err != nil {
			return errorv(msgErrOpenaiURLBad, cfg.OpenAIBaseURL, err)
		}
		if cfg.ProxyAddr != "" {
			if _, _, err := net.SplitHostPort(cfg.ProxyAddr); err != nil {
				return errorv(msgErrProxyAddrBad, cfg.ProxyAddr)
			}
		}
	}
	return nil
}

// resolveChatTemplate turns the raw -chat-template value into the path (or
// "") actually used.
//
// Any value other than the literal "auto" — an explicit path or an empty
// string meaning "no external template" — is returned unchanged. For "auto"
// the model file name is inspected: if its base name contains "qwen"
// (case-insensitively), the fallback path is selected; otherwise "" is
// returned, letting llama-server use the template embedded in the GGUF.
//
// fallback is the auto-mode target path, configurable via
// Config.ChatTemplateFallback (previously hardcoded), so it is supplied by
// the caller rather than being a constant here.
func resolveChatTemplate(raw, modelPath, fallback string) string {
	if raw != "auto" {
		return raw
	}
	if strings.Contains(strings.ToLower(filepath.Base(modelPath)), "qwen") {
		return fallback
	}
	return ""
}

// validateTemplatePath expands raw (which may start with "~") to an absolute
// path and checks that it refers to a regular file, returning that absolute
// path.
//
// An empty raw is always valid and yields ("", nil). When the file is
// missing (or is not a regular file), behaviour depends on explicit:
// a template the user named via -chat-template/chatTemplate is a hard error,
// while an auto-detected one degrades quietly to ("", nil) so llama-server
// falls back to the template embedded in the GGUF.
func validateTemplatePath(raw, home string, explicit bool) (string, error) {
	if raw == "" {
		return "", nil
	}
	p, err := filepath.Abs(expandHome(raw, home))
	if err != nil {
		return "", errorf(msgErrTemplateAbs, err)
	}
	info, statErr := os.Stat(p)
	if statErr != nil || !info.Mode().IsRegular() {
		if explicit {
			return "", errorv(msgErrTemplateMissing, p, statErr)
		}
		return "", nil
	}
	return p, nil
}

// ServerPaths bundles the absolute, existence-verified file paths that
// llama-server is launched with. Empty Mmproj, Draft and Template mean
// "not configured / not used"; Model is always set on a successful launch
// path (see resolvePaths).
type ServerPaths struct {
	// Model is the GGUF model file passed as -m.
	Model string
	// Mmproj is the multimodal projector passed as --mmproj ("" = none).
	Mmproj string
	// Draft is the speculative-decoding draft model passed as
	// --spec-draft-model ("" = none).
	Draft string
	// Template is the external chat template passed as
	// --chat-template-file ("" = use the template embedded in the GGUF).
	Template string
}

// buildServerArgs assembles the llama-server command line from cfg and the
// verified paths in ServerPaths.
//
// Positional/core arguments (model, host, port, tools, context, batch sizes,
// flash attention, parallel slots) are always present. Everything else is
// emitted only when configured: boolean switches (--jinja,
// --webui-mcp-proxy, lazy mmap) when enabled, and optional string parameters
// (-ngl, --temp, --top-p, cache types, --fit, -lv, reasoning controls,
// speculative-decoding knobs) when non-empty. Values are passed through
// verbatim — validation happens earlier in validateConfig.
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

// buildClaudeEnv returns a fresh environment slice (KEY=VALUE strings) for
// the Claude Code child process. baseEnv is copied, never mutated.
//
// The returned environment:
//   - points Claude Code at baseURL via ANTHROPIC_BASE_URL;
//   - authenticates with cfg.ProxyToken (or the historical default
//     "local-llama") in BOTH ANTHROPIC_AUTH_TOKEN and ANTHROPIC_API_KEY,
//     because different Claude Code versions send Authorization: Bearer or
//     x-api-key and the proxy accepts either;
//   - sets CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 to keep a local
//     deployment from phoning home;
//   - maps the standard model aliases: in hybrid mode (external provider
//     configured) SONNET/HAIKU/OPUS all resolve to the external model and the
//     local llama-server is exposed as the single custom entry in the /model
//     menu; in local mode they resolve to cfg.Alias when one is configured
//     (run() supplies the default "local" unless the alias was set
//     explicitly).
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

// buildClaudeArgs returns the command-line arguments for the claude child
// process. When mcpConfigPath is set, Claude Code is pointed at that MCP
// configuration file, and --strict-mcp-config is appended when
// cfg.McpStrict is set (restricting Claude Code to exactly the servers in
// that file). With no MCP config the result is an empty slice.
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

// credentialsPath returns the file path where llama-runner looks for the
// external provider key when neither the -key flag nor a supporting
// environment variable provides one:
// ~/.config/llama-runner/credentials.
func credentialsPath(home string) string {
	return filepath.Join(home, ".config", "llama-runner", "credentials")
}

// resolveOpenAIKey selects the external provider's API key, checking sources
// in strict priority order and returning the first non-empty hit:
//
//	-key flag > LLAMA_RUNNER_OPENAI_KEY > OPENAI_API_KEY >
//	credentials file > "" (no key).
//
// The credentials file (~/.config/llama-runner/credentials) is a simple
// KEY=VALUE list, one pair per line, with "#" comments and optional
// single/double quotes around values; the keys OPENAI_KEY and
// OPENAI_API_KEY are recognised. If the file's permissions are wider than
// 0600 the key is still accepted, but a warning is written to stderr.
//
// A missing, unreadable or malformed file is not fatal — it simply means
// "no key found" — though read failures do produce a stderr warning.
//
// env and the file system are accessed through injected dependencies
// (env is a lookup function; in production os.Getenv is passed) so tests can
// exercise every priority branch without touching the real home directory.
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
			fmt.Fprint(stderr, TF(msgWarnCredCheck, path, err))
		}
		return "", nil
	}
	info, err := os.Stat(path)
	if err == nil && info.Mode().Perm() != 0o600 {
		fmt.Fprint(stderr, TF(msgWarnCredPerms, path, info.Mode().Perm()))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprint(stderr, TF(msgWarnCredRead, path, err))
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
