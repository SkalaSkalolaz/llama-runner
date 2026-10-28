package i18n

// Message keys are declared as constants and used as catalog keys at the
// declaration site of messages below, so a missing translation is impossible
// by construction: each constant appears in exactly one catalog entry.

const (
	// --- General / process helpers (main.go) ---
	MsgStatus             = "status"
	MsgFinishedBeforeRdy  = "finished_before_ready"
	MsgTimeout            = "timeout"
	MsgNotReadyInTime     = "not_ready_in_time"
	MsgLabelServer        = "label_server"
	MsgMcpExitErr         = "mcp_exit_error"
	MsgMcpKill            = "mcp_kill"
	MsgMcpWaitErr         = "mcp_wait_error"
	MsgMcpKillUnconfirmed = "mcp_kill_unconfirmed"

	// --- Config resolution and validation (parse.go, config_file.go) ---
	MsgErrHomeDir              = "err_home_dir"
	MsgErrConfigFileNotFound   = "err_config_file_not_found"
	MsgErrProfileNoConfig      = "err_profile_no_config"
	MsgErrProfileNotFound      = "err_profile_not_found"
	MsgErrReadyTimeoutPositive = "err_ready_timeout_positive"
	MsgErrHostEmpty            = "err_host_empty"
	MsgErrPortBad              = "err_port_bad"
	MsgErrMcpAddrEmpty         = "err_mcp_addr_empty"
	MsgErrMcpAddrHostPort      = "err_mcp_addr_hostport"
	MsgErrReasoning            = "err_reasoning"
	MsgErrReasoningBudget      = "err_reasoning_budget"
	MsgErrOpenaiRequired       = "err_openai_required"
	MsgErrKeyRequired          = "err_key_required"
	MsgErrModelOpenaiRequired  = "err_model_openai_required"
	MsgErrOpenaiURLBad         = "err_openai_url_bad"
	MsgErrProxyAddrBad         = "err_proxy_addr_bad"
	MsgErrTemplateAbs          = "err_template_abs"
	MsgErrTemplateMissing      = "err_template_missing"
	MsgWarnCredCheck           = "warn_cred_check"
	MsgWarnCredPerms           = "warn_cred_perms"
	MsgWarnCredRead            = "warn_cred_read"
	MsgErrReadyTimeoutBad      = "err_ready_timeout_bad"
	MsgErrConfigRead           = "err_config_read"
	MsgErrConfigJSON           = "err_config_json"

	// --- Flag help descriptions (parse.go) ---
	MsgFlagDir             = "flag_dir"
	MsgFlagHost            = "flag_host"
	MsgFlagPort            = "flag_port"
	MsgFlagModel           = "flag_model"
	MsgFlagBin             = "flag_bin"
	MsgFlagC               = "flag_c"
	MsgFlagB               = "flag_b"
	MsgFlagUb              = "flag_ub"
	MsgFlagFa              = "flag_fa"
	MsgFlagParallel        = "flag_parallel"
	MsgFlagTools           = "flag_tools"
	MsgFlagJinja           = "flag_jinja"
	MsgFlagMcpProxy        = "flag_webui_mcp_proxy"
	MsgFlagLazyMmap        = "flag_lm_lazy"
	MsgFlagVerbose         = "flag_v"
	MsgFlagNgl             = "flag_ngl"
	MsgFlagMmproj          = "flag_mmproj"
	MsgFlagTemp            = "flag_temp"
	MsgFlagTopP            = "flag_top_p"
	MsgFlagCacheK          = "flag_cache_type_k"
	MsgFlagCacheV          = "flag_cache_type_v"
	MsgFlagFit             = "flag_fit"
	MsgFlagLv              = "flag_lv"
	MsgFlagReasoning       = "flag_reasoning"
	MsgFlagReasoningBudget = "flag_reasoning_budget"
	MsgFlagSpecType        = "flag_spec_type"
	MsgFlagSpecDraftModel  = "flag_spec_draft_model"
	MsgFlagSpecDraftNMax   = "flag_spec_draft_n_max"
	MsgFlagSpecDraftNGL    = "flag_spec_draft_ngl"
	MsgFlagUpdate          = "flag_update"
	MsgFlagLlamaSrc        = "flag_llama_src"
	MsgFlagInstallPrefix   = "flag_install_prefix"
	MsgFlagCuda            = "flag_cuda"
	MsgFlagCudaArch        = "flag_cuda_arch"
	MsgFlagServer          = "flag_server"
	MsgFlagClaudeBin       = "flag_claude_bin"
	MsgFlagClaudeDir       = "flag_claude_dir"
	MsgFlagAlias           = "flag_alias"
	MsgFlagReadyTimeout    = "flag_ready_timeout"
	MsgFlagChatTemplate    = "flag_chat_template"
	MsgFlagLogFile         = "flag_log_file"
	MsgFlagQuietServer     = "flag_quiet_server"
	MsgFlagLogMaxMB        = "flag_log_max_mb"
	MsgFlagLogKeep         = "flag_log_keep"
	MsgFlagMcpAddr         = "flag_mcp_addr"
	MsgFlagMcpConfig       = "flag_mcp_config"
	MsgFlagMcpStrict       = "flag_mcp_strict"
	MsgFlagVersion         = "flag_version"
	MsgFlagOpenai          = "flag_openai"
	MsgFlagKey             = "flag_key"
	MsgFlagModelOpenai     = "flag_model_openai"
	MsgFlagProxyAddr       = "flag_proxy_addr"
	MsgFlagProxyToken      = "flag_proxy_token"
	MsgFlagProxyAllowRem   = "flag_proxy_allow_remote"
	MsgFlagConfig          = "flag_config"
	MsgFlagP               = "flag_p"
	MsgFlagDoctor          = "flag_doctor"

	// --- Launch flow (run.go) ---
	MsgWarnOpenaiIgnored    = "warn_openai_ignored"
	MsgErrBinAbs            = "err_bin_abs"
	MsgErrModelAbs          = "err_model_abs"
	MsgErrMmprojAbs         = "err_mmproj_abs"
	MsgErrDraftAbs          = "err_draft_abs"
	MsgBinNotFound          = "bin_not_found"
	MsgModelNotFound        = "model_not_found"
	MsgMmprojNotFound       = "mmproj_not_found"
	MsgDraftNotFound        = "draft_not_found"
	MsgTemplateAutoDetected = "template_auto_detected"
	MsgErrHomeDirShort      = "err_home_dir_short"
	MsgErrResolved          = "err_resolved"
	MsgWarnTemplateAuto     = "warn_template_auto"
	MsgLdLibPathAlready     = "ld_lib_path_already"
	MsgLdLibPath            = "ld_lib_path"
	MsgErrWorkDirAbs        = "err_work_dir_abs"
	MsgErrWorkDirUnavail    = "err_work_dir_unavail"
	MsgErrLogDirCreate      = "err_log_dir_create"
	MsgErrLogOpenRotating   = "err_log_open_rotating"
	MsgErrLogOpen           = "err_log_open"
	MsgErrLogChmod          = "err_log_chmod"
	MsgWorkDirLine          = "work_dir_line"
	MsgCommandLine          = "command_line"
	MsgServerLogFile        = "server_log_file"
	MsgStartingServer       = "starting_server"
	MsgMcpCommandLine       = "mcp_command_line"
	MsgErrMcpStart          = "err_mcp_start"
	MsgErrServerStart       = "err_server_start"
	MsgServerExitErr        = "server_exit_error"
	MsgServerDone           = "server_done"
	MsgWaitingReady         = "waiting_ready"
	MsgServerReady          = "server_ready"
	MsgWarnProxyExposure    = "warn_proxy_exposure"
	MsgStartingProxy        = "starting_proxy"
	MsgProxyLogFile         = "proxy_log_file"
	MsgWarnProxyLogDisabled = "warn_proxy_log_disabled"
	MsgErrProxyStart        = "err_proxy_start"
	MsgProxyNotUp           = "proxy_not_up"
	MsgWaitingMcpReady      = "waiting_mcp_ready"
	MsgWarnMcpNotReady      = "warn_mcp_not_ready"
	MsgWarnMcpConfigWrite   = "warn_mcp_config_write"
	MsgMcpConfigLine        = "mcp_config_line"
	MsgErrMcpAddrInvalid    = "err_mcp_addr_invalid"
	MsgClaudeNotFound       = "claude_not_found"
	MsgClaudeTtyRedirect    = "claude_tty_redirect"
	MsgWarnNoTty            = "warn_no_tty"
	MsgClaudeArgsLine       = "claude_args_line"
	MsgClaudeRunLine        = "claude_run_line"
	MsgErrClaudeDirAbs      = "err_claude_dir_abs"
	MsgErrClaudeDirUnavail  = "err_claude_dir_unavail"
	MsgAnthropicBaseURL     = "anthropic_base_url"
	MsgAnthropicTokenHidden = "anthropic_token_hidden"
	MsgErrClaudeStart       = "err_claude_start"
	MsgClaudeExitCode       = "claude_exit_code"
	MsgClaudeExitErr        = "claude_exit_error"
	MsgClaudeDone           = "claude_done"
	MsgDone                 = "done"
	MsgSignalReceived       = "signal_received"
	MsgSignalRepeatEscalate = "signal_repeat_escalate"
	MsgEscalation           = "escalation"

	// --- Update mode (run.go, update.go) ---
	MsgUpdateSuccess       = "update_success"
	MsgErrUpdate           = "err_update"
	MsgErrSrcAbs           = "err_src_abs"
	MsgErrSrcNotFound      = "err_src_not_found"
	MsgErrSrcNotDir        = "err_src_not_dir"
	MsgNoUpstreamSkipPull  = "no_upstream_skip_pull"
	MsgGitCmd              = "git_cmd"
	MsgErrGitPull          = "err_git_pull"
	MsgErrInstallPrefixAbs = "err_install_prefix_abs"
	MsgCMakeCmd            = "cmake_cmd"
	MsgErrCMakeConfigure   = "err_cmake_configure"
	MsgBuilding            = "building"
	MsgErrCMakeBuild       = "err_cmake_build"
	MsgInstalling          = "installing"
	MsgErrCMakeInstall     = "err_cmake_install"

	// --- Doctor report (doctor.go) ---
	MsgDoctorVersion      = "doctor_version"
	MsgDoctorConfig       = "doctor_config"
	MsgDoctorConfigMiss   = "doctor_config_missing"
	MsgDoctorProfile      = "doctor_profile"
	MsgErrDoctorConfig    = "doctor_err_config"
	MsgLblModel           = "lbl_model"
	MsgLblServerBin       = "lbl_server_bin"
	MsgLblMmproj          = "lbl_mmproj"
	MsgLblSpecDraft       = "lbl_spec_draft"
	MsgLblChatTemplate    = "lbl_chat_template"
	MsgLblClaude          = "lbl_claude"
	MsgLblOpenaiKey       = "lbl_openai_key_fmt"
	MsgOpenaiKeyLine      = "openai_key_line"
	MsgNotSet             = "not_set"
	MsgDoctorNotSet       = "doctor_not_set"
	MsgDoctorPathErr      = "doctor_path_err"
	MsgDoctorNotFound     = "doctor_not_found"
	MsgDoctorIsDir        = "doctor_is_dir"
	MsgDoctorOk           = "doctor_ok"
	MsgDoctorClaudeNoPath = "doctor_claude_no_path"
	MsgKeySrcEnvRunner    = "key_src_env_runner"
	MsgKeySrcEnvOpenai    = "key_src_env_openai"
	MsgKeySrcSet          = "key_src_set"
	MsgDoctorServerCmd    = "doctor_server_cmd"

	// --- Proxy (proxy.go, convert.go) ---
	MsgBodyTooLarge     = "proxy_body_too_large"
	MsgInvalidJSON      = "proxy_invalid_json"
	MsgConvertErr       = "proxy_convert_error"
	MsgLlamaUpstream    = "proxy_llama_upstream"
	MsgOpenaiUpstream   = "proxy_openai_upstream"
	MsgUpstreamErr      = "proxy_upstream_connect_error"
	MsgUpstreamOpenai   = "proxy_upstream_openai_error"
	MsgStreamingUnsupp  = "proxy_streaming_unsupported"
	MsgStreamNoDone     = "proxy_stream_no_done"
	MsgStreamInterrupte = "proxy_stream_interrupted"
	MsgProxyAuthReject  = "proxy_auth_reject"
	MsgProxyUpstream    = "proxy_upstream_line"
	MsgProxyUsage       = "proxy_usage_line"
	MsgProxyDropped     = "proxy_dropped_blocks"
	MsgSummaryUnknown   = "summary_unknown"
	MsgSummaryMessages  = "summary_messages"
)

