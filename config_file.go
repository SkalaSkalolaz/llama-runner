package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// defaultConfigPath returns the location of the config file used when the
// user does not pass -config: ~/.config/llama-runner/config.json (built from
// the injected home directory so the function stays testable).
func defaultConfigPath(home string) string {
	return filepath.Join(home, ".config", "llama-runner", "config.json")
}

// defaultConfig returns a Config populated with the built-in defaults — the
// bottom layer of the precedence chain (defaults < file < profile < CLI).
// The values deliberately match the historical pre-M6 hardcoded behaviour, so
// running without any config file behaves exactly as it did before the
// config-file feature existed. Paths keep their "~" prefix; expansion happens
// at use time.
func defaultConfig() *Config {
	return &Config{
		Host:                 "127.0.0.1",
		Port:                 "8080",
		Model:                "~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf",
		ServerBin:            "~/.local/bin/llama-server",
		CtxSize:              "65536",
		BatchSize:            "2048",
		UbatchSize:           "1024",
		FlashAttn:            "auto",
		Parallel:             "1",
		Tools:                "all",
		Jinja:                true,
		LlamaSrc:             "~/src/llama.cpp",
		InstallPrefix:        "~/.local",
		WithCuda:             true,
		CudaArch:             "61",
		ClaudeBin:            "claude",
		ReadyTimeout:         180 * time.Second,
		ChatTemplate:         "auto",
		ChatTemplateFallback: "~/GGUF/qwen-fixed-chat-template.jinja",
		LogFile:              "~/.cache/llama-runner/llama-server.log",
		LogMaxMB:             50,
		LogKeep:              5,
		QuietServer:          true,
		McpAddr:              "127.0.0.1:8090",
		ProxyAddr:            "127.0.0.1:8082",
	}
}

// fileConfig is the JSON schema of the config file. It is a separate type
// from Config on purpose:
//   - derived fields (Home, ChatTemplateSet, AliasSet) are not serializable
//     state and must never appear in the file;
//   - CLI-only one-shot flags (VersionFlag, UpdateFlag, DoctorFlag,
//     ConfigPath, Profile) make no sense as persisted settings;
//   - ReadyTimeout is stored as a human-friendly duration string ("180s")
//     rather than int64 nanoseconds.
//
// Every field is a pointer: nil means "absent from the file", so
// applyFileConfig overlays only the fields the user actually set. This is
// what implements the precedence chain — the file overrides built-in
// defaults, and CLI flags later override the file.
//
// Secrets (openaiKey, proxyToken) can technically live in the file, but this
// is discouraged: prefer LLAMA_RUNNER_OPENAI_KEY / OPENAI_API_KEY or the
// ~/.config/llama-runner/credentials file for openaiKey, and the -proxy-token
// flag for the proxy token.
type fileConfig struct {
	// Base
	WorkDir    *string `json:"dir,omitempty"`
	Host       *string `json:"host,omitempty"`
	Port       *string `json:"port,omitempty"`
	Model      *string `json:"model,omitempty"`
	ServerBin  *string `json:"bin,omitempty"`
	CtxSize    *string `json:"c,omitempty"`
	BatchSize  *string `json:"b,omitempty"`
	UbatchSize *string `json:"ub,omitempty"`
	FlashAttn  *string `json:"fa,omitempty"`
	Parallel   *string `json:"parallel,omitempty"`
	Tools      *string `json:"tools,omitempty"`
	Jinja      *bool   `json:"jinja,omitempty"`
	McpProxy   *bool   `json:"webuiMcpProxy,omitempty"`
	LazyMmap   *bool   `json:"lmLazy,omitempty"`
	Verbose    *bool   `json:"verbose,omitempty"`

	// Extra llama.cpp
	NGL          *string `json:"ngl,omitempty"`
	Mmproj       *string `json:"mmproj,omitempty"`
	Temp         *string `json:"temp,omitempty"`
	TopP         *string `json:"topP,omitempty"`
	CacheTypeK   *string `json:"cacheTypeK,omitempty"`
	CacheTypeV   *string `json:"cacheTypeV,omitempty"`
	FitMode      *string `json:"fit,omitempty"`
	LogVerbosity *string `json:"logVerbosity,omitempty"`

	Reasoning       *string `json:"reasoning,omitempty"`
	ReasoningBudget *string `json:"reasoningBudget,omitempty"`

	// Speculative
	SpecType       *string `json:"specType,omitempty"`
	SpecDraftModel *string `json:"specDraftModel,omitempty"`
	SpecDraftNMax  *string `json:"specDraftNMax,omitempty"`
	SpecDraftNGL   *string `json:"specDraftNGL,omitempty"`

	// Update
	LlamaSrc      *string `json:"llamaSrc,omitempty"`
	InstallPrefix *string `json:"installPrefix,omitempty"`
	WithCuda      *bool   `json:"cuda,omitempty"`
	CudaArch      *string `json:"cudaArch,omitempty"`

	// Claude Code
	ServerOnly           *bool   `json:"serverOnly,omitempty"`
	ClaudeBin            *string `json:"claudeBin,omitempty"`
	ClaudeDir            *string `json:"claudeDir,omitempty"`
	Alias                *string `json:"alias,omitempty"`
	ReadyTimeout         *string `json:"readyTimeout,omitempty"` // duration string, e.g. "180s"
	ChatTemplate         *string `json:"chatTemplate,omitempty"`
	ChatTemplateFallback *string `json:"chatTemplateFallback,omitempty"`
	LogFile              *string `json:"logFile,omitempty"`
	LogMaxMB             *int    `json:"logMaxMB,omitempty"`
	LogKeep              *int    `json:"logKeep,omitempty"`
	QuietServer          *bool   `json:"quietServer,omitempty"`
	McpAddr              *string `json:"mcpAddr,omitempty"`
	McpConfigOut         *string `json:"mcpConfig,omitempty"`
	McpStrict            *bool   `json:"mcpStrict,omitempty"`

	// OpenAI
	OpenAIBaseURL *string `json:"openai,omitempty"`
	OpenAIKey     *string `json:"openaiKey,omitempty"` // not recommended; prefer env / credentials
	OpenAIModel   *string `json:"modelOpenai,omitempty"`

	// Proxy
	ProxyAddr        *string `json:"proxyAddr,omitempty"`
	ProxyToken       *string `json:"proxyToken,omitempty"` // not recommended; prefer flag or env
	ProxyAllowRemote *bool   `json:"proxyAllowRemote,omitempty"`

	// Profiles
	Profiles map[string]fileConfig `json:"profiles,omitempty"`
}

