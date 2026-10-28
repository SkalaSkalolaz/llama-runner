package cli

import (
	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

// ServerPaths bundles the absolute, existence-verified file paths that
// llama-server is launched with. Empty Mmproj, Draft and Template mean
// "not configured / not used"; Model is always set on a successful launch
// path (see ResolvePaths).
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

// BuildServerArgs assembles the llama-server command line from cfg and the
// verified paths in ServerPaths.
//
// Positional/core arguments (model, host, port, tools, context, batch sizes,
// flash attention, parallel slots) are always present. Everything else is
// emitted only when configured: boolean switches (--jinja,
// --webui-mcp-proxy, lazy mmap) when enabled, and optional string parameters
// (-ngl, --temp, --top-p, cache types, --fit, -lv, reasoning controls,
// speculative-decoding knobs) when non-empty. Values are passed through
// verbatim — validation happens earlier in ValidateConfig.
func BuildServerArgs(cfg *config.Config, paths ServerPaths) []string {
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

// BuildClaudeEnv returns a fresh environment slice (KEY=VALUE strings) for
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
func BuildClaudeEnv(cfg *config.Config, baseURL string, baseEnv []string) []string {
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

// BuildClaudeArgs returns the command-line arguments for the claude child
// process. When mcpConfigPath is set, Claude Code is pointed at that MCP
// configuration file, and --strict-mcp-config is appended when
// cfg.McpStrict is set (restricting Claude Code to exactly the servers in
// that file). With no MCP config the result is an empty slice.
func BuildClaudeArgs(cfg *config.Config, mcpConfigPath string) []string {
	args := []string{}
	if mcpConfigPath != "" {
		args = append(args, "--mcp-config", mcpConfigPath)
		if cfg.McpStrict {
			args = append(args, "--strict-mcp-config")
		}
	}
	return args
}