// messages is the complete translation catalog: one entry per key with the
// Russian and English variants side by side.
var messages = map[string]i18nMessage{
	// --- General / process helpers ---
	MsgStatus:             {"статус %d", "status %d"},
	MsgFinishedBeforeRdy:  {"%s завершился до готовности", "%s finished before becoming ready"},
	MsgTimeout:            {"таймаут", "timeout"},
	MsgNotReadyInTime:     {"%s не стал готов за %v: %v", "%s didn't become ready in %v: %v"},
	MsgLabelServer:        {"сервер", "server"},
	MsgMcpExitErr:         {"mcp-search-server завершился с ошибкой: %v", "mcp-search-server exited with an error: %v"},
	MsgMcpKill:            {"mcp-search-server не завершился за 3 секунды — убиваем", "mcp-search-server did not exit within 3 seconds — killing it"},
	MsgMcpWaitErr:         {"ошибка ожидания mcp-search-server: %v", "error waiting for mcp-search-server: %v"},
	MsgMcpKillUnconfirmed: {"предупреждение: процесс mcp-search-server не подтвердил завершение после Kill", "warning: mcp-search-server did not confirm termination after Kill"},

	// --- Config resolution and validation ---
	MsgErrHomeDir:              {"не удалось получить домашний каталог", "failed to get home directory"},
	MsgErrConfigFileNotFound:   {"конфиг-файл %q не найден", "config file %q not found"},
	MsgErrProfileNoConfig:      {"профиль %q не найден: конфиг-файл отсутствует", "profile %q not found: config file is missing"},
	MsgErrProfileNotFound:      {"профиль %q не найден в конфиге", "profile %q not found in config"},
	MsgErrReadyTimeoutPositive: {"-ready-timeout должен быть больше нуля", "-ready-timeout must be greater than zero"},
	MsgErrHostEmpty:            {"-host не может быть пустым", "-host cannot be empty"},
	MsgErrPortBad:              {"некорректный -port %q: ожидается число от 1 до 65535", "invalid -port %q: expected a number from 1 to 65535"},
	MsgErrMcpAddrEmpty:         {"-mcp-addr не может быть пустым", "-mcp-addr cannot be empty"},
	MsgErrMcpAddrHostPort:      {"некорректный -mcp-addr %q: используйте host:port", "invalid -mcp-addr %q: use host:port"},
	MsgErrReasoning:            {"некорректный -reasoning %q: ожидается on, off или auto", "invalid -reasoning %q: expected on, off or auto"},
	MsgErrReasoningBudget:      {"некорректный -reasoning-budget %q: ожидается -1, 0 или положительное число", "invalid -reasoning-budget %q: expected -1, 0 or a positive number"},
	MsgErrOpenaiRequired:       {"-openai обязателен при использовании -key или -model-openai", "-openai is required when using -key or -model-openai"},
	MsgErrKeyRequired:          {"-key обязателен при использовании -openai", "-key is required when using -openai"},
	MsgErrModelOpenaiRequired:  {"-model-openai обязателен при использовании -openai", "-model-openai is required when using -openai"},
	MsgErrOpenaiURLBad:         {"некорректный -openai URL %q: %v", "invalid -openai URL %q: %v"},
	MsgErrProxyAddrBad:         {"некорректный -proxy-addr %q: используйте host:port", "invalid -proxy-addr %q: use host:port"},
	MsgErrTemplateAbs:          {"не удалось определить абсолютный путь chat-template", "failed to determine absolute chat-template path"},
	MsgErrTemplateMissing:      {"указанный -chat-template недоступен или не является обычным файлом: %q (%v)", "the specified -chat-template is unavailable or not a regular file: %q (%v)"},
	MsgWarnCredCheck:           {"предупреждение: не удалось проверить %q: %v", "warning: failed to check %q: %v"},
	MsgWarnCredPerms:           {"предупреждение: %q имеет права %o, рекомендуется 0600", "warning: %q has permissions %o, 0600 is recommended"},
	MsgWarnCredRead:            {"предупреждение: не удалось прочитать %q: %v", "warning: failed to read %q: %v"},
	MsgErrReadyTimeoutBad:      {"некорректный readyTimeout %q", "invalid readyTimeout %q"},
	MsgErrConfigRead:           {"не удалось прочитать конфиг %q", "failed to read config %q"},
	MsgErrConfigJSON:           {"некорректный JSON в конфиге %q", "invalid JSON in config %q"},

	// --- Flag help descriptions ---
	MsgFlagDir:             {"рабочая директория...", "working directory..."},
	MsgFlagHost:            {"адрес...", "server address..."},
	MsgFlagPort:            {"порт...", "server port..."},
	MsgFlagModel:           {"путь к GGUF...", "path to GGUF model..."},
	MsgFlagBin:             {"путь к llama-server...", "path to llama-server..."},
	MsgFlagC:               {"размер контекста (-c)", "context size (-c)"},
	MsgFlagB:               {"размер батча (-b)", "batch size (-b)"},
	MsgFlagUb:              {"размер микро-батча (-ub)", "micro-batch size (-ub)"},
	MsgFlagFa:              {"flash attention (-fa)", "flash attention (-fa)"},
	MsgFlagParallel:        {"число параллельных слотов", "number of parallel slots"},
	MsgFlagTools:           {"режим инструментов", "tool mode"},
	MsgFlagJinja:           {"включить Jinja-шаблоны", "enable Jinja templates"},
	MsgFlagMcpProxy:        {"включить CORS-прокси", "enable CORS proxy"},
	MsgFlagLazyMmap:        {"mmap с ленивым режимом", "mmap with lazy mode"},
	MsgFlagVerbose:         {"печатать команду", "print the command before launch"},
	MsgFlagNgl:             {"число слоёв на GPU", "number of layers on GPU"},
	MsgFlagMmproj:          {"путь к mmproj", "path to mmproj"},
	MsgFlagTemp:            {"температура", "temperature"},
	MsgFlagTopP:            {"top-p", "top-p"},
	MsgFlagCacheK:          {"тип K-кэша", "K-cache type"},
	MsgFlagCacheV:          {"тип V-кэша", "V-cache type"},
	MsgFlagFit:             {"режим --fit", "--fit mode"},
	MsgFlagLv:              {"уровень логов (-lv)", "log verbosity (-lv)"},
	MsgFlagReasoning:       {"режим размышлений: on|off|auto (пусто — значение llama-server по умолчанию)", "reasoning mode: on|off|auto (empty = llama-server default)"},
	MsgFlagReasoningBudget: {"бюджет токенов на размышления: -1 (без лимита), 0 (отключить), N (положительное число)", "reasoning token budget: -1 (unlimited), 0 (disabled), N (positive number)"},
	MsgFlagSpecType:        {"тип spec-декодирования", "speculative decoding type"},
	MsgFlagSpecDraftModel:  {"путь к драфтеру", "path to draft model"},
	MsgFlagSpecDraftNMax:   {"n-max драфтера", "draft n-max"},
	MsgFlagSpecDraftNGL:    {"ngl драфтера", "draft ngl"},
	MsgFlagUpdate:          {"обновить llama.cpp", "update llama.cpp"},
	MsgFlagLlamaSrc:        {"путь к исходникам", "path to llama.cpp sources"},
	MsgFlagInstallPrefix:   {"префикс установки", "install prefix"},
	MsgFlagCuda:            {"собирать с CUDA", "build with CUDA"},
	MsgFlagCudaArch:        {"архитектура CUDA", "CUDA architecture"},
	MsgFlagServer:          {"только llama-server", "llama-server only"},
	MsgFlagClaudeBin:       {"путь к claude", "path to claude"},
	MsgFlagClaudeDir:       {"рабочая директория claude", "working directory for claude"},
	MsgFlagAlias:           {"алиас модели", "model alias"},
	MsgFlagReadyTimeout:    {"таймаут готовности", "readiness timeout"},
	MsgFlagChatTemplate:    {"шаблон чата", "chat template"},
	MsgFlagLogFile:         {"лог-файл", "log file"},
	MsgFlagQuietServer:     {"уводить логи в файл", "redirect server logs to file"},
	MsgFlagLogMaxMB:        {"макс. размер лога llama-server в МБ перед ротацией (0 — не ротировать)", "max llama-server log size in MB before rotation (0 = no rotation)"},
	MsgFlagLogKeep:         {"сколько архивов лога хранить (0 — без архивов)", "number of log archives to keep (0 = no archives)"},
	MsgFlagMcpAddr:         {"адрес mcp-search-server", "mcp-search-server address"},
	MsgFlagMcpConfig:       {"куда записать MCP-конфиг", "where to write the MCP config"},
	MsgFlagMcpStrict:       {"strict-mcp-config", "strict-mcp-config"},
	MsgFlagVersion:         {"показать версию", "show version"},
	MsgFlagOpenai:          {"URL OpenAI-совместимого API", "OpenAI-compatible API URL"},
	MsgFlagKey:             {"API-ключ для внешнего провайдера", "API key for the external provider"},
	MsgFlagModelOpenai:     {"Название модели у внешнего провайдера", "model name at the external provider"},
	MsgFlagProxyAddr:       {"адрес локального прокси (гибридный режим)", "local proxy address (hybrid mode)"},
	MsgFlagProxyToken:      {"токен для аутентификации входящих запросов прокси (пусто — без проверки)", "token for incoming proxy requests (empty = no check)"},
	MsgFlagProxyAllowRem:   {"разрешить не-loopback -proxy-addr без предупреждения", "allow non-loopback -proxy-addr without warning"},
	MsgFlagConfig:          {"путь к конфиг-файлу (\"\" — не читать)", "config file path (\"\" = don't read)"},
	MsgFlagP:               {"профиль из конфиг-файла", "profile from config file"},
	MsgFlagDoctor:          {"показать разрешённые пути и версию, не запуская процессы", "show resolved paths and version without starting processes"},

	// --- Launch flow ---
	MsgWarnOpenaiIgnored:    {"предупреждение: -openai/-key/-model-openai игнорируются в режиме -server", "warning: -openai/-key/-model-openai are ignored in -server mode"},
	MsgErrBinAbs:            {"не удалось определить абсолютный путь llama-server: %v", "failed to determine absolute llama-server path: %v"},
	MsgErrModelAbs:          {"не удалось определить абсолютный путь модели: %v", "failed to determine absolute model path: %v"},
	MsgErrMmprojAbs:         {"не удалось определить абсолютный путь mmproj: %v", "failed to determine absolute mmproj path: %v"},
	MsgErrDraftAbs:          {"не удалось определить абсолютный путь spec-draft-model: %v", "failed to determine absolute spec-draft-model path: %v"},
	MsgBinNotFound:          {"llama-server не найден по пути %q: %v", "llama-server not found at %q: %v"},
	MsgModelNotFound:        {"модель не найдена по пути %q: %v", "model not found at %q: %v"},
	MsgMmprojNotFound:       {"mmproj не найден по пути %q: %v", "mmproj not found at %q: %v"},
	MsgDraftNotFound:        {"spec-draft-model не найден по пути %q: %v", "spec-draft-model not found at %q: %v"},
	MsgTemplateAutoDetected: {"Автоопределение шаблона: qwen → %s", "Auto-detected template: qwen → %s"},
	MsgErrHomeDirShort:      {"не удалось получить домашний каталог: %v", "failed to get home directory: %v"},
	MsgErrResolved:          {"ошибка: %v", "error: %v"},
	MsgWarnTemplateAuto:     {"предупреждение: автоматически выбранный шаблон %q недоступен; использую шаблон из GGUF", "warning: automatically selected template %q is unavailable; using the template embedded in the GGUF"},
	MsgLdLibPathAlready:     {"LD_LIBRARY_PATH для llama-server: %s (уже содержит %s)", "LD_LIBRARY_PATH for llama-server: %s (already contains %s)"},
	MsgLdLibPath:            {"LD_LIBRARY_PATH для llama-server: %s", "LD_LIBRARY_PATH for llama-server: %s"},
	MsgErrWorkDirAbs:        {"не удалось определить абсолютный путь рабочей директории: %v", "failed to determine absolute working directory path: %v"},
	MsgErrWorkDirUnavail:    {"рабочая директория %q недоступна: %v", "working directory %q is unavailable: %v"},
	MsgErrLogDirCreate:      {"не удалось создать каталог для логов %q: %v", "failed to create log directory %q: %v"},
	MsgErrLogOpenRotating:   {"не удалось открыть ротируемый лог-файл %q: %v", "failed to open rotating log file %q: %v"},
	MsgErrLogOpen:           {"не удалось открыть лог-файл %q: %v", "failed to open log file %q: %v"},
	MsgErrLogChmod:          {"не удалось установить права доступа к лог-файлу %q: %v", "failed to set log file permissions %q: %v"},
	MsgWorkDirLine:          {"Рабочая директория: %s", "Working directory: %s"},
	MsgCommandLine:          {"Команда: %s %s", "Command: %s %s"},
	MsgServerLogFile:        {"Логи llama-server: %s", "llama-server logs: %s"},
	MsgStartingServer:       {"Запуск llama-server на %s:%s ...", "Starting llama-server on %s:%s ..."},
	MsgMcpCommandLine:       {"MCP-сервер: SEARCH_ENABLED=true mcp-search-server --addr %s", "MCP server: SEARCH_ENABLED=true mcp-search-server --addr %s"},
	MsgErrMcpStart:          {"ошибка запуска mcp-search-server: %v", "failed to start mcp-search-server: %v"},
	MsgErrServerStart:       {"ошибка запуска сервера: %v", "failed to start the server: %v"},
	MsgServerExitErr:        {"сервер завершился с ошибкой: %v", "the server exited with an error: %v"},
	MsgServerDone:           {"llama-server завершён.", "llama-server has finished."},
	MsgWaitingReady:         {"Ожидание готовности сервера (%s), таймаут %v ...", "Waiting for the server to become ready (%s), timeout %v ..."},
	MsgServerReady:          {"llama-server готов. Запуск Claude Code...", "llama-server is ready. Starting Claude Code..."},
	MsgWarnProxyExposure:    {"предупреждение: прокси слушает не-loopback адрес %s без аутентификации — любой в сети может использовать ваш платный ключ. Задайте -proxy-token (и тот же токен в Claude Code) или явный -proxy-allow-remote.", "warning: the proxy listens on the non-loopback address %s without authentication — anyone on the network can use your paid key. Set -proxy-token (and the same token in Claude Code) or an explicit -proxy-allow-remote."},
	MsgStartingProxy:        {"Запуск прокси на %s (llama → %s, openai → %s)", "Starting the proxy on %s (llama → %s, openai → %s)"},
	MsgProxyLogFile:         {"Логи прокси: %s", "Proxy logs: %s"},
	MsgWarnProxyLogDisabled: {"предупреждение: не удалось открыть лог-файл для [proxy] — логи прокси отключены, чтобы не портить TUI Claude Code", "warning: could not open the [proxy] log file — proxy logging is disabled to avoid corrupting the Claude Code TUI"},
	MsgErrProxyStart:        {"ошибка запуска прокси: %v", "failed to start the proxy: %v"},
	MsgProxyNotUp:           {"прокси не поднялся: %v", "the proxy did not come up: %v"},
	MsgWaitingMcpReady:      {"Ожидание готовности MCP-сервера (%s) ...", "Waiting for the MCP server to become ready (%s) ..."},
	MsgWarnMcpNotReady:      {"предупреждение: mcp-search-server не готов: %v\nClaude Code будет запущен без web_search.", "warning: mcp-search-server is not ready: %v\nClaude Code will start without web_search."},
	MsgWarnMcpConfigWrite:   {"предупреждение: не удалось создать MCP-конфиг: %v", "warning: failed to create the MCP config: %v"},
	MsgMcpConfigLine:        {"MCP-конфиг: %s (url=%s, type=http)", "MCP config: %s (url=%s, type=http)"},
	MsgErrMcpAddrInvalid:    {"некорректный -mcp-addr %q: %v", "invalid -mcp-addr %q: %v"},
	MsgClaudeNotFound:       {"Claude Code не найден (%q): %v", "Claude Code not found (%q): %v"},
	MsgClaudeTtyRedirect:    {"Claude Code: stdio перенаправлен в /dev/tty (родительские stdout/stderr — не TTY)", "Claude Code: stdio redirected to /dev/tty (parent stdout/stderr are not a TTY)"},
	MsgWarnNoTty:            {"предупреждение: stdout/stderr не TTY, /dev/tty недоступен — Claude Code может не запуститься интерактивно", "warning: stdout/stderr are not a TTY and /dev/tty is unavailable — Claude Code may fail to start interactively"},
	MsgClaudeArgsLine:       {"Claude Code args: %v", "Claude Code args: %v"},
	MsgClaudeRunLine:        {"Claude Code: %s (cwd=%s)", "Claude Code: %s (cwd=%s)"},
	MsgErrClaudeDirAbs:      {"не удалось определить абсолютный путь claude-dir: %v", "failed to determine absolute claude-dir path: %v"},
	MsgErrClaudeDirUnavail:  {"claude-dir %q недоступна: %v", "claude-dir %q is unavailable: %v"},
	MsgAnthropicBaseURL:     {"ANTHROPIC_BASE_URL=%s", "ANTHROPIC_BASE_URL=%s"},
	MsgAnthropicTokenHidden: {"ANTHROPIC_AUTH_TOKEN=<configured via -proxy-token, hidden>", "ANTHROPIC_AUTH_TOKEN=<configured via -proxy-token, hidden>"},
	MsgErrClaudeStart:       {"ошибка запуска Claude Code: %v", "failed to start Claude Code: %v"},
	MsgClaudeExitCode:       {"Claude Code завершён с кодом %d. Останавливаем llama-server...", "Claude Code exited with code %d. Stopping llama-server..."},
	MsgClaudeExitErr:        {"Claude Code завершился с ошибкой: %v", "Claude Code exited with an error: %v"},
	MsgClaudeDone:           {"Claude Code завершён. Останавливаем llama-server...", "Claude Code has finished. Stopping llama-server..."},
	MsgDone:                 {"Готово.", "Done."},
	MsgSignalReceived:       {"\nПолучен сигнал %v — останавливаем дочерние процессы...", "\nReceived signal %v — stopping child processes..."},
	MsgSignalRepeatEscalate: {"\nПовторный сигнал %v — эскалация без ожидания", "\nRepeated signal %v — escalating without waiting"},
	MsgEscalation:           {"\nЭскалация: %s", "\nEscalation: %s"},

	// --- Update mode ---
	MsgUpdateSuccess:       {"llama.cpp успешно обновлён и установлен.", "llama.cpp has been updated and installed successfully."},
	MsgErrUpdate:           {"ошибка обновления llama.cpp: %v", "llama.cpp update failed: %v"},
	MsgErrSrcAbs:           {"не удалось определить абсолютный путь исходников", "failed to determine absolute sources path"},
	MsgErrSrcNotFound:      {"исходники llama.cpp не найдены по пути %q", "llama.cpp sources not found at %q"},
	MsgErrSrcNotDir:        {"путь %q не является каталогом", "path %q is not a directory"},
	MsgNoUpstreamSkipPull:  {"  (у текущей ветки нет upstream — git pull пропущен, используем текущий код)", "  (current branch has no upstream — git pull skipped, building the current checkout)"},
	MsgGitCmd:              {"  git %s", "  git %s"},
	MsgErrGitPull:          {"git pull", "git pull"},
	MsgErrInstallPrefixAbs: {"не удалось определить абсолютный install-prefix", "failed to determine absolute install-prefix"},
	MsgCMakeCmd:            {"  cmake %s", "  cmake %s"},
	MsgErrCMakeConfigure:   {"cmake configure", "cmake configure"},
	MsgBuilding:            {"Сборка (%d потоков)...", "Building (%d threads)..."},
	MsgErrCMakeBuild:       {"cmake build", "cmake build"},
	MsgInstalling:          {"Установка в %s...", "Installing into %s..."},
	MsgErrCMakeInstall:     {"cmake install", "cmake install"},

	// --- Doctor report ---
	MsgDoctorVersion:      {"  версия:         %s", "  version:        %s"},
	MsgDoctorConfig:       {"  конфиг:         %s", "  config:         %s"},
	MsgDoctorConfigMiss:   {"  конфиг:         %s (не найден — встроенные дефолты)", "  config:         %s (not found — built-in defaults)"},
	MsgDoctorProfile:      {"  профиль:        %s", "  profile:        %s"},
	MsgErrDoctorConfig:    {"  ошибка конфига: %v", "  config error: %v"},
	MsgLblModel:           {"модель:", "model:"},
	MsgLblServerBin:       {"llama-server:", "llama-server:"},
	MsgLblMmproj:          {"mmproj:", "mmproj:"},
	MsgLblSpecDraft:       {"spec-draft:", "spec-draft:"},
	MsgLblChatTemplate:    {"chat-template:", "chat-template:"},
	MsgLblClaude:          {"claude:", "claude:"},
	MsgLblOpenaiKey:       {"OpenAI-ключ:", "OpenAI key:"},
	MsgOpenaiKeyLine:      {"  %-15s %s", "  %-15s %s"},
	MsgNotSet:             {"(не задан)", "(not set)"},
	MsgDoctorNotSet:       {"  %-15s (не задан)", "  %-15s (not set)"},
	MsgDoctorPathErr:      {"  %-15s %s (ошибка пути: %v)", "  %-15s %s (path error: %v)"},
	MsgDoctorNotFound:     {"  %-15s %s (НЕ найден: %v)", "  %-15s %s (NOT FOUND: %v)"},
	MsgDoctorIsDir:        {"  %-15s %s (ЭТО КАТАЛОГ)", "  %-15s %s (IS A DIRECTORY)"},
	MsgDoctorOk:           {"  %-15s %s (OK)", "  %-15s %s (OK)"},
	MsgDoctorClaudeNoPath: {"  %-15s %s (НЕ найден в PATH)", "  %-15s %s (NOT found in PATH)"},
	MsgKeySrcEnvRunner:    {"задан (env LLAMA_RUNNER_OPENAI_KEY)", "set (env LLAMA_RUNNER_OPENAI_KEY)"},
	MsgKeySrcEnvOpenai:    {"задан (env OPENAI_API_KEY)", "set (env OPENAI_API_KEY)"},
	MsgKeySrcSet:          {"задан", "set"},
	MsgDoctorServerCmd:    {"\nКоманда llama-server:\n  %s %s", "\nllama-server command:\n  %s %s"},

	// --- Proxy ---
	MsgBodyTooLarge:     {"тело запроса больше %d байт", "request body larger than %d bytes"},
	MsgInvalidJSON:      {"invalid JSON: %v", "invalid JSON: %v"},
	MsgConvertErr:       {"convert: %v", "convert: %v"},
	MsgLlamaUpstream:    {"llama-server upstream: %s", "llama-server upstream: %s"},
	MsgOpenaiUpstream:   {"openai upstream: %s", "openai upstream: %s"},
	MsgUpstreamErr:      {"llama-server: %v", "llama-server: %v"},
	MsgUpstreamOpenai:   {"openai: %v", "openai: %v"},
	MsgStreamingUnsupp:  {"streaming unsupported", "streaming unsupported"},
	MsgStreamNoDone:     {"upstream stream ended before [DONE]", "upstream stream ended before [DONE]"},
	MsgStreamInterrupte: {"upstream stream interrupted: %v", "upstream stream interrupted: %v"},
	MsgProxyAuthReject:  {"auth rejected: %s %s (token mismatch or missing)", "auth rejected: %s %s (token mismatch or missing)"},
	MsgProxyUpstream:    {"→ апстрим: %s", "→ upstream: %s"},
	MsgProxyUsage:       {"← usage: input_tokens=%d, output_tokens=%d", "← usage: input_tokens=%d, output_tokens=%d"},
	MsgProxyDropped:     {"отброшено блоков: %s (thinking не имеет аналога в OpenAI, image требует -mmproj)", "dropped blocks: %s (thinking has no OpenAI equivalent, image requires -mmproj)"},
	MsgSummaryUnknown:   {"%d неизвестных", "%d unknown"},
	MsgSummaryMessages:  {"%d сообщений: system=%v, assistant с tool_calls=%d, tool-ответов=%d, схем инструментов=%d", "%d messages: system=%v, assistant with tool_calls=%d, tool replies=%d, tool schemas=%d"},
}