// applyFileConfig overlays the set (non-nil) fields of fc onto cfg; nil
// fields leave cfg untouched. It is the single mechanism by which both the
// config file's root section and an individual profile are applied, so the
// same overlay rules hold at both levels.
//
// The only field requiring interpretation is readyTimeout: it is parsed from
// a Go duration string and a malformed value is returned as an error rather
// than silently keeping the previous setting.
func applyFileConfig(cfg *Config, fc fileConfig) error {
	set := func(dst *string, src *string) {
		if src != nil {
			*dst = *src
		}
	}
	setB := func(dst *bool, src *bool) {
		if src != nil {
			*dst = *src
		}
	}

	set(&cfg.WorkDir, fc.WorkDir)
	set(&cfg.Host, fc.Host)
	set(&cfg.Port, fc.Port)
	set(&cfg.Model, fc.Model)
	set(&cfg.ServerBin, fc.ServerBin)
	set(&cfg.CtxSize, fc.CtxSize)
	set(&cfg.BatchSize, fc.BatchSize)
	set(&cfg.UbatchSize, fc.UbatchSize)
	set(&cfg.FlashAttn, fc.FlashAttn)
	set(&cfg.Parallel, fc.Parallel)
	set(&cfg.Tools, fc.Tools)
	setB(&cfg.Jinja, fc.Jinja)
	setB(&cfg.McpProxy, fc.McpProxy)
	setB(&cfg.LazyMmap, fc.LazyMmap)
	setB(&cfg.Verbose, fc.Verbose)

	set(&cfg.NGL, fc.NGL)
	set(&cfg.Mmproj, fc.Mmproj)
	set(&cfg.Temp, fc.Temp)
	set(&cfg.TopP, fc.TopP)
	set(&cfg.CacheTypeK, fc.CacheTypeK)
	set(&cfg.CacheTypeV, fc.CacheTypeV)
	set(&cfg.FitMode, fc.FitMode)
	set(&cfg.LogVerbosity, fc.LogVerbosity)

	set(&cfg.Reasoning, fc.Reasoning)
	set(&cfg.ReasoningBudget, fc.ReasoningBudget)

	set(&cfg.SpecType, fc.SpecType)
	set(&cfg.SpecDraftModel, fc.SpecDraftModel)
	set(&cfg.SpecDraftNMax, fc.SpecDraftNMax)
	set(&cfg.SpecDraftNGL, fc.SpecDraftNGL)

	set(&cfg.LlamaSrc, fc.LlamaSrc)
	set(&cfg.InstallPrefix, fc.InstallPrefix)
	setB(&cfg.WithCuda, fc.WithCuda)
	set(&cfg.CudaArch, fc.CudaArch)

	setB(&cfg.ServerOnly, fc.ServerOnly)
	set(&cfg.ClaudeBin, fc.ClaudeBin)
	set(&cfg.ClaudeDir, fc.ClaudeDir)
	set(&cfg.Alias, fc.Alias)
	set(&cfg.ChatTemplate, fc.ChatTemplate)
	set(&cfg.ChatTemplateFallback, fc.ChatTemplateFallback)
	set(&cfg.LogFile, fc.LogFile)
	if fc.LogMaxMB != nil {
		cfg.LogMaxMB = *fc.LogMaxMB
	}
	if fc.LogKeep != nil {
		cfg.LogKeep = *fc.LogKeep
	}
	setB(&cfg.QuietServer, fc.QuietServer)
	set(&cfg.McpAddr, fc.McpAddr)
	set(&cfg.McpConfigOut, fc.McpConfigOut)
	setB(&cfg.McpStrict, fc.McpStrict)

	set(&cfg.OpenAIBaseURL, fc.OpenAIBaseURL)
	set(&cfg.OpenAIKey, fc.OpenAIKey)
	set(&cfg.OpenAIModel, fc.OpenAIModel)

	set(&cfg.ProxyAddr, fc.ProxyAddr)
	set(&cfg.ProxyToken, fc.ProxyToken)
	setB(&cfg.ProxyAllowRemote, fc.ProxyAllowRemote)

	if fc.ReadyTimeout != nil {
		d, err := time.ParseDuration(*fc.ReadyTimeout)
		if err != nil {
			return fmt.Errorf("некорректный readyTimeout %q: %w", *fc.ReadyTimeout, err)
		}
		cfg.ReadyTimeout = d
	}
	return nil
}

