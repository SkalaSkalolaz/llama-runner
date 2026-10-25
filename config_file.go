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

// defaultConfigPath — путь к конфиг-файлу по умолчанию.
func defaultConfigPath(home string) string {
	return filepath.Join(home, ".config", "llama-runner", "config.json")
}

// defaultConfig возвращает Config со встроенными дефолтами. Значения совпадают
// с историческими (до M6), чтобы поведение без конфиг-файла не менялось.
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

// fileConfig — JSON-представление конфиг-файла. Отдельный тип, чтобы:
//   - не сериализовать производные поля (Home, ChatTemplateSet, AliasSet);
//   - не включать CLI-only поля (VersionFlag, UpdateFlag, DoctorFlag,
//     ConfigPath, Profile);
//   - ReadyTimeout хранить как duration-string, а не int64 наносекунд.
//
// Все поля — указатели: nil означает «не задано», поэтому applyFileConfig
// применяет только заданные. Это даёт правильный приоритет
// CLI > файл > дефолт: файл переопределяет дефолт, CLI переопределяет файл.
//
// Секреты (openaiKey, proxyToken) технически можно хранить в файле, но
// рекомендуется env (LLAMA_RUNNER_OPENAI_KEY / OPENAI_API_KEY) или
// ~/.config/llama-runner/credentials для openaiKey. proxyToken — пока только
// флаг (в M4 добавлен -proxy-token); конфиг это просто альтернатива.
type fileConfig struct {
	// Базовые
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

	// Доп. llama.cpp
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
	OpenAIKey     *string `json:"openaiKey,omitempty"` // не рекомендуется; лучше env / credentials
	OpenAIModel   *string `json:"modelOpenai,omitempty"`

	// Proxy
	ProxyAddr        *string `json:"proxyAddr,omitempty"`
	ProxyToken       *string `json:"proxyToken,omitempty"` // не рекомендуется; лучше флаг или env
	ProxyAllowRemote *bool   `json:"proxyAllowRemote,omitempty"`

	// Профили
	Profiles map[string]fileConfig `json:"profiles,omitempty"`
}

// applyFileConfig применяет заданные (не-nil) поля из fc к cfg.
// Поля из fc переопределяют текущие значения cfg; nil-поля не трогаются.
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

// loadConfigFile читает конфиг. Возвращает (nil, nil), если path=="" или файла
// нет — это не ошибка. Невалидный JSON — ошибка.
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
			// Позиционный аргумент — стандартный flag.Parse остановится
			// здесь, значит target-флаг после него фактически не будет
			// разобран. Возвращаем «не найдено».
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
		// Неизвестный флаг. Если следующий токен не похож на флаг —
		// считаем его значением этого флага и перескакиваем. Это
		// грубо (bool-флаги вроде -v значения не берут), но -config
		// и -p — всегда non-bool, а вызывающие ищут именно их.
		// Случай `-v /path -config /x` разберётся неверно, но тогда
		// и flag.Parse остановится на /path и не увидит -config вовсе.
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
		}
	}
	return "", false
}
