package main

import "time"

// Config is the fully resolved runtime configuration for a single
// llama-runner invocation. It is assembled in layers — built-in defaults
// (see defaultConfig), then the JSON config file, then a named profile from
// that file, and finally command-line flags — so that precedence is
// CLI > profile > file > defaults (see parseFlagsWithHome).
//
// Path-valued fields may still contain a leading "~"; expansion happens
// later, at the point of use (expandHome plus filepath.Abs). String-typed
// numeric fields (CtxSize, Port, NGL, …) are kept verbatim and passed through
// to llama-server untouched; only fields llama-runner itself interprets are
// parsed. The boolean *Set fields record whether a value was provided
// explicitly (by flag or by file), which changes validation and defaulting
// behaviour in run.
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

	// Derived — not settable by the user; filled in after parse/validate.
	// Home is the resolved user home directory used to expand "~" in paths.
	Home string
	// ChatTemplateSet reports whether -chat-template (or the file field
	// chatTemplate) was provided explicitly, as opposed to coming from the
	// "auto" default. Explicit templates that are missing are an error;
	// auto-detected ones degrade to the template embedded in the GGUF.
	ChatTemplateSet bool
	// AliasSet reports whether -alias (or the file field alias) was provided
	// explicitly. When it was not, run substitutes the default alias "local"
	// for Claude Code mode.
	AliasSet bool
}
