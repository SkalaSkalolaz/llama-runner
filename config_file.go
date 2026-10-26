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

// defaultConfigPath returns the default config file path.
func defaultConfigPath(home string) string {
	return filepath.Join(home, ".config", "llama-runner", "config.json")
}

// defaultConfig returns Config with built-in defaults. Values match
// historical (pre-M6) so behavior without a config file doesn't change.
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

// fileConfig JSON representation of the config file. Separate type to:
//   - not serialize derived fields (Home, ChatTemplateSet, AliasSet);
//   - not include CLI-only fields (VersionFlag, UpdateFlag, DoctorFlag,
//     ConfigPath, Profile);
//   - store ReadyTimeout as duration-string, not int64 nanoseconds.
//
// All fields are pointers: nil means "not set", so applyFileConfig
// applies only set fields. This gives the correct priority
// CLI > file > default: file overrides default, CLI overrides file.
//
// Secrets (openaiKey, proxyToken) can technically be stored in the file, but
// env is recommended (LLAMA_RUNNER_OPENAI_KEY / OPENAI_API_KEY) or
// ~/.config/llama-runner/credentials for openaiKey. proxyToken — only a
// flag for now (added -proxy-token in M4); config is just an alternative.
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
	ReadyTimeout         *string `json:"readyTimeout,omitempty"` // duration-string, напр. "180s"
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

// applyFileConfig applies set (non-nil) fields from fc to cfg.
// Fields from fc override current cfg values; nil fields are untouched.
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

// loadConfigFile reads the config. Returns (nil, nil) if path=="" or the file
// doesn't exist — this is not an error. Invalid JSON — error.
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