// loadConfigFile reads and parses the JSON config file at path (which may
// start with "~").
//
// A missing file is not an error: it returns (nil, nil), matching the
// "no config file is normal" rule for the default path. path == "" likewise
// yields (nil, nil) — reading is disabled (explicit -config ""). Any other
// read failure or invalid JSON is returned as an error, and whether a missing
// file should be promoted to an error (explicit -config) is decided by the
// caller.
func loadConfigFile(path, home string) (*fileConfig, error) {
	if path == "" {
		return nil, nil
	}
	absPath := expandHome(path, home)
	data, err := os.ReadFile(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("не удалось прочитать конфиг %q: %w", absPath, err)
	}
	var fc fileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("некорректный JSON в конфиге %q: %w", absPath, err)
	}
	return &fc, nil
}

// findFlag pre-scans raw argument list args for a flag given as
// -name value, --name value or -name=value (single and double dashes are
// equivalent). It returns the flag's value and true, or ("", false) when the
// flag is absent.
//
// This scan exists because -config and -p must be discovered before the
// FlagSet is built — they determine where flag defaults come from. Scanning
// stops at the first argument that is neither a flag nor a pending flag
// value, and at the "--" terminator, so positional arguments are never
// mistaken for flag values.
func findFlag(args []string, name string) (string, bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return "", false
		}
		if !strings.HasPrefix(a, "-") {
			return "", false
		}
		trimmed := strings.TrimLeft(a, "-")
		if eq := strings.IndexByte(trimmed, '='); eq >= 0 {
			if trimmed[:eq] == name {
				return trimmed[eq+1:], true
			}
			continue
		}
		if trimmed == name {
			if i+1 < len(args) {
				return args[i+1], true
			}
			return "", true
		}
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
		}
	}
	return "", false
}
