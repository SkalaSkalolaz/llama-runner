package cli

import (
	"strconv"
	"strings"

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

// openaiSlotPlaceholder is the value published in the Opus slot of the
// Claude Code /model menu when no external OpenAI-compatible provider is
// configured. The Opus slot is reserved for OpenAI-API models; this literal
// marks the slot as "OpenAI provider not configured".
const openaiSlotPlaceholder = "openai"

// BuildClaudeEnv returns a fresh environment slice (KEY=VALUE strings) for
// the Claude Code child process. baseEnv is never mutated: it is copied with
// the model-menu variables it may carry from an outer llama-runner session
// removed (see scrubModelMenuVars), then this launcher's own values are
// appended.
//
// The returned environment:
//   - points Claude Code at baseURL via ANTHROPIC_BASE_URL;
//   - authenticates with cfg.ProxyToken (or the historical default
//     "local-llama") in BOTH ANTHROPIC_AUTH_TOKEN and ANTHROPIC_API_KEY,
//     because different Claude Code versions send Authorization: Bearer or
//     x-api-key and the proxy accepts either;
//   - sets CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 to keep a local
//     deployment from phoning home;
//   - publishes the whole /model menu from BuildModelMenu(cfg), which is the
//     single source of truth for the layout and is mode-aware: the OPUS slot
//     is reserved for models served through the OpenAI-compatible API and
//     carries cfg.OpenAIModel in hybrid mode, the openaiSlotPlaceholder
//     otherwise; SONNET/HAIKU/FABLE take the aliases of the first three
//     config profiles (or the "unused" marker when a profile is missing),
//     the first custom entry takes the model actually loaded in local mode
//     (cfg.Alias,
//     which run() has already resolved — see cli.ResolveAlias) or the proxy's
//     "llama-server" local-route token in hybrid mode, and further custom
//     entries take the remaining profiles.
func BuildClaudeEnv(cfg *config.Config, baseURL string, baseEnv []string) []string {
	env := scrubModelMenuVars(baseEnv)

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
	// One loop for both modes: the /model layout lives in BuildModelMenu,
	// and this is the only place it is turned into environment variables.
	for _, slot := range BuildModelMenu(cfg) {
		env = append(env, modelSlotEnvVars(slot)...)
	}
	return env
}

// scrubModelMenuVars removes the variables that publish the /model menu —
// the standard aliases and every custom entry, numbered or not — from a base
// environment. The launcher always publishes the complete menu itself, so an
// inherited value (typically from an outer llama-runner session when the tool
// is nested) is stale: it would survive as a ghost entry the user never
// configured for this run. Entries without '=' are passed through untouched.
func scrubModelMenuVars(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "ANTHROPIC_DEFAULT_") ||
			strings.HasPrefix(k, "ANTHROPIC_CUSTOM_MODEL_OPTION") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// modelSlotEnvVars serialises one menu slot into the variables Claude Code
// reads it from.
//
// The four standard positions (Opus/Sonnet/Haiku/Fable) have one variable
// each. A custom position has a set: the model identifier plus, when
// present, a display name and a description. The first custom position keeps
// the documented, singular names
// (ANTHROPIC_CUSTOM_MODEL_OPTION and friends); further positions number the
// family itself: ANTHROPIC_CUSTOM_MODEL_OPTION_2,
// ANTHROPIC_CUSTOM_MODEL_OPTION_2_NAME, ANTHROPIC_CUSTOM_MODEL_OPTION_2_DESCRIPTION.
//
// The numbered form is the one part of this design not confirmed against
// Claude Code: only the singular set is documented, and whether numbered
// variants are read has to be checked against a live /model menu (see the note
// in README). It is used because it is the only shape that can express extra
// positions at all, and it degrades harmlessly: if Claude Code ignores the
// numbered variables the first three profiles still appear (they ride on
// Sonnet/Haiku/Fable) and the loaded model still appears in the singular
// custom entry — the user simply sees no entries from the fourth profile
// onwards.
func modelSlotEnvVars(slot ModelSlot) []string {
	if slot.ModelValue == "" {
		return nil
	}
	if slot.Kind != SlotCustom {
		name := standardSlotVar(slot)
		if name == "" {
			return nil
		}
		return []string{name + "=" + slot.ModelValue}
	}
	prefix := customSlotVarPrefix(slot.CustomIndex)
	vars := []string{prefix + "=" + slot.ModelValue}
	if slot.Name != "" {
		vars = append(vars, prefix+"_NAME="+slot.Name)
	}
	if slot.Description != "" {
		vars = append(vars, prefix+"_DESCRIPTION="+slot.Description)
	}
	return vars
}

// standardSlotVar names the variable behind a standard menu position. An
// unknown position yields "" and the caller publishes nothing for it.
func standardSlotVar(slot ModelSlot) string {
	switch slot.Kind {
	case SlotOpus:
		return "ANTHROPIC_DEFAULT_OPUS_MODEL"
	case SlotSonnet:
		return "ANTHROPIC_DEFAULT_SONNET_MODEL"
	case SlotHaiku:
		return "ANTHROPIC_DEFAULT_HAIKU_MODEL"
	case SlotFable:
		return "ANTHROPIC_DEFAULT_FABLE_MODEL"
	}
	return ""
}

// customSlotVarPrefix names the variable family behind custom position
// number index (1-based; index <= 1 is the singular, documented entry).
func customSlotVarPrefix(index int) string {
	if index <= 1 {
		return "ANTHROPIC_CUSTOM_MODEL_OPTION"
	}
	return "ANTHROPIC_CUSTOM_MODEL_OPTION_" + strconv.Itoa(index)
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
