package main

import (
	"fmt"
	"strings"
)

// This file implements the launcher's lightweight localization: every message
// printed to the terminal (progress lines, warnings, errors, the -doctor
// report, proxy diagnostics) or surfaced as an error is looked up here and
// rendered in the console language — Russian when isRussianLocale reports a
// Russian locale, English otherwise.
//
// Usage at call sites:
//
//	fmt.Fprint(stderr, TF(msgKey, arg1, arg2), "\n") // localized line to a stream
//	err := errorf(msgKey, err)                        // wrapped %w error
//	err := errorv(msgKey, args...)                    // %v-style error
//
// TF always appends a trailing newline; templates must not carry "\n"
// themselves. Templates keep the original format specifiers (%s, %q, %d, %v,
// %w), and the same argument list is passed to both language variants, so
// every entry's ru and en templates must use identical specifiers.
//
// Tests must not depend on the ambient locale: pin it explicitly with
// t.Setenv("LC_ALL", "ru_RU.UTF-8") (or "C") when asserting message text.

// i18nMessage is one message in all supported languages. The ru and en
// templates must contain the same set of format specifiers.
type i18nMessage struct{ ru, en string }

// T returns the localized message template for key. An unknown key returns
// the key itself, which makes missing translations visible instead of
// crashing or printing nothing.
func T(key string) string {
	m, ok := messages[key]
	if !ok {
		return key
	}
	if isRussianLocale() {
		return m.ru
	}
	return m.en
}

// TF renders the localized template for key with args (fmt.Sprintf) and
// appends a newline. Use it with fmt.Fprint to emit a localized line:
//
//	fmt.Fprint(stderr, TF(msgKey, value))
func TF(key string, args ...any) string {
	return fmt.Sprintf(T(key), args...) + "\n"
}

// errorf wraps cause into a localized error of the form "<message>: <cause>".
// The cause is always attached with %w, preserving errors.Is/As chains, so
// templates never carry a format specifier for it.
func errorf(key string, cause error, args ...any) error {
	return fmt.Errorf("%s: %w", strings.TrimSuffix(TF(key, args...), "\n"), cause)
}

// errorv builds a localized error from the template itself; the template's
// format specifiers consume args. Use it when there is no single wrapped
// cause (validation failures and similar).
func errorv(key string, args ...any) error {
	return fmt.Errorf("%s", strings.TrimSuffix(TF(key, args...), "\n"))
}

// Message keys are declared as constants and used as catalog keys at the
// declaration site of messages below, so a missing translation is impossible
// by construction: each constant appears in exactly one catalog entry.

