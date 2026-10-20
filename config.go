package main

import "time"

type Config struct {
    // Базовые
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

    // Доп. llama.cpp
    NGL          string
    Mmproj       string
    Temp         string
    TopP         string
    CacheTypeK   string
    CacheTypeV   string
    FitMode      string
    LogVerbosity string

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

   // Внешний OpenAI-совместимый провайдер
    OpenAIBaseURL string
    OpenAIKey     string   
    OpenAIModel   string       

    VersionFlag bool
    ProxyAddr string

    // Производные (заполняются после Parse/Validate)
    Home            string
    ChatTemplateSet bool // -chat-template задан явно
    AliasSet        bool // -alias задан явно
}