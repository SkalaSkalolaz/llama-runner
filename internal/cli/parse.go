package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// ParseFlags resolves the real user home directory and delegates to
// ParseFlagsWithHome. It is the production entry point for building a config.Config
// from command-line arguments.
//
// On flag.ErrHelp (the -h/--help path) it returns the error unchanged; the
// caller is expected to treat it as a successful exit (code 0), since the
// help text has already been printed to stderr by FlagSet.Usage.
func ParseFlags(args []string, stderr io.Writer) (*config.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, i18n.Errorf(i18n.MsgErrHomeDir, err)
	}
	return ParseFlagsWithHome(args, stderr, home)
}

// ParseFlagsWithHome builds a config.Config from command-line arguments using an
// explicit home directory, so tests never touch the real
// ~/.config/llama-runner/config.json.
//
// Layered resolution, later layers overriding earlier ones:
// built-in defaults → config-file root section → selected profile (-p) →
// CLI flags. Environment variables take part only in API-key selection,
// handled separately by ResolveOpenAIKey.
//
// Resolution steps:
//  1. -config and -p are located by a pre-scan (FindFlag) before FlagSet
//     parsing, because they determine where defaults come from;
//  2. the config file is loaded unless disabled with -config ""; an
//     explicitly named file that does not exist is an error, while a missing
//     default-path file is normal;
//  3. -p without a loaded file is an error — silently ignoring it would make
//     -doctor report a profile that run() never applied;
//  4. alias and chat-template are special: being set by the file counts as
//     "explicit" too (AliasSet/ChatTemplateSet), otherwise a file-provided
//     alias would be replaced by derived/default naming and a file-provided
//     template would not be validated as explicit. Flags additionally record
//     their own origin: -model sets ModelSet (which invalidates a stale
//     file/profile alias in favour of a name derived from the chosen GGUF)
//     and -alias sets AliasFromCLI (which outranks that derivation).
//
// Returns flag.ErrHelp (after printing localized usage to stderr) when the
// user asks for help, and a descriptive error for bad flag syntax or missing
// configuration.
func ParseFlagsWithHome(args []string, stderr io.Writer, home string) (*config.Config, error) {
	// 1. Early search for -config and -p: they define the defaults source.
	configPath, configSet := config.FindFlag(args, "config")
	profileName, _ := config.FindFlag(args, "p")
	// 2. Read config file (if not explicitly disabled with -config "").
	// configExplicit — path set explicitly via -config (not ""), file is required.
	configExplicit := configSet && configPath != ""
	var fc *config.FileConfig
	if !configSet || configPath != "" {
		effectivePath := configPath
		if !configSet {
			effectivePath = config.DefaultConfigPath(home)
		}
		var loadErr error
		fc, loadErr = config.LoadConfigFile(effectivePath, home)
		if loadErr != nil {
			return nil, loadErr
		}
		// Explicit -config but no file — error. For the default
		// path, a missing file is normal (most users have no config).
		if fc == nil && configExplicit {
			return nil, i18n.Errorv(i18n.MsgErrConfigFileNotFound, config.ExpandHome(effectivePath, home))
		}
	}

	// -p set but config not loaded (no file / -config "") —
	// nothing to apply the profile from. Cannot ignore silently: doctor
	// would then lie about "profile: coder", and run() uses defaults.
	if profileName != "" && fc == nil {
		return nil, i18n.Errorv(i18n.MsgErrProfileNoConfig, profileName)
	}

	// 3. Assemble config.Config: built-in defaults → root file → profile.

	cfg := config.DefaultConfig()
	if fc != nil {
		if err := config.ApplyFileConfig(cfg, *fc); err != nil {
			return nil, err
		}
		// File set a value — this is also "explicit", otherwise run() replaces
		// alias with "local" and ValidateTemplatePath won't count
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
				return nil, i18n.Errorv(i18n.MsgErrProfileNotFound, profileName)
			}
			// mode belongs to the profile alone: a root planner must not
			// leak into a worker selected with -p.
			cfg.Mode = ""
			if err := config.ApplyFileConfig(cfg, p); err != nil {
				return nil, err
			}
			if p.Alias != nil {
				cfg.AliasSet = true
				cfg.AliasFromProfile = true
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
	fs.Usage = func() { PrintUsageForLocale(stderr) }

	fs.StringVar(&cfg.WorkDir, "dir", cfg.WorkDir, i18n.T(i18n.MsgFlagDir))
	fs.StringVar(&cfg.Host, "host", cfg.Host, i18n.T(i18n.MsgFlagHost))
	fs.StringVar(&cfg.Port, "port", cfg.Port, i18n.T(i18n.MsgFlagPort))
	fs.StringVar(&cfg.Model, "model", cfg.Model, i18n.T(i18n.MsgFlagModel))
	fs.StringVar(&cfg.ServerBin, "bin", cfg.ServerBin, i18n.T(i18n.MsgFlagBin))
	fs.StringVar(&cfg.CtxSize, "c", cfg.CtxSize, i18n.T(i18n.MsgFlagC))
	fs.StringVar(&cfg.BatchSize, "b", cfg.BatchSize, i18n.T(i18n.MsgFlagB))
	fs.StringVar(&cfg.UbatchSize, "ub", cfg.UbatchSize, i18n.T(i18n.MsgFlagUb))
	fs.StringVar(&cfg.FlashAttn, "fa", cfg.FlashAttn, i18n.T(i18n.MsgFlagFa))
	fs.StringVar(&cfg.Parallel, "parallel", cfg.Parallel, i18n.T(i18n.MsgFlagParallel))
	fs.StringVar(&cfg.Tools, "tools", cfg.Tools, i18n.T(i18n.MsgFlagTools))
	fs.BoolVar(&cfg.Jinja, "jinja", cfg.Jinja, i18n.T(i18n.MsgFlagJinja))
	fs.BoolVar(&cfg.McpProxy, "webui-mcp-proxy", cfg.McpProxy, i18n.T(i18n.MsgFlagMcpProxy))
	fs.BoolVar(&cfg.LazyMmap, "lm-lazy", cfg.LazyMmap, i18n.T(i18n.MsgFlagLazyMmap))
	fs.BoolVar(&cfg.Verbose, "v", cfg.Verbose, i18n.T(i18n.MsgFlagVerbose))

	fs.StringVar(&cfg.NGL, "ngl", cfg.NGL, i18n.T(i18n.MsgFlagNgl))
	fs.StringVar(&cfg.Mmproj, "mmproj", cfg.Mmproj, i18n.T(i18n.MsgFlagMmproj))
	fs.StringVar(&cfg.Temp, "temp", cfg.Temp, i18n.T(i18n.MsgFlagTemp))
	fs.StringVar(&cfg.TopP, "top-p", cfg.TopP, i18n.T(i18n.MsgFlagTopP))
	fs.StringVar(&cfg.CacheTypeK, "cache-type-k", cfg.CacheTypeK, i18n.T(i18n.MsgFlagCacheK))
	fs.StringVar(&cfg.CacheTypeV, "cache-type-v", cfg.CacheTypeV, i18n.T(i18n.MsgFlagCacheV))
	fs.StringVar(&cfg.FitMode, "fit", cfg.FitMode, i18n.T(i18n.MsgFlagFit))
	fs.StringVar(&cfg.LogVerbosity, "lv", cfg.LogVerbosity, i18n.T(i18n.MsgFlagLv))
	fs.StringVar(&cfg.Reasoning, "reasoning", cfg.Reasoning, i18n.T(i18n.MsgFlagReasoning))
	fs.StringVar(&cfg.ReasoningBudget, "reasoning-budget", cfg.ReasoningBudget, i18n.T(i18n.MsgFlagReasoningBudget))
	fs.StringVar(&cfg.SpecType, "spec-type", cfg.SpecType, i18n.T(i18n.MsgFlagSpecType))
	fs.StringVar(&cfg.SpecDraftModel, "spec-draft-model", cfg.SpecDraftModel, i18n.T(i18n.MsgFlagSpecDraftModel))
	fs.StringVar(&cfg.SpecDraftNMax, "spec-draft-n-max", cfg.SpecDraftNMax, i18n.T(i18n.MsgFlagSpecDraftNMax))
	fs.StringVar(&cfg.SpecDraftNGL, "spec-draft-ngl", cfg.SpecDraftNGL, i18n.T(i18n.MsgFlagSpecDraftNGL))

	fs.StringVar(&cfg.Download, "download", cfg.Download, i18n.T(i18n.MsgFlagDownload))

	fs.BoolVar(&cfg.UpdateFlag, "update", false, i18n.T(i18n.MsgFlagUpdate))
	fs.StringVar(&cfg.LlamaSrc, "llama-src", cfg.LlamaSrc, i18n.T(i18n.MsgFlagLlamaSrc))
	fs.StringVar(&cfg.InstallPrefix, "install-prefix", cfg.InstallPrefix, i18n.T(i18n.MsgFlagInstallPrefix))
	fs.BoolVar(&cfg.WithCuda, "cuda", cfg.WithCuda, i18n.T(i18n.MsgFlagCuda))
	fs.StringVar(&cfg.CudaArch, "cuda-arch", cfg.CudaArch, i18n.T(i18n.MsgFlagCudaArch))

	fs.BoolVar(&cfg.ServerOnly, "server", false, i18n.T(i18n.MsgFlagServer))
	fs.StringVar(&cfg.ClaudeBin, "claude-bin", cfg.ClaudeBin, i18n.T(i18n.MsgFlagClaudeBin))
	fs.StringVar(&cfg.ClaudeDir, "claude-dir", cfg.ClaudeDir, i18n.T(i18n.MsgFlagClaudeDir))
	fs.StringVar(&cfg.Alias, "alias", cfg.Alias, i18n.T(i18n.MsgFlagAlias))
	fs.DurationVar(&cfg.ReadyTimeout, "ready-timeout", cfg.ReadyTimeout, i18n.T(i18n.MsgFlagReadyTimeout))
	fs.StringVar(&cfg.ChatTemplate, "chat-template", cfg.ChatTemplate, i18n.T(i18n.MsgFlagChatTemplate))
	fs.StringVar(&cfg.LogFile, "log-file", cfg.LogFile, i18n.T(i18n.MsgFlagLogFile))
	fs.BoolVar(&cfg.QuietServer, "quiet-server", cfg.QuietServer, i18n.T(i18n.MsgFlagQuietServer))
	fs.IntVar(&cfg.LogMaxMB, "log-max-mb", cfg.LogMaxMB, i18n.T(i18n.MsgFlagLogMaxMB))
	fs.IntVar(&cfg.LogKeep, "log-keep", cfg.LogKeep, i18n.T(i18n.MsgFlagLogKeep))
	fs.StringVar(&cfg.McpAddr, "mcp-addr", cfg.McpAddr, i18n.T(i18n.MsgFlagMcpAddr))
	fs.StringVar(&cfg.McpConfigOut, "mcp-config", cfg.McpConfigOut, i18n.T(i18n.MsgFlagMcpConfig))
	fs.BoolVar(&cfg.McpStrict, "mcp-strict", cfg.McpStrict, i18n.T(i18n.MsgFlagMcpStrict))

	fs.BoolVar(&cfg.VersionFlag, "version", false, i18n.T(i18n.MsgFlagVersion))

	fs.StringVar(&cfg.OpenAIBaseURL, "openai", cfg.OpenAIBaseURL, i18n.T(i18n.MsgFlagOpenai))
	fs.StringVar(&cfg.OpenAIKey, "key", cfg.OpenAIKey, i18n.T(i18n.MsgFlagKey))
	fs.StringVar(&cfg.OpenAIModel, "model-openai", cfg.OpenAIModel, i18n.T(i18n.MsgFlagModelOpenai))
	fs.StringVar(&cfg.OpenAIPath, "openai-path", cfg.OpenAIPath, i18n.T(i18n.MsgFlagOpenaiPath))
	fs.StringVar(&cfg.ProxyAddr, "proxy-addr", cfg.ProxyAddr, i18n.T(i18n.MsgFlagProxyAddr))
	fs.StringVar(&cfg.ProxyToken, "proxy-token", cfg.ProxyToken, i18n.T(i18n.MsgFlagProxyToken))
	fs.BoolVar(&cfg.ProxyAllowRemote, "proxy-allow-remote", cfg.ProxyAllowRemote, i18n.T(i18n.MsgFlagProxyAllowRem))

	// M6: config, profile, doctor.
	fs.StringVar(&cfg.ConfigPath, "config", cfg.ConfigPath, i18n.T(i18n.MsgFlagConfig))
	fs.StringVar(&cfg.Profile, "p", cfg.Profile, i18n.T(i18n.MsgFlagP))
	fs.BoolVar(&cfg.DoctorFlag, "doctor", false, i18n.T(i18n.MsgFlagDoctor))
	// auto mode: the task and the headless plan-only dry run.
	fs.StringVar(&cfg.Task, "task", cfg.Task, i18n.T(i18n.MsgFlagTask))
	fs.BoolVar(&cfg.PlanOnly, "plan-only", false, i18n.T(i18n.MsgFlagPlanOnly))

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	var sawModel, sawDownload, sawCtx bool
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "c":
			// Only the fact that the flag was given matters: an explicit
			// -c contradicts the context contract of an auto planner.
			sawCtx = true
		case "chat-template":
			cfg.ChatTemplateSet = true
		case "alias":
			// Recorded twice on purpose: AliasSet keeps the value from being
			// treated as unset, AliasFromCLI puts it on top of the
			// ResolveAlias precedence chain (rule 1).
			cfg.AliasSet = true
			cfg.AliasFromCLI = true
		case "model":
			// A command-line -model is a deliberate model choice, so the
			// visible alias must follow it rather than keep the config file's
			// name (rule 2 of ResolveAlias).
			cfg.ModelSet = true
			sawModel = true
		case "download":
			// A CLI -download is a deliberate model choice, just like
			// -model: the visible alias must follow the downloaded file
			// (rule 2 of ResolveAlias), and an inherited per-model
			// parallel is reset below by the same ModelSet rule.
			cfg.ModelSet = true
			sawDownload = true
		case "parallel":
			// Record only the fact that the flag was given: an explicit
			// -parallel outranks the reset triggered by a CLI -model (see
			// the post-Visit block below), and keeps its own value even
			// when the model came from the config file or a profile.
			cfg.ParallelSet = true
		case "key":
			// Only the fact that the flag was given matters: it is what lets
			// -key "" mean "no authentication" instead of "no key provided".
			// The value itself is interpreted by ResolveOpenAIKey.
			cfg.OpenAIKeyFromCLI = true
		}
	})
	// -download together with -model: the fetch wins — run() replaces
	// cfg.Model with the downloaded entry point before anything reads it.
	// Record the collision so the -v path can warn about the ignored
	// -model (the values alone cannot reveal it: cfg.Model has a default).
	if sawDownload && sawModel {
		cfg.DownloadOverridesModel = true
	}
	// -model on the CLI picks a different model; a parallel carried over
	// from the file or a profile describes the OLD model and must not leak
	// into the new one. Reset it to the built-in default ("1") unless
	// -parallel was given explicitly, which outranks everything as usual.
	// This mirrors ResolveAlias rule 2, which resets a stale alias in the
	// same situation.
	if cfg.ModelSet && !cfg.ParallelSet {
		cfg.Parallel = "1"
	}
	// --- auto mode: the planner profile is self-contained ---
	//
	// These flags break a contract the orchestrator relies on, so they are
	// rejected (exit 2) rather than warned about:
	//   -model / -download  replace the planner while mode stays "auto"
	//                       (ResolveAlias would also discard its alias);
	//   -alias              renames the running model, so switch-to:<alias>
	//                       no longer names it and the catalog gains a
	//                       second entry for the same GGUF;
	//   -parallel           changes the slot count and the per-slot context;
	//   -c                  changes the context budget published to Claude.
	// Tuning flags (-temp, -ngl, -fit, -reasoning*, -b/-ub, …) stay allowed:
	// they affect quality, not identity/context/concurrency.
	//
	// The guard is AutoMode(), so nothing here touches ordinary launches:
	// -p coder -model X keeps working exactly as before.
	if cfg.AutoMode() {
		var banned []string
		for _, f := range []struct {
			name string
			set  bool
		}{
			{"model", sawModel},
			{"download", sawDownload},
			{"alias", cfg.AliasFromCLI},
			{"parallel", cfg.ParallelSet},
			{"c", sawCtx},
		} {
			if f.set {
				banned = append(banned, "-"+f.name)
			}
		}
		if len(banned) > 0 {
			return nil, i18n.Errorv(i18n.MsgErrAutoFlagConflict, strings.Join(banned, ", "))
		}
	}
	// --- Profile catalog (opt-in hot-swap) ---
	// When the config file declares profiles, build each one as a fully
	// merged Config, resolved independently of the active CLI profile.
	// This catalog drives the /model menu and the proxy's hot-swap; with
	// no profiles in the file, cfg.Profiles stays nil and every code path
	// below falls through to the pre-existing single-model behaviour.
	if fc != nil && len(fc.Profiles) > 0 {
		cfg.Profiles = make(map[string]*config.Config, len(fc.Profiles)+1)
		cfg.ProfileDescriptions = make(map[string]string, len(fc.Profiles))

		for name := range fc.Profiles {
			pfc := fc.Profiles[name]
			pc := config.DefaultConfig()
			if err := config.ApplyFileConfig(pc, *fc); err != nil {
				return nil, err
			}
			// Same non-inheritance rule as above; rootCfg below keeps the
			// root's own mode, so a root planner is a legitimate catalog
			// entry.
			pc.Mode = ""
			if err := config.ApplyFileConfig(pc, pfc); err != nil {
				return nil, err
			}
			if pfc.Alias != nil {
				pc.AliasSet = true
			}
			if pfc.ChatTemplate != nil {
				pc.ChatTemplateSet = true
			}
			pc.Profile = name
			pc.ConfigPath = configPath
			alias, _ := ResolveAlias(pc)
			pc.Alias = alias
			pc.AliasSet = true
			if _, dup := cfg.Profiles[alias]; dup {
				fmt.Fprint(stderr, i18n.TF(i18n.MsgWarnProfileDupAlias, name, alias))
				continue
			}
			cfg.Profiles[alias] = pc
			if pfc.Description != nil && *pfc.Description != "" {
				cfg.ProfileDescriptions[alias] = *pfc.Description
			}
		}

		// Root as an implicit profile: rebuild it from DefaultConfig + the file's
		// root section (NOT from cfg, which already carries the -p profile).
		rootCfg := config.DefaultConfig()
		if err := config.ApplyFileConfig(rootCfg, *fc); err != nil {
			return nil, err
		}
		rootCfg.ConfigPath = configPath
		if fc.Alias != nil {
			rootCfg.AliasSet = true // ← КРИТИЧНО: без этого ResolveAlias вернёт "local"
		}
		rootAlias, _ := ResolveAlias(rootCfg)
		if rootAlias != "" {
			rootCfg.Alias = rootAlias
			rootCfg.AliasSet = true
			if _, exists := cfg.Profiles[rootAlias]; !exists {
				cfg.Profiles[rootAlias] = rootCfg
			}
		}
	}
	return cfg, nil
}