const (
	// --- General / process helpers (main.go) ---
	msgStatus             = "status"
	msgFinishedBeforeRdy  = "finished_before_ready"
	msgTimeout            = "timeout"
	msgNotReadyInTime     = "not_ready_in_time"
	msgLabelServer        = "label_server"
	msgMcpExitErr         = "mcp_exit_error"
	msgMcpKill            = "mcp_kill"
	msgMcpWaitErr         = "mcp_wait_error"
	msgMcpKillUnconfirmed = "mcp_kill_unconfirmed"

	// --- Config resolution and validation (parse.go, config_file.go) ---
	msgErrHomeDir              = "err_home_dir"
	msgErrConfigFileNotFound   = "err_config_file_not_found"
	msgErrProfileNoConfig      = "err_profile_no_config"
	msgErrProfileNotFound      = "err_profile_not_found"
	msgErrReadyTimeoutPositive = "err_ready_timeout_positive"
	msgErrHostEmpty            = "err_host_empty"
	msgErrPortBad              = "err_port_bad"
	msgErrMcpAddrEmpty         = "err_mcp_addr_empty"
	msgErrMcpAddrHostPort      = "err_mcp_addr_hostport"
	msgErrReasoning            = "err_reasoning"
	msgErrReasoningBudget      = "err_reasoning_budget"
	msgErrOpenaiRequired       = "err_openai_required"
	msgErrKeyRequired          = "err_key_required"
	msgErrModelOpenaiRequired  = "err_model_openai_required"
	msgErrOpenaiURLBad         = "err_openai_url_bad"
	msgErrProxyAddrBad         = "err_proxy_addr_bad"
	msgErrTemplateAbs          = "err_template_abs"
	msgErrTemplateMissing      = "err_template_missing"
	msgWarnCredCheck           = "warn_cred_check"
	msgWarnCredPerms           = "warn_cred_perms"
	msgWarnCredRead            = "warn_cred_read"
	msgErrReadyTimeoutBad      = "err_ready_timeout_bad"
	msgErrConfigRead           = "err_config_read"
	msgErrConfigJSON           = "err_config_json"

	// --- Flag help descriptions (parse.go) ---
	msgFlagDir             = "flag_dir"
	msgFlagHost            = "flag_host"
	msgFlagPort            = "flag_port"
	msgFlagModel           = "flag_model"
	msgFlagBin             = "flag_bin"
	msgFlagC               = "flag_c"
	msgFlagB               = "flag_b"
	msgFlagUb              = "flag_ub"
	msgFlagFa              = "flag_fa"
	msgFlagParallel        = "flag_parallel"
	msgFlagTools           = "flag_tools"
	msgFlagJinja           = "flag_jinja"
	msgFlagMcpProxy        = "flag_webui_mcp_proxy"
	msgFlagLazyMmap        = "flag_lm_lazy"
	msgFlagVerbose         = "flag_v"
	msgFlagNgl             = "flag_ngl"
	msgFlagMmproj          = "flag_mmproj"
	msgFlagTemp            = "flag_temp"
	msgFlagTopP            = "flag_top_p"
	msgFlagCacheK          = "flag_cache_type_k"
	msgFlagCacheV          = "flag_cache_type_v"
	msgFlagFit             = "flag_fit"
	msgFlagLv              = "flag_lv"
	msgFlagReasoning       = "flag_reasoning"
	msgFlagReasoningBudget = "flag_reasoning_budget"
	msgFlagSpecType        = "flag_spec_type"
	msgFlagSpecDraftModel  = "flag_spec_draft_model"
	msgFlagSpecDraftNMax   = "flag_spec_draft_n_max"
	msgFlagSpecDraftNGL    = "flag_spec_draft_ngl"
	msgFlagUpdate          = "flag_update"
	msgFlagLlamaSrc        = "flag_llama_src"
	msgFlagInstallPrefix   = "flag_install_prefix"
	msgFlagCuda            = "flag_cuda"
	msgFlagCudaArch        = "flag_cuda_arch"
	msgFlagServer          = "flag_server"
	msgFlagClaudeBin       = "flag_claude_bin"
	msgFlagClaudeDir       = "flag_claude_dir"
	msgFlagAlias           = "flag_alias"
	msgFlagReadyTimeout    = "flag_ready_timeout"
	msgFlagChatTemplate    = "flag_chat_template"
	msgFlagLogFile         = "flag_log_file"
	msgFlagQuietServer     = "flag_quiet_server"
	msgFlagLogMaxMB        = "flag_log_max_mb"
	msgFlagLogKeep         = "flag_log_keep"
	msgFlagMcpAddr         = "flag_mcp_addr"
	msgFlagMcpConfig       = "flag_mcp_config"
	msgFlagMcpStrict       = "flag_mcp_strict"
	msgFlagVersion         = "flag_version"
	msgFlagOpenai          = "flag_openai"
	msgFlagKey             = "flag_key"
	msgFlagModelOpenai     = "flag_model_openai"
	msgFlagProxyAddr       = "flag_proxy_addr"
	msgFlagProxyToken      = "flag_proxy_token"
	msgFlagProxyAllowRem   = "flag_proxy_allow_remote"
	msgFlagConfig          = "flag_config"
	msgFlagP               = "flag_p"
	msgFlagDoctor          = "flag_doctor"

	// --- Launch flow (run.go) ---
	msgWarnOpenaiIgnored    = "warn_openai_ignored"
	msgErrBinAbs            = "err_bin_abs"
	msgErrModelAbs          = "err_model_abs"
	msgErrMmprojAbs         = "err_mmproj_abs"
	msgErrDraftAbs          = "err_draft_abs"
	msgBinNotFound          = "bin_not_found"
	msgModelNotFound        = "model_not_found"
	msgMmprojNotFound       = "mmproj_not_found"
	msgDraftNotFound        = "draft_not_found"
	msgTemplateAutoDetected = "template_auto_detected"
	msgErrHomeDirShort      = "err_home_dir_short"
	msgErrResolved          = "err_resolved"
	msgWarnTemplateAuto     = "warn_template_auto"
	msgLdLibPathAlready     = "ld_lib_path_already"
	msgLdLibPath            = "ld_lib_path"
	msgErrWorkDirAbs        = "err_work_dir_abs"
	msgErrWorkDirUnavail    = "err_work_dir_unavail"
	msgErrLogDirCreate      = "err_log_dir_create"
	msgErrLogOpenRotating   = "err_log_open_rotating"
	msgErrLogOpen           = "err_log_open"
	msgErrLogChmod          = "err_log_chmod"
	msgWorkDirLine          = "work_dir_line"
	msgCommandLine          = "command_line"
	msgServerLogFile        = "server_log_file"
	msgStartingServer       = "starting_server"
	msgMcpCommandLine       = "mcp_command_line"
	msgErrMcpStart          = "err_mcp_start"
	msgErrServerStart       = "err_server_start"
	msgServerExitErr        = "server_exit_error"
	msgServerDone           = "server_done"
	msgWaitingReady         = "waiting_ready"
	msgServerReady          = "server_ready"
	msgWarnProxyExposure    = "warn_proxy_exposure"
	msgStartingProxy        = "starting_proxy"
	msgProxyLogFile         = "proxy_log_file"
	msgWarnProxyLogDisabled = "warn_proxy_log_disabled"
	msgErrProxyStart        = "err_proxy_start"
	msgProxyNotUp           = "proxy_not_up"
	msgWaitingMcpReady      = "waiting_mcp_ready"
	msgWarnMcpNotReady      = "warn_mcp_not_ready"
	msgWarnMcpConfigWrite   = "warn_mcp_config_write"
	msgMcpConfigLine        = "mcp_config_line"
	msgErrMcpAddrInvalid    = "err_mcp_addr_invalid"
	msgClaudeNotFound       = "claude_not_found"
	msgClaudeTtyRedirect    = "claude_tty_redirect"
	msgWarnNoTty            = "warn_no_tty"
	msgClaudeArgsLine       = "claude_args_line"
	msgClaudeRunLine        = "claude_run_line"
	msgErrClaudeDirAbs      = "err_claude_dir_abs"
	msgErrClaudeDirUnavail  = "err_claude_dir_unavail"
	msgAnthropicBaseURL     = "anthropic_base_url"
	msgAnthropicTokenHidden = "anthropic_token_hidden"
	msgErrClaudeStart       = "err_claude_start"
	msgClaudeExitCode       = "claude_exit_code"
	msgClaudeExitErr        = "claude_exit_error"
	msgClaudeDone           = "claude_done"
	msgDone                 = "done"
	msgSignalReceived       = "signal_received"
	msgSignalRepeatEscalate = "signal_repeat_escalate"
	msgEscalation           = "escalation"

	// --- Update mode (run.go, update.go) ---
	msgUpdateSuccess       = "update_success"
	msgErrUpdate           = "err_update"
	msgErrSrcAbs           = "err_src_abs"
	msgErrSrcNotFound      = "err_src_not_found"
	msgErrSrcNotDir        = "err_src_not_dir"
	msgNoUpstreamSkipPull  = "no_upstream_skip_pull"
	msgGitCmd              = "git_cmd"
	msgErrGitPull          = "err_git_pull"
	msgErrInstallPrefixAbs = "err_install_prefix_abs"
	msgCMakeCmd            = "cmake_cmd"
	msgErrCMakeConfigure   = "err_cmake_configure"
	msgBuilding            = "building"
	msgErrCMakeBuild       = "err_cmake_build"
	msgInstalling          = "installing"
	msgErrCMakeInstall     = "err_cmake_install"

	// --- Doctor report (doctor.go) ---
	msgDoctorVersion      = "doctor_version"
	msgDoctorConfig       = "doctor_config"
	msgDoctorConfigMiss   = "doctor_config_missing"
	msgDoctorProfile      = "doctor_profile"
	msgErrDoctorConfig    = "doctor_err_config"
	msgLblModel           = "lbl_model"
	msgLblServerBin       = "lbl_server_bin"
	msgLblMmproj          = "lbl_mmproj"
	msgLblSpecDraft       = "lbl_spec_draft"
	msgLblChatTemplate    = "lbl_chat_template"
	msgLblClaude          = "lbl_claude"
	msgLblOpenaiKey       = "lbl_openai_key_fmt"
	msgOpenaiKeyLine      = "openai_key_line"
	msgNotSet             = "not_set"
	msgDoctorNotSet       = "doctor_not_set"
	msgDoctorPathErr      = "doctor_path_err"
	msgDoctorNotFound     = "doctor_not_found"
	msgDoctorIsDir        = "doctor_is_dir"
	msgDoctorOk           = "doctor_ok"
	msgDoctorClaudeNoPath = "doctor_claude_no_path"
	msgKeySrcEnvRunner    = "key_src_env_runner"
	msgKeySrcEnvOpenai    = "key_src_env_openai"
	msgKeySrcSet          = "key_src_set"
	msgDoctorServerCmd    = "doctor_server_cmd"

	// --- Proxy (proxy.go, convert.go) ---
	msgBodyTooLarge     = "proxy_body_too_large"
	msgInvalidJSON      = "proxy_invalid_json"
	msgConvertErr       = "proxy_convert_error"
	msgLlamaUpstream    = "proxy_llama_upstream"
	msgOpenaiUpstream   = "proxy_openai_upstream"
	msgUpstreamErr      = "proxy_upstream_connect_error"
	msgUpstreamOpenai   = "proxy_upstream_openai_error"
	msgStreamingUnsupp  = "proxy_streaming_unsupported"
	msgStreamNoDone     = "proxy_stream_no_done"
	msgStreamInterrupte = "proxy_stream_interrupted"
	msgProxyAuthReject  = "proxy_auth_reject"
	msgProxyUpstream    = "proxy_upstream_line"
	msgProxyUsage       = "proxy_usage_line"
	msgProxyDropped     = "proxy_dropped_blocks"
	msgSummaryUnknown   = "summary_unknown"
	msgSummaryMessages  = "summary_messages"
)

