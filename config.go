package main

import "time"

type Config struct {
	// Base
	WorkDir    string
	Host       string
	Port       string
	Model      string
	ServerBin  string
	CtxSize    string
	BatchSize  string
	UbatchSize string
	FlashAttn  string
	Parallel   string
	Tools      string
	Jinja      bool
	McpProxy   bool
	LazyMmap   bool
	Verbose    bool

	// Extra llama.cpp
	NGL          string
	Mmproj       string
	Temp         string
	TopP         string
	CacheTypeK   string
	CacheTypeV   string
	FitMode      string
	LogVerbosity string

	// Reasoning depth control for llama-server
	Reasoning       string
	ReasoningBudget string

	// Speculative
	SpecType       string
	SpecDraftModel string
	SpecDraftNMax  string
	SpecDraftNGL   string

	// Update
	UpdateFlag    bool
	LlamaSrc      string
	InstallPrefix string
	WithCuda      bool
	CudaArch      string

	// Claude Code
	ServerOnly   bool
	ClaudeBin    string
	ClaudeDir    string
	Alias        string
	ReadyTimeout time.Duration
	ChatTemplate string
	LogFile      string
	QuietServer  bool
	McpAddr      string
	McpConfigOut string
	McpStrict    bool

	// External OpenAI-compatible provider
	OpenAIBaseURL string
	OpenAIKey     string
	OpenAIModel   string

	VersionFlag      bool
	ProxyAddr        string
	ProxyToken       string
	ProxyAllowRemote bool

	ConfigPath           string
	Profile              string
	DoctorFlag           bool
	ChatTemplateFallback string

	LogMaxMB int
	LogKeep  int
	// Derived (filled after Parse/Validate)
	Home            string
	ChatTemplateSet bool // -chat-template set explicitly
	AliasSet        bool // -alias set explicitly
}
