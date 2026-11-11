package config

import "time"

// Config is the fully resolved runtime configuration for a single
// llama-runner invocation. It is assembled in layers — built-in defaults
// (see DefaultConfig), then the JSON config file, then a named profile from
// that file, and finally command-line flags — so that precedence is
// CLI > profile > file > defaults (see ParseFlagsWithHome).
//
// Path-valued fields may still contain a leading "~"; expansion happens
// later, at the point of use (ExpandHome plus filepath.Abs). String-typed
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

	// Auto-download
	// Download holds the composite -download value "<local-dir><url>"; it
	// is consumed by run() before launch (the downloaded file becomes
	// Model) and reset to "" afterwards, so a re-parse cannot trigger a
	// second download.
	Download string

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
	// OpenAIPath is the path suffix appended to OpenAIBaseURL when the proxy
	// forwards a request to the external provider. Empty means "no suffix"
	// (used by providers such as OVH Kepler and VLM.run, which serve the
	// endpoint at the base URL itself). The default /chat/completions matches
	// the OpenAI chat/completions path and keeps standard providers
	// (OpenAI, OpenRouter, DeepSeek) working unchanged.
	OpenAIPath string
	// OpenAIKeyless records that the provider was asked to run WITHOUT
	// authentication (-key none, -key "", or "openaiKey": "none" in the
	// config file), as decided by cli.ResolveOpenAIKey. OpenAIKey stays empty
	// in that case: the distinction between "no key on purpose" and "no key
	// found" is what keeps the proxy from inventing an Authorization header,
	// and keeping it a flag rather than a magic string means no code path can
	// accidentally send the literal "none" as a bearer token.
	OpenAIKeyless bool

	VersionFlag      bool
	ProxyAddr        string
	ProxyToken       string
	ProxyAllowRemote bool

	ConfigPath string
	Profile    string
	// Mode is the profile's declared operating mode ("mode" in the config
	// file): "" — the ordinary single-model launch, ModeAuto — the
	// autonomous orchestrator. It is not inherited by profiles from the
	// root section; see AutoMode and cli.ParseFlagsWithHome.
	Mode                 string
	DoctorFlag           bool
	ChatTemplateFallback string

	LogMaxMB int
	LogKeep  int

	// Derived — not settable by the user; filled in after parse/validate.
	// ChatTemplateSet reports whether -chat-template (or the file field
	// chatTemplate) was provided explicitly, as opposed to coming from the
	// "auto" default. Explicit templates that are missing are an error;
	// auto-detected ones degrade to the template embedded in the GGUF.
	ChatTemplateSet bool
	// AliasSet reports whether -alias (or the file/profile field alias) was
	// provided explicitly. It is what keeps a file-provided alias from being
	// replaced by the derived/default naming (see cli.ResolveAlias rule 3).
	AliasSet bool
	// AliasFromCLI reports whether -alias was given on the command line. It
	// outranks every other naming source, including a CLI -model override
	// (see cli.ResolveAlias rule 1).
	AliasFromCLI bool
	// AliasFromProfile reports whether the selected -p profile supplied the
	// alias itself. Only the origin label changes: without it a root-file
	// alias would be reported as "profile" in -doctor when a profile that
	// sets no alias is active.
	AliasFromProfile bool
	// OpenAIKeyFromCLI reports whether the -key flag itself was given, which
	// is the only way to tell "-key with an empty value" (no authentication,
	// see OpenAIKeyless) from "no -key at all" (search the environment and the
	// credentials file). Set by flag.Visit in ParseFlagsWithHome, like the
	// other origin flags above.
	OpenAIKeyFromCLI bool
	// ModelSet reports whether -model was given on the command line. A
	// command-line model choice invalidates a stale alias coming from the
	// config file, so the visible model name is derived from the chosen
	// GGUF instead (see cli.ResolveAlias rule 2). A profile's model does
	// NOT set this flag: a profile is self-contained and brings its own alias.
	ModelSet bool
	// DownloadOverridesModel reports that BOTH -download and -model were
	// given on the command line: the fetched model wins and the explicit
	// -model is ignored (run() warns about it in -v mode). Like the other
	// origin flags it cannot be derived from the values themselves, because
	// cfg.Model always has at least the built-in default.
	DownloadOverridesModel bool
	// ParallelSet reports whether -parallel was given on the command line.
	// Like AliasFromCLI, it outranks every value inherited from the config
	// file or a profile. When -model is given too, an inherited parallel
	// describes a DIFFERENT model and is reset to "1" unless -parallel was
	// explicit (see ParseFlagsWithHome). Set by flag.Visit, like the other
	// origin flags above.
	ParallelSet bool
	// Profiles holds the fully merged, launch-ready config of every named
	// profile declared in the config file (root config overlaid with the
	// profile's FileConfig, with the profile's alias resolved independently).
	// Empty when the config file has no "profiles" section: that is the
	// signal that the launcher serves a single model, exactly as before.
	// When non-empty, the /model menu advertises every profile and the
	// proxy hot-swaps llama-server on a change of the requested model.
	Profiles map[string]*Config
	// ProfileDescriptions maps a profile's resolved alias to the optional
	// human description from the config file ("description": "..."). Only
	// used to annotate custom /model entries. May be nil.
	ProfileDescriptions map[string]string
}

// OpenAIProviderConfigured reports whether an external OpenAI-compatible
// provider is fully configured and should be used: a base URL, a model name
// there, and either an API key or the explicit decision to go without one.
//
// This is the single answer to "is hybrid mode on?". It used to be spelled
// three ways in three places — validation triggered on ANY of the three
// settings, the proxy started on ALL of them including the key, and the Claude
// Code menu branch keyed on URL+model alone. Those agreed only because a key
// was mandatory; with keyless providers they would disagree, so run() and the
// menu now ask this method. Validation keeps its own, deliberately broader
// test: it must still report a half-configured provider as an error rather
// than quietly falling back to local mode.
func (c *Config) OpenAIProviderConfigured() bool {
	return c.OpenAIBaseURL != "" && c.OpenAIModel != "" &&
		(c.OpenAIKey != "" || c.OpenAIKeyless)
}