// messages is the complete translation catalog: one entry per key with the
// Russian and English variants side by side.
var messages = map[string]i18nMessage{
	// --- General / process helpers ---
	msgStatus:             {"статус %d", "status %d"},
	msgFinishedBeforeRdy:  {"%s завершился до готовности", "%s finished before becoming ready"},
	msgTimeout:            {"таймаут", "timeout"},
	msgNotReadyInTime:     {"%s не стал готов за %v: %v", "%s didn't become ready in %v: %v"},
	msgLabelServer:        {"сервер", "server"},
	msgMcpExitErr:         {"mcp-search-server завершился с ошибкой: %v", "mcp-search-server exited with an error: %v"},
	msgMcpKill:            {"mcp-search-server не завершился за 3 секунды — убиваем", "mcp-search-server did not exit within 3 seconds — killing it"},
	msgMcpWaitErr:         {"ошибка ожидания mcp-search-server: %v", "error waiting for mcp-search-server: %v"},
	msgMcpKillUnconfirmed: {"предупреждение: процесс mcp-search-server не подтвердил завершение после Kill", "warning: mcp-search-server did not confirm termination after Kill"},

	// --- Config resolution and validation ---
	msgErrHomeDir:              {"не удалось получить домашний каталог", "failed to get home directory"},
	msgErrConfigFileNotFound:   {"конфиг-файл %q не найден", "config file %q not found"},
	msgErrProfileNoConfig:      {"профиль %q не найден: конфиг-файл отсутствует", "profile %q not found: config file is missing"},
	msgErrProfileNotFound:      {"профиль %q не найден в конфиге", "profile %q not found in config"},
	msgErrReadyTimeoutPositive: {"-ready-timeout должен быть больше нуля", "-ready-timeout must be greater than zero"},
	msgErrHostEmpty:            {"-host не может быть пустым", "-host cannot be empty"},
	msgErrPortBad:              {"некорректный -port %q: ожидается число от 1 до 65535", "invalid -port %q: expected a number from 1 to 65535"},
	msgErrMcpAddrEmpty:         {"-mcp-addr не может быть пустым", "-mcp-addr cannot be empty"},
	msgErrMcpAddrHostPort:      {"некорректный -mcp-addr %q: используйте host:port", "invalid -mcp-addr %q: use host:port"},
	msgErrReasoning:            {"некорректный -reasoning %q: ожидается on, off или auto", "invalid -reasoning %q: expected on, off or auto"},
	msgErrReasoningBudget:      {"некорректный -reasoning-budget %q: ожидается -1, 0 или положительное число", "invalid -reasoning-budget %q: expected -1, 0 or a positive number"},
	msgErrOpenaiRequired:       {"-openai обязателен при использовании -key или -model-openai", "-openai is required when using -key or -model-openai"},
	msgErrKeyRequired:          {"-key обязателен при использовании -openai", "-key is required when using -openai"},
	msgErrModelOpenaiRequired:  {"-model-openai обязателен при использовании -openai", "-model-openai is required when using -openai"},
	msgErrOpenaiURLBad:         {"некорректный -openai URL %q: %v", "invalid -openai URL %q: %v"},
	msgErrProxyAddrBad:         {"некорректный -proxy-addr %q: используйте host:port", "invalid -proxy-addr %q: use host:port"},
	msgErrTemplateAbs:          {"не удалось определить абсолютный путь chat-template", "failed to determine absolute chat-template path"},
	msgErrTemplateMissing:      {"указанный -chat-template недоступен или не является обычным файлом: %q (%v)", "the specified -chat-template is unavailable or not a regular file: %q (%v)"},
	msgWarnCredCheck:           {"предупреждение: не удалось проверить %q: %v", "warning: failed to check %q: %v"},
	msgWarnCredPerms:           {"предупреждение: %q имеет права %o, рекомендуется 0600", "warning: %q has permissions %o, 0600 is recommended"},
	msgWarnCredRead:            {"предупреждение: не удалось прочитать %q: %v", "warning: failed to read %q: %v"},
	msgErrReadyTimeoutBad:      {"некорректный readyTimeout %q", "invalid readyTimeout %q"},
	msgErrConfigRead:           {"не удалось прочитать конфиг %q", "failed to read config %q"},
	msgErrConfigJSON:           {"некорректный JSON в конфиге %q", "invalid JSON in config %q"},

	// --- Flag help descriptions ---
	msgFlagDir:             {"рабочая директория...", "working directory..."},
	msgFlagHost:            {"адрес...", "server address..."},
	msgFlagPort:            {"порт...", "server port..."},
	msgFlagModel:           {"путь к GGUF...", "path to GGUF model..."},
	msgFlagBin:             {"путь к llama-server...", "path to llama-server..."},
	msgFlagC:               {"размер контекста (-c)", "context size (-c)"},
	msgFlagB:               {"размер батча (-b)", "batch size (-b)"},
	msgFlagUb:              {"размер микро-батча (-ub)", "micro-batch size (-ub)"},
	msgFlagFa:              {"flash attention (-fa)", "flash attention (-fa)"},
	msgFlagParallel:        {"число параллельных слотов", "number of parallel slots"},
	msgFlagTools:           {"режим инструментов", "tool mode"},
	msgFlagJinja:           {"включить Jinja-шаблоны", "enable Jinja templates"},
	msgFlagMcpProxy:        {"включить CORS-прокси", "enable CORS proxy"},
	msgFlagLazyMmap:        {"mmap с ленивым режимом", "mmap with lazy mode"},
	msgFlagVerbose:         {"печатать команду", "print the command before launch"},
	msgFlagNgl:             {"число слоёв на GPU", "number of layers on GPU"},
	msgFlagMmproj:          {"путь к mmproj", "path to mmproj"},
	msgFlagTemp:            {"температура", "temperature"},
	msgFlagTopP:            {"top-p", "top-p"},
	msgFlagCacheK:          {"тип K-кэша", "K-cache type"},
	msgFlagCacheV:          {"тип V-кэша", "V-cache type"},
	msgFlagFit:             {"режим --fit", "--fit mode"},
	msgFlagLv:              {"уровень логов (-lv)", "log verbosity (-lv)"},
	msgFlagReasoning:       {"режим размышлений: on|off|auto (пусто — значение llama-server по умолчанию)", "reasoning mode: on|off|auto (empty = llama-server default)"},
	msgFlagReasoningBudget: {"бюджет токенов на размышления: -1 (без лимита), 0 (отключить), N (положительное число)", "reasoning token budget: -1 (unlimited), 0 (disabled), N (positive number)"},
	msgFlagSpecType:        {"тип spec-декодирования", "speculative decoding type"},
	msgFlagSpecDraftModel:  {"путь к драфтеру", "path to draft model"},
	msgFlagSpecDraftNMax:   {"n-max драфтера", "draft n-max"},
	msgFlagSpecDraftNGL:    {"ngl драфтера", "draft ngl"},
	msgFlagUpdate:          {"обновить llama.cpp", "update llama.cpp"},
	msgFlagLlamaSrc:        {"путь к исходникам", "path to llama.cpp sources"},
	msgFlagInstallPrefix:   {"префикс установки", "install prefix"},
	msgFlagCuda:            {"собирать с CUDA", "build with CUDA"},
	msgFlagCudaArch:        {"архитектура CUDA", "CUDA architecture"},
	msgFlagServer:          {"только llama-server", "llama-server only"},
	msgFlagClaudeBin:       {"путь к claude", "path to claude"},
	msgFlagClaudeDir:       {"рабочая директория claude", "working directory for claude"},
	msgFlagAlias:           {"алиас модели", "model alias"},
	msgFlagReadyTimeout:    {"таймаут готовности", "readiness timeout"},
	msgFlagChatTemplate:    {"шаблон чата", "chat template"},
	msgFlagLogFile:         {"лог-файл", "log file"},
	msgFlagQuietServer:     {"уводить логи в файл", "redirect server logs to file"},
	msgFlagLogMaxMB:        {"макс. размер лога llama-server в МБ перед ротацией (0 — не ротировать)", "max llama-server log size in MB before rotation (0 = no rotation)"},
	msgFlagLogKeep:         {"сколько архивов лога хранить (0 — без архивов)", "number of log archives to keep (0 = no archives)"},
	msgFlagMcpAddr:         {"адрес mcp-search-server", "mcp-search-server address"},
	msgFlagMcpConfig:       {"куда записать MCP-конфиг", "where to write the MCP config"},
	msgFlagMcpStrict:       {"strict-mcp-config", "strict-mcp-config"},
	msgFlagVersion:         {"показать версию", "show version"},
	msgFlagOpenai:          {"URL OpenAI-совместимого API", "OpenAI-compatible API URL"},
	msgFlagKey:             {"API-ключ для внешнего провайдера", "API key for the external provider"},
	msgFlagModelOpenai:     {"Название модели у внешнего провайдера", "model name at the external provider"},
	msgFlagProxyAddr:       {"адрес локального прокси (гибридный режим)", "local proxy address (hybrid mode)"},
	msgFlagProxyToken:      {"токен для аутентификации входящих запросов прокси (пусто — без проверки)", "token for incoming proxy requests (empty = no check)"},
	msgFlagProxyAllowRem:   {"разрешить не-loopback -proxy-addr без предупреждения", "allow non-loopback -proxy-addr without warning"},
	msgFlagConfig:          {"путь к конфиг-файлу (\"\" — не читать)", "config file path (\"\" = don't read)"},
	msgFlagP:               {"профиль из конфиг-файла", "profile from config file"},
	msgFlagDoctor:          {"показать разрешённые пути и версию, не запуская процессы", "show resolved paths and version without starting processes"},

	// --- Launch flow ---
	msgWarnOpenaiIgnored:    {"предупреждение: -openai/-key/-model-openai игнорируются в режиме -server", "warning: -openai/-key/-model-openai are ignored in -server mode"},
	msgErrBinAbs:            {"не удалось определить абсолютный путь llama-server: %v", "failed to determine absolute llama-server path: %v"},
	msgErrModelAbs:          {"не удалось определить абсолютный путь модели: %v", "failed to determine absolute model path: %v"},
	msgErrMmprojAbs:         {"не удалось определить абсолютный путь mmproj: %v", "failed to determine absolute mmproj path: %v"},
	msgErrDraftAbs:          {"не удалось определить абсолютный путь spec-draft-model: %v", "failed to determine absolute spec-draft-model path: %v"},
	msgBinNotFound:          {"llama-server не найден по пути %q: %v", "llama-server not found at %q: %v"},
	msgModelNotFound:        {"модель не найдена по пути %q: %v", "model not found at %q: %v"},
	msgMmprojNotFound:       {"mmproj не найден по пути %q: %v", "mmproj not found at %q: %v"},
	msgDraftNotFound:        {"spec-draft-model не найден по пути %q: %v", "spec-draft-model not found at %q: %v"},
	msgTemplateAutoDetected: {"Автоопределение шаблона: qwen → %s", "Auto-detected template: qwen → %s"},
	msgErrHomeDirShort:      {"не удалось получить домашний каталог: %v", "failed to get home directory: %v"},
	msgErrResolved:          {"ошибка: %v", "error: %v"},
	msgWarnTemplateAuto:     {"предупреждение: автоматически выбранный шаблон %q недоступен; использую шаблон из GGUF", "warning: automatically selected template %q is unavailable; using the template embedded in the GGUF"},
	msgLdLibPathAlready:     {"LD_LIBRARY_PATH для llama-server: %s (уже содержит %s)", "LD_LIBRARY_PATH for llama-server: %s (already contains %s)"},
	msgLdLibPath:            {"LD_LIBRARY_PATH для llama-server: %s", "LD_LIBRARY_PATH for llama-server: %s"},
	msgErrWorkDirAbs:        {"не удалось определить абсолютный путь рабочей директории: %v", "failed to determine absolute working directory path: %v"},
	msgErrWorkDirUnavail:    {"рабочая директория %q недоступна: %v", "working directory %q is unavailable: %v"},
	msgErrLogDirCreate:      {"не удалось создать каталог для логов %q: %v", "failed to create log directory %q: %v"},
	msgErrLogOpenRotating:   {"не удалось открыть ротируемый лог-файл %q: %v", "failed to open rotating log file %q: %v"},
	msgErrLogOpen:           {"не удалось открыть лог-файл %q: %v", "failed to open log file %q: %v"},
	msgErrLogChmod:          {"не удалось установить права доступа к лог-файлу %q: %v", "failed to set log file permissions %q: %v"},
	msgWorkDirLine:          {"Рабочая директория: %s", "Working directory: %s"},
	msgCommandLine:          {"Команда: %s %s", "Command: %s %s"},
	msgServerLogFile:        {"Логи llama-server: %s", "llama-server logs: %s"},
	msgStartingServer:       {"Запуск llama-server на %s:%s ...", "Starting llama-server on %s:%s ..."},
	msgMcpCommandLine:       {"MCP-сервер: SEARCH_ENABLED=true mcp-search-server --addr %s", "MCP server: SEARCH_ENABLED=true mcp-search-server --addr %s"},
	msgErrMcpStart:          {"ошибка запуска mcp-search-server: %v", "failed to start mcp-search-server: %v"},
	msgErrServerStart:       {"ошибка запуска сервера: %v", "failed to start the server: %v"},
	msgServerExitErr:        {"сервер завершился с ошибкой: %v", "the server exited with an error: %v"},
	msgServerDone:           {"llama-server завершён.", "llama-server has finished."},
	msgWaitingReady:         {"Ожидание готовности сервера (%s), таймаут %v ...", "Waiting for the server to become ready (%s), timeout %v ..."},
	msgServerReady:          {"llama-server готов. Запуск Claude Code...", "llama-server is ready. Starting Claude Code..."},
	msgWarnProxyExposure:    {"предупреждение: прокси слушает не-loopback адрес %s без аутентификации — любой в сети может использовать ваш платный ключ. Задайте -proxy-token (и тот же токен в Claude Code) или явный -proxy-allow-remote.", "warning: the proxy listens on the non-loopback address %s without authentication — anyone on the network can use your paid key. Set -proxy-token (and the same token in Claude Code) or an explicit -proxy-allow-remote."},
	msgStartingProxy:        {"Запуск прокси на %s (llama → %s, openai → %s)", "Starting the proxy on %s (llama → %s, openai → %s)"},
	msgProxyLogFile:         {"Логи прокси: %s", "Proxy logs: %s"},
	msgWarnProxyLogDisabled: {"предупреждение: не удалось открыть лог-файл для [proxy] — логи прокси отключены, чтобы не портить TUI Claude Code", "warning: could not open the [proxy] log file — proxy logging is disabled to avoid corrupting the Claude Code TUI"},
	msgErrProxyStart:        {"ошибка запуска прокси: %v", "failed to start the proxy: %v"},
	msgProxyNotUp:           {"прокси не поднялся: %v", "the proxy did not come up: %v"},
	msgWaitingMcpReady:      {"Ожидание готовности MCP-сервера (%s) ...", "Waiting for the MCP server to become ready (%s) ..."},
	msgWarnMcpNotReady:      {"предупреждение: mcp-search-server не готов: %v\nClaude Code будет запущен без web_search.", "warning: mcp-search-server is not ready: %v\nClaude Code will start without web_search."},
	msgWarnMcpConfigWrite:   {"предупреждение: не удалось создать MCP-конфиг: %v", "warning: failed to create the MCP config: %v"},
	msgMcpConfigLine:        {"MCP-конфиг: %s (url=%s, type=http)", "MCP config: %s (url=%s, type=http)"},
	msgErrMcpAddrInvalid:    {"некорректный -mcp-addr %q: %v", "invalid -mcp-addr %q: %v"},
	msgClaudeNotFound:       {"Claude Code не найден (%q): %v", "Claude Code not found (%q): %v"},
	msgClaudeTtyRedirect:    {"Claude Code: stdio перенаправлен в /dev/tty (родительские stdout/stderr — не TTY)", "Claude Code: stdio redirected to /dev/tty (parent stdout/stderr are not a TTY)"},
	msgWarnNoTty:            {"предупреждение: stdout/stderr не TTY, /dev/tty недоступен — Claude Code может не запуститься интерактивно", "warning: stdout/stderr are not a TTY and /dev/tty is unavailable — Claude Code may fail to start interactively"},
	msgClaudeArgsLine:       {"Claude Code args: %v", "Claude Code args: %v"},
	msgClaudeRunLine:        {"Claude Code: %s (cwd=%s)", "Claude Code: %s (cwd=%s)"},
	msgErrClaudeDirAbs:      {"не удалось определить абсолютный путь claude-dir: %v", "failed to determine absolute claude-dir path: %v"},
	msgErrClaudeDirUnavail:  {"claude-dir %q недоступна: %v", "claude-dir %q is unavailable: %v"},
	msgAnthropicBaseURL:     {"ANTHROPIC_BASE_URL=%s", "ANTHROPIC_BASE_URL=%s"},
	msgAnthropicTokenHidden: {"ANTHROPIC_AUTH_TOKEN=<configured via -proxy-token, hidden>", "ANTHROPIC_AUTH_TOKEN=<configured via -proxy-token, hidden>"},
	msgErrClaudeStart:       {"ошибка запуска Claude Code: %v", "failed to start Claude Code: %v"},
	msgClaudeExitCode:       {"Claude Code завершён с кодом %d. Останавливаем llama-server...", "Claude Code exited with code %d. Stopping llama-server..."},
	msgClaudeExitErr:        {"Claude Code завершился с ошибкой: %v", "Claude Code exited with an error: %v"},
	msgClaudeDone:           {"Claude Code завершён. Останавливаем llama-server...", "Claude Code has finished. Stopping llama-server..."},
	msgDone:                 {"Готово.", "Done."},
	msgSignalReceived:       {"\nПолучен сигнал %v — останавливаем дочерние процессы...", "\nReceived signal %v — stopping child processes..."},
	msgSignalRepeatEscalate: {"\nПовторный сигнал %v — эскалация без ожидания", "\nRepeated signal %v — escalating without waiting"},
	msgEscalation:           {"\nЭскалация: %s", "\nEscalation: %s"},

	// --- Update mode ---
	msgUpdateSuccess:       {"llama.cpp успешно обновлён и установлен.", "llama.cpp has been updated and installed successfully."},
	msgErrUpdate:           {"ошибка обновления llama.cpp: %v", "llama.cpp update failed: %v"},
	msgErrSrcAbs:           {"не удалось определить абсолютный путь исходников", "failed to determine absolute sources path"},
	msgErrSrcNotFound:      {"исходники llama.cpp не найдены по пути %q", "llama.cpp sources not found at %q"},
	msgErrSrcNotDir:        {"путь %q не является каталогом", "path %q is not a directory"},
	msgNoUpstreamSkipPull:  {"  (у текущей ветки нет upstream — git pull пропущен, используем текущий код)", "  (current branch has no upstream — git pull skipped, building the current checkout)"},
	msgGitCmd:              {"  git %s", "  git %s"},
	msgErrGitPull:          {"git pull", "git pull"},
	msgErrInstallPrefixAbs: {"не удалось определить абсолютный install-prefix", "failed to determine absolute install-prefix"},
	msgCMakeCmd:            {"  cmake %s", "  cmake %s"},
	msgErrCMakeConfigure:   {"cmake configure", "cmake configure"},
	msgBuilding:            {"Сборка (%d потоков)...", "Building (%d threads)..."},
	msgErrCMakeBuild:       {"cmake build", "cmake build"},
	msgInstalling:          {"Установка в %s...", "Installing into %s..."},
	msgErrCMakeInstall:     {"cmake install", "cmake install"},

	// --- Doctor report ---
	msgDoctorVersion:      {"  версия:         %s", "  version:        %s"},
	msgDoctorConfig:       {"  конфиг:         %s", "  config:         %s"},
	msgDoctorConfigMiss:   {"  конфиг:         %s (не найден — встроенные дефолты)", "  config:         %s (not found — built-in defaults)"},
	msgDoctorProfile:      {"  профиль:        %s", "  profile:        %s"},
	msgErrDoctorConfig:    {"  ошибка конфига: %v", "  config error: %v"},
	msgLblModel:           {"модель:", "model:"},
	msgLblServerBin:       {"llama-server:", "llama-server:"},
	msgLblMmproj:          {"mmproj:", "mmproj:"},
	msgLblSpecDraft:       {"spec-draft:", "spec-draft:"},
	msgLblChatTemplate:    {"chat-template:", "chat-template:"},
	msgLblClaude:          {"claude:", "claude:"},
	msgLblOpenaiKey:       {"OpenAI-ключ:", "OpenAI key:"},
	msgOpenaiKeyLine:      {"  %-15s %s", "  %-15s %s"},
	msgNotSet:             {"(не задан)", "(not set)"},
	msgDoctorNotSet:       {"  %-15s (не задан)", "  %-15s (not set)"},
	msgDoctorPathErr:      {"  %-15s %s (ошибка пути: %v)", "  %-15s %s (path error: %v)"},
	msgDoctorNotFound:     {"  %-15s %s (НЕ найден: %v)", "  %-15s %s (NOT FOUND: %v)"},
	msgDoctorIsDir:        {"  %-15s %s (ЭТО КАТАЛОГ)", "  %-15s %s (IS A DIRECTORY)"},
	msgDoctorOk:           {"  %-15s %s (OK)", "  %-15s %s (OK)"},
	msgDoctorClaudeNoPath: {"  %-15s %s (НЕ найден в PATH)", "  %-15s %s (NOT found in PATH)"},
	msgKeySrcEnvRunner:    {"задан (env LLAMA_RUNNER_OPENAI_KEY)", "set (env LLAMA_RUNNER_OPENAI_KEY)"},
	msgKeySrcEnvOpenai:    {"задан (env OPENAI_API_KEY)", "set (env OPENAI_API_KEY)"},
	msgKeySrcSet:          {"задан", "set"},
	msgDoctorServerCmd:    {"\nКоманда llama-server:\n  %s %s", "\nllama-server command:\n  %s %s"},

	// --- Proxy ---
	msgBodyTooLarge:     {"тело запроса больше %d байт", "request body larger than %d bytes"},
	msgInvalidJSON:      {"invalid JSON: %v", "invalid JSON: %v"},
	msgConvertErr:       {"convert: %v", "convert: %v"},
	msgLlamaUpstream:    {"llama-server upstream: %s", "llama-server upstream: %s"},
	msgOpenaiUpstream:   {"openai upstream: %s", "openai upstream: %s"},
	msgUpstreamErr:      {"llama-server: %v", "llama-server: %v"},
	msgUpstreamOpenai:   {"openai: %v", "openai: %v"},
	msgStreamingUnsupp:  {"streaming unsupported", "streaming unsupported"},
	msgStreamNoDone:     {"upstream stream ended before [DONE]", "upstream stream ended before [DONE]"},
	msgStreamInterrupte: {"upstream stream interrupted: %v", "upstream stream interrupted: %v"},
	msgProxyAuthReject:  {"auth rejected: %s %s (token mismatch or missing)", "auth rejected: %s %s (token mismatch or missing)"},
	msgProxyUpstream:    {"→ апстрим: %s", "→ upstream: %s"},
	msgProxyUsage:       {"← usage: input_tokens=%d, output_tokens=%d", "← usage: input_tokens=%d, output_tokens=%d"},
	msgProxyDropped:     {"отброшено блоков: %s (thinking не имеет аналога в OpenAI, image требует -mmproj)", "dropped blocks: %s (thinking has no OpenAI equivalent, image requires -mmproj)"},
	msgSummaryUnknown:   {"%d неизвестных", "%d unknown"},
	msgSummaryMessages:  {"%d сообщений: system=%v, assistant с tool_calls=%d, tool-ответов=%d, схем инструментов=%d", "%d messages: system=%v, assistant with tool_calls=%d, tool replies=%d, tool schemas=%d"},
}
