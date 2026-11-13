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
	MsgErrAutoFlagConflict     = "err_auto_flag_conflict"
	MsgErrModeBad              = "err_mode_bad"
	MsgErrTaskNeedsAuto        = "err_task_needs_auto"
	MsgErrPlanOnlyNoTask       = "err_plan_only_no_task"
	MsgErrPlanOnlyServer       = "err_plan_only_server"
	MsgErrPlanOnlyUpdate       = "err_plan_only_update"
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
	MsgFlagDownload        = "flag_download"
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
	MsgFlagOpenaiPath      = "flag_openai_path"
	MsgFlagProxyAddr       = "flag_proxy_addr"
	MsgFlagProxyToken      = "flag_proxy_token"
	MsgFlagProxyAllowRem   = "flag_proxy_allow_remote"
	MsgFlagConfig          = "flag_config"
	MsgFlagP               = "flag_p"
	MsgFlagDoctor          = "flag_doctor"
	MsgFlagTask            = "flag_task"
	MsgFlagPlanOnly        = "flag_plan_only"

	// --- Launch flow (run.go) ---
	MsgWarnOpenaiIgnored       = "warn_openai_ignored"
	MsgErrBinAbs               = "err_bin_abs"
	MsgErrModelAbs             = "err_model_abs"
	MsgErrMmprojAbs            = "err_mmproj_abs"
	MsgErrDraftAbs             = "err_draft_abs"
	MsgBinNotFound             = "bin_not_found"
	MsgModelNotFound           = "model_not_found"
	MsgMmprojNotFound          = "mmproj_not_found"
	MsgDraftNotFound           = "draft_not_found"
	MsgTemplateAutoDetected    = "template_auto_detected"
	MsgErrHomeDirShort         = "err_home_dir_short"
	MsgErrResolved             = "err_resolved"
	MsgWarnTemplateAuto        = "warn_template_auto"
	MsgLdLibPathAlready        = "ld_lib_path_already"
	MsgLdLibPath               = "ld_lib_path"
	MsgErrWorkDirAbs           = "err_work_dir_abs"
	MsgErrWorkDirUnavail       = "err_work_dir_unavail"
	MsgErrLogDirCreate         = "err_log_dir_create"
	MsgErrLogOpenRotating      = "err_log_open_rotating"
	MsgErrLogOpen              = "err_log_open"
	MsgErrLogChmod             = "err_log_chmod"
	MsgWorkDirLine             = "work_dir_line"
	MsgCommandLine             = "command_line"
	MsgServerLogFile           = "server_log_file"
	MsgStartingServer          = "starting_server"
	MsgMcpCommandLine          = "mcp_command_line"
	MsgErrMcpStart             = "err_mcp_start"
	MsgErrServerStart          = "err_server_start"
	MsgServerExitErr           = "server_exit_error"
	MsgServerDone              = "server_done"
	MsgWaitingReady            = "waiting_ready"
	MsgServerReady             = "server_ready"
	MsgWarnProxyExposure       = "warn_proxy_exposure"
	MsgStartingProxy           = "starting_proxy"
	MsgStartingProxyLocal      = "starting_proxy_local"
	MsgProxyLogFile            = "proxy_log_file"
	MsgWarnProxyLogDisabled    = "warn_proxy_log_disabled"
	MsgErrProxyStart           = "err_proxy_start"
	MsgProxyNotUp              = "proxy_not_up"
	MsgWaitingMcpReady         = "waiting_mcp_ready"
	MsgWarnMcpNotReady         = "warn_mcp_not_ready"
	MsgWarnMcpConfigWrite      = "warn_mcp_config_write"
	MsgMcpConfigLine           = "mcp_config_line"
	MsgErrMcpAddrInvalid       = "err_mcp_addr_invalid"
	MsgClaudeNotFound          = "claude_not_found"
	MsgClaudeTtyRedirect       = "claude_tty_redirect"
	MsgWarnNoTty               = "warn_no_tty"
	MsgClaudeArgsLine          = "claude_args_line"
	MsgClaudeRunLine           = "claude_run_line"
	MsgErrClaudeDirAbs         = "err_claude_dir_abs"
	MsgErrClaudeDirUnavail     = "err_claude_dir_unavail"
	MsgAnthropicBaseURL        = "anthropic_base_url"
	MsgAnthropicTokenHidden    = "anthropic_token_hidden"
	MsgErrClaudeStart          = "err_claude_start"
	MsgClaudeExitCode          = "claude_exit_code"
	MsgClaudeExitErr           = "claude_exit_error"
	MsgClaudeDone              = "claude_done"
	MsgDone                    = "done"
	MsgSignalReceived          = "signal_received"
	MsgSignalRepeatEscalate    = "signal_repeat_escalate"
	MsgEscalation              = "escalation"
	MsgWarnProfileInvalid      = "warn_profile_invalid"
	MsgWarnProfileUnlaunchable = "warn_profile_unlaunchable"
	MsgWarnProfileDupAlias     = "warn_profile_dup_alias"
	MsgSwapFailed              = "proxy_swap_failed"
	MsgAutoModeLine            = "auto_mode_line"
	MsgAutoServerOnly          = "auto_server_only"
	MsgPlanBuilding            = "plan_building"
	MsgPlanRepaired            = "plan_repaired"
	MsgPlanFallback            = "plan_fallback"
	MsgPlanHeader              = "plan_header"
	MsgPlanSummary             = "plan_summary"
	MsgPlanModels              = "plan_models"
	MsgPlanStepLine            = "plan_step_line"
	MsgPlanGoal                = "plan_goal"
	MsgPlanArtifact            = "plan_artifact"
	MsgPlanVerify              = "plan_verify"
	MsgPlanDepends             = "plan_depends"
	MsgPlanModelCount          = "plan_model_count"
	MsgErrPlanBuild            = "err_plan_build"
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

	// --- Auto-download (-download, internal/download) ---
	MsgErrDownloadSpec           = "err_download_spec"
	MsgErrDownloadNoURL          = "err_download_no_url"
	MsgErrDownloadNoDir          = "err_download_no_dir"
	MsgErrDownloadUpdateConflict = "err_download_update_conflict"
	MsgWarnDownloadOverrides     = "warn_download_overrides"
	MsgWarnDownloadShardNotFirst = "warn_download_shard_not_first"
	MsgDownloadStarting          = "download_starting"
	MsgDownloadShards            = "download_shards"
	MsgDownloadProgress          = "download_progress"
	MsgDownloadFromCache         = "download_from_cache"
	MsgDownloadDone              = "download_done"
	MsgDownloadFailed            = "download_failed"
	MsgDownloadHTTPError         = "download_http_error"
	MsgDownloadGated             = "download_gated"
	MsgDownloadNotFound          = "download_not_found"
	MsgDownloadRetry             = "download_retry"
	MsgDownloadSizeMismatch      = "download_size_mismatch"
	MsgDownloadURLPath           = "download_url_path"
	MsgErrDownloadFailed         = "err_download_failed"
	MsgErrDownloadDisk           = "err_download_disk"
	MsgErrDownloadResumeFailed   = "err_download_resume_failed"

	// --- Doctor report (doctor.go) ---
	MsgDoctorVersion      = "doctor_version"
	MsgDoctorConfig       = "doctor_config"
	MsgDoctorConfigMiss   = "doctor_config_missing"
	MsgDoctorProfile      = "doctor_profile"
	MsgDoctorModeAuto     = "doctor_mode_auto"
	MsgDoctorAutoCtx      = "doctor_auto_ctx"
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
	MsgKeySrcNone         = "key_src_none"
	MsgDoctorServerCmd    = "doctor_server_cmd"
	MsgDoctorAlias        = "doctor_alias"
	MsgAliasSrcCLI        = "alias_src_cli"
	MsgAliasSrcDerived    = "alias_src_derived"
	MsgAliasSrcProfile    = "alias_src_profile"
	MsgAliasSrcFile       = "alias_src_file"
	MsgAliasSrcDefault    = "alias_src_default"

	// --- Proxy (proxy.go, convert.go) ---
	MsgBodyTooLarge      = "proxy_body_too_large"
	MsgInvalidJSON       = "proxy_invalid_json"
	MsgConvertErr        = "proxy_convert_error"
	MsgOpenaiUpstream    = "proxy_openai_upstream"
	MsgUpstreamErr       = "proxy_upstream_connect_error"
	MsgUpstreamOpenai    = "proxy_upstream_openai_error"
	MsgStreamingUnsupp   = "proxy_streaming_unsupported"
	MsgStreamNoDone      = "proxy_stream_no_done"
	MsgStreamInterrupte  = "proxy_stream_interrupted"
	MsgProxyAuthReject   = "proxy_auth_reject"
	MsgProxyUpstream     = "proxy_upstream_line"
	MsgProxyUsage        = "proxy_usage_line"
	MsgProxyDropped      = "proxy_dropped_blocks"
	MsgSummaryUnknown    = "summary_unknown"
	MsgSummaryMessages   = "summary_messages"
	MsgErrUnknownProfile = "err_unknown_profile"
	MsgSwapVerbose       = "swap_verbose"
	MsgAutoSwitchIgnored = "auto_switch_ignored"

	// --- Auto mode, interactive loop (internal/app/interactive.go) ---
	MsgServerReadyAuto       = "server_ready_auto"
	MsgInteractivePrompt     = "interactive_prompt"
	MsgInteractiveBye        = "interactive_bye"
	MsgInteractiveTaskFailed = "interactive_task_failed"

	// --- Planner (internal/planner) ---
	MsgErrPlanNoJSON    = "err_plan_no_json"
	MsgErrPlanJSON      = "err_plan_json"
	MsgErrPlanEmpty     = "err_plan_empty"
	MsgErrPlanTooMany   = "err_plan_too_many"
	MsgErrPlanStepID    = "err_plan_step_id"
	MsgErrPlanDupID     = "err_plan_dup_id"
	MsgErrPlanStepField = "err_plan_step_field"
	MsgErrPlanAlias     = "err_plan_alias"
	MsgErrPlanDep       = "err_plan_dep"
	MsgErrPlanSelfDep   = "err_plan_self_dep"
	MsgErrPlanCycle     = "err_plan_cycle"
	MsgErrPlannerHTTP   = "err_planner_http"
	MsgErrPlannerEmpty  = "err_planner_empty"
	// --- Auto executor (internal/orchestrator, app/execute.go) ---
	MsgExecStepStart      = "exec_step_start"
	MsgExecSwap           = "exec_swap"
	MsgExecWave           = "exec_wave"
	MsgErrExecSwap        = "err_exec_swap"
	MsgExecStepOK         = "exec_step_ok"
	MsgExecStepFail       = "exec_step_fail"
	MsgErrExecStepTimeout = "err_exec_step_timeout"
	MsgErrExecArtifact    = "err_exec_artifact"
	MsgErrExecVerify      = "err_exec_verify"
	MsgErrExecPlan        = "err_exec_plan"
	MsgExecDone           = "exec_done"
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
	MsgErrHomeDir:            {"не удалось получить домашний каталог", "failed to get home directory"},
	MsgErrConfigFileNotFound: {"конфиг-файл %q не найден", "config file %q not found"},
	MsgErrProfileNoConfig:    {"профиль %q не найден: конфиг-файл отсутствует", "profile %q not found: config file is missing"},
	MsgErrProfileNotFound:    {"профиль %q не найден в конфиге", "profile %q not found in config"},
	MsgErrAutoFlagConflict: {
		"режим auto: флаги %s запрещены — профиль планировщика самодостаточен; уберите флаги или измените профиль в конфиге",
		"auto mode: flags %s are not allowed — the planner profile is self-contained; drop the flags or edit the profile in the config file",
	},
	MsgErrModeBad: {
		"некорректный mode %q: ожидается \"auto\" или пустое значение",
		"invalid mode %q: expected \"auto\" or an empty value",
	},
	MsgErrTaskNeedsAuto: {
		"-task и -plan-only доступны только в профиле с \"mode\": \"auto\"",
		"-task and -plan-only are available only in a profile declaring \"mode\": \"auto\"",
	},
	MsgErrPlanOnlyNoTask: {
		"-plan-only требует -task: нечего планировать",
		"-plan-only requires -task: there is nothing to plan",
	},
	MsgErrPlanOnlyServer: {
		"-plan-only несовместим с -server: планировщику нужен режим Claude Code",
		"-plan-only is not compatible with -server: the planner needs the Claude Code mode",
	},
	MsgErrPlanOnlyUpdate: {
		"-plan-only несовместим с -update",
		"-plan-only is not compatible with -update",
	},
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

	MsgWarnProfileInvalid:      {"предупреждение: профиль %q отброшен: %v", "warning: profile %q excluded: %v"},
	MsgWarnProfileUnlaunchable: {"предупреждение: профиль %q не запускается; исключён из меню /model", "warning: profile %q cannot launch; excluded from /model menu"},
	MsgWarnProfileDupAlias:     {"предупреждение: профиль %q пропущен — алиас %q уже занят", "warning: profile %q skipped — alias %q already taken"},
	MsgSwapFailed:              {"переключение на %q не удалось: %v", "swap to %q failed: %v"},
	MsgAutoModeLine: {
		"Режим: auto — планировщик %s, профилей в каталоге: %d",
		"Mode: auto — planner %s, profiles in catalog: %d",
	},
	MsgAutoServerOnly: {
		"режим auto: -server — запускается только планировщик, без оркестрации",
		"auto mode: -server — starting the planner only, no orchestration",
	},
	MsgPlanBuilding: {"Планировщик %s строит план ...", "Planner %s is building a plan ..."},
	MsgPlanRepaired: {"план принят после исправления", "plan accepted after a repair round"},
	MsgPlanFallback: {
		"предупреждение: валидный план не получен (%v) — показан одношаговый запасной план",
		"warning: no valid plan was produced (%v) — showing a single-step fallback plan",
	},
	MsgPlanHeader:     {"План (%d шагов):", "Plan (%d steps):"},
	MsgPlanSummary:    {"сводка:", "summary:"},
	MsgPlanModels:     {"модели:", "models:"},
	MsgPlanStepLine:   {"%d/%d [%s] %s · %s", "%d/%d [%s] %s · %s"},
	MsgPlanGoal:       {"цель:", "goal:"},
	MsgPlanArtifact:   {"артефакт:", "artifact:"},
	MsgPlanVerify:     {"проверка:", "verify:"},
	MsgPlanDepends:    {"зависит:", "depends:"},
	MsgPlanModelCount: {"%s × %d", "%s × %d"},
	MsgErrPlanBuild:   {"не удалось построить план: %v", "failed to build the plan: %v"},

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
	MsgFlagDownload:        {"загрузить модель: <директория><URL> (например ~/GGUF/https://huggingface.co/…)", "download a model: <dir><URL> (e.g. ~/GGUF/https://huggingface.co/…)"},
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
	MsgFlagKey:             {"API-ключ для внешнего провайдера (\"none\" или \"\" — работать без ключа)", "API key for the external provider (\"none\" or \"\" = no authentication)"},
	MsgFlagModelOpenai:     {"Название модели у внешнего провайдера", "model name at the external provider"},
	MsgFlagOpenaiPath:      {"путь к chat/completions у внешнего провайдера (\"\" — без суффикса, для OVH Kepler / VLM.run)", "path to chat/completions at the external provider (\"\" — no suffix, for OVH Kepler / VLM.run)"},
	MsgFlagProxyAddr:       {"адрес локального прокси (гибридный режим)", "local proxy address (hybrid mode)"},
	MsgFlagProxyToken:      {"токен для аутентификации входящих запросов прокси (пусто — без проверки)", "token for incoming proxy requests (empty = no check)"},
	MsgFlagProxyAllowRem:   {"разрешить не-loopback -proxy-addr без предупреждения", "allow non-loopback -proxy-addr without warning"},
	MsgFlagConfig:          {"путь к конфиг-файлу (\"\" — не читать)", "config file path (\"\" = don't read)"},
	MsgFlagP:               {"профиль из конфиг-файла", "profile from config file"},
	MsgFlagDoctor:          {"показать разрешённые пути и версию, не запуская процессы", "show resolved paths and version without starting processes"},
	MsgFlagTask:            {"задача для планировщика (профиль с \"mode\": \"auto\")", "task for the planner (a profile declaring \"mode\": \"auto\")"},
	MsgFlagPlanOnly:        {"построить план и выйти, без исполнения", "build the plan and exit, without executing it"},

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
	MsgStartingProxyLocal:   {"Запуск прокси на %s (все запросы → %s)", "Starting the proxy on %s (all requests → %s)"},
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

	// --- Auto-download (-download) ---
	MsgErrDownloadSpec: {
		"некорректный -download %q: ожидается <dir><url>",
		"invalid -download %q: expected <dir><url>",
	},
	MsgErrDownloadNoURL:          {"в -download не найден http(s)-URL", "no http(s) URL found in -download"},
	MsgErrDownloadNoDir:          {"в -download не указана целевая директория (префикс до URL)", "no target directory in -download (the prefix before the URL)"},
	MsgErrDownloadUpdateConflict: {"-download несовместим с -update", "-download is not compatible with -update"},
	MsgWarnDownloadOverrides:     {"предупреждение: -model игнорируется, потому что задан -download", "warning: -model is ignored because -download is set"},
	MsgWarnDownloadShardNotFirst: {
		"  передан шард %d — точка входа будет шард 1",
		"  shard %d was given — the entry point will be shard 1",
	},
	MsgDownloadProgress: {
		"  %s: %d%% %s %s / %s  %s/s  ETA %s",
		"  %s: %d%% %s %s / %s  %s/s  ETA %s",
	},
	MsgDownloadStarting: {"Загрузка %s → %s", "Downloading %s → %s"},
	MsgDownloadShards:   {"  это шардированная модель: %d файлов", "  this is a sharded model: %d files"},
	MsgDownloadFromCache: {
		"  уже скачано: %s",
		"  already downloaded: %s",
	},
	MsgDownloadDone:      {"  ✓ %s (%s)", "  ✓ %s (%s)"},
	MsgDownloadFailed:    {"  не удалось скачать %s: %v", "  failed to download %s: %v"},
	MsgDownloadHTTPError: {"  HTTP %d на %s", "  HTTP %d on %s"},
	MsgDownloadGated:     {"  модель требует токена (HF_TOKEN / HUGGING_FACE_HUB_TOKEN): %s", "  the model requires a token (HF_TOKEN / HUGGING_FACE_HUB_TOKEN): %s"},
	MsgDownloadNotFound:  {"  файл не найден на HuggingFace: %s", "  file not found on HuggingFace: %s"},
	MsgDownloadRetry:     {"  повтор через %v (%d/%d): %v", "  retrying in %v (%d/%d): %v"},
	MsgDownloadSizeMismatch: {
		"  размер не совпадает (локально %d, удалённо %d) — перекачиваю",
		"  size mismatch (local %d, remote %d) — re-downloading",
	},
	MsgDownloadURLPath: {"  URL: %s\n  путь: %s", "  URL: %s\n  path: %s"},
	MsgErrDownloadFailed: {
		"не удалось скачать %s",
		"failed to download %s",
	},
	MsgErrDownloadDisk: {
		"не удалось создать директорию %q",
		"failed to create directory %q",
	},
	MsgErrDownloadResumeFailed: {
		"повторите команду — докачка продолжится с .partial-файла",
		"run the command again — the download will resume from the .partial file",
	},

	// --- Doctor report ---
	MsgDoctorVersion:    {"  версия:         %s", "  version:        %s"},
	MsgDoctorConfig:     {"  конфиг:         %s", "  config:         %s"},
	MsgDoctorConfigMiss: {"  конфиг:         %s (не найден — встроенные дефолты)", "  config:         %s (not found — built-in defaults)"},
	MsgDoctorProfile:    {"  профиль:        %s", "  profile:        %s"},
	MsgDoctorModeAuto:   {"  режим:          auto (планировщик)", "  mode:           auto (planner)"},
	MsgDoctorAutoCtx: {
		"  контекст auto:  %d (максимум по профилям с parallel=1)",
		"  auto context:   %d (max over profiles with parallel=1)",
	},
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
	MsgKeySrcNone:         {"не требуется (ключ не передаётся)", "not required (no key sent)"},
	MsgDoctorServerCmd:    {"\nКоманда llama-server:\n  %s %s", "\nllama-server command:\n  %s %s"},
	MsgDoctorAlias:        {"  эффективный алиас:  %s (%s)", "  effective alias:  %s (%s)"},
	MsgAliasSrcCLI:        {"CLI", "CLI"},
	MsgAliasSrcDerived:    {"выведен из модели", "derived from model"},
	MsgAliasSrcProfile:    {"профиль", "profile"},
	MsgAliasSrcFile:       {"конфиг-файл", "config file"},
	MsgAliasSrcDefault:    {"по умолчанию", "default"},

	// --- Proxy ---
	MsgBodyTooLarge:     {"тело запроса больше %d байт", "request body larger than %d bytes"},
	MsgInvalidJSON:      {"некорректный JSON: %v", "invalid JSON: %v"},
	MsgConvertErr:       {"ошибка конвертации: %v", "convert: %v"},
	MsgOpenaiUpstream:   {"ответ openai: %s", "openai upstream: %s"},
	MsgStreamingUnsupp:  {"стриминг не поддерживается", "streaming unsupported"},
	MsgStreamNoDone:     {"поток апстрима завершился без [DONE]", "upstream stream ended before [DONE]"},
	MsgStreamInterrupte: {"поток апстрима прерван: %v", "upstream stream interrupted: %v"},
	MsgProxyAuthReject:  {"аутентификация отклонена: %s %s (токен не совпадает или отсутствует)", "auth rejected: %s %s (token mismatch or missing)"},
	MsgProxyUsage:       {"← использование: input_tokens=%d, output_tokens=%d", "← usage: input_tokens=%d, output_tokens=%d"},
	MsgUpstreamErr:      {"llama-server: %v", "llama-server: %v"},
	MsgUpstreamOpenai:   {"openai: %v", "openai: %v"},
	MsgProxyUpstream:    {"→ апстрим: %s", "→ upstream: %s"},
	MsgProxyDropped:     {"отброшено блоков: %s (thinking не имеет аналога в OpenAI, image требует -mmproj)", "dropped blocks: %s (thinking has no OpenAI equivalent, image requires -mmproj)"},
	MsgSummaryUnknown:   {"%d неизвестных", "%d unknown"},
	MsgSummaryMessages:  {"%d сообщений: system=%v, assistant с tool_calls=%d, tool-ответов=%d, схем инструментов=%d", "%d messages: system=%v, assistant with tool_calls=%d, tool replies=%d, tool schemas=%d"},
	MsgErrUnknownProfile: {
		"профиль %q отсутствует в каталоге",
		"profile %q is not in the catalog",
	},
	MsgSwapVerbose: {
		"переключение профиля: %s → %s",
		"switching profile: %s → %s",
	},
	MsgServerReadyAuto: {
		"llama-server готов. Режим auto — введите задачу.",
		"llama-server is ready. Auto mode — type a task.",
	},
	MsgInteractivePrompt: {
		"Задача (или 'exit' для выхода): ",
		"Task (or 'exit' to quit): ",
	},
	MsgInteractiveBye: {
		"Выход.",
		"Bye.",
	},
	MsgInteractiveTaskFailed: {
		"Задача завершилась с кодом %d. Введите новую задачу или 'exit'.",
		"Task exited with code %d. Enter a new task or 'exit'.",
	},
	MsgAutoSwitchIgnored: {
		"auto: ручное переключение на %q проигнорировано — моделью владеет оркестратор",
		"auto: manual switch to %q ignored — the orchestrator owns the model",
	},
	// --- Planner ---
	MsgErrPlanNoJSON:    {"в ответе модели нет JSON", "no JSON found in the model reply"},
	MsgErrPlanJSON:      {"не удалось разобрать JSON плана", "failed to parse the plan JSON"},
	MsgErrPlanEmpty:     {"план не содержит шагов", "the plan contains no steps"},
	MsgErrPlanTooMany:   {"шагов %d больше лимита %d", "%d steps exceed the limit of %d"},
	MsgErrPlanStepID:    {"шаг %d: пустой id", "step %d: empty id"},
	MsgErrPlanDupID:     {"повторяющийся id шага %q", "duplicate step id %q"},
	MsgErrPlanStepField: {"шаг %q: не заполнено поле %s", "step %q: field %s is empty"},
	MsgErrPlanAlias:     {"шаг %q: алиас %q отсутствует в каталоге", "step %q: alias %q is not in the catalog"},
	MsgErrPlanDep:       {"шаг %q: зависимость %q не найдена в плане", "step %q: dependency %q is not in the plan"},
	MsgErrPlanSelfDep:   {"шаг %q зависит сам от себя", "step %q depends on itself"},
	MsgErrPlanCycle:     {"зависимости плана образуют цикл", "the plan dependencies form a cycle"},
	MsgErrPlannerHTTP:   {"planner: HTTP %d: %s", "planner: HTTP %d: %s"},
	MsgErrPlannerEmpty:  {"planner: модель вернула пустой ответ", "planner: the model returned an empty reply"},
	// --- Auto executor ---
	MsgExecStepStart:      {"Шаг %d/%d: %s (%s) — модель %s", "Step %d/%d: %s (%s) — model %s"},
	MsgExecSwap:           {"переключение модели: %s → %s", "switching model: %s → %s"},
	MsgExecWave:           {"волна на модели %s: шагов %d, слотов %d", "wave on model %s: %d steps, %d slots"},
	MsgErrExecSwap:        {"переключение на %q не удалось: %v", "swap to %q failed: %v"},
	MsgExecStepOK:         {"шаг %s выполнен за %s", "step %s done in %s"},
	MsgExecStepFail:       {"шаг %s не выполнен за %s: %v", "step %s failed in %s: %v"},
	MsgErrExecStepTimeout: {"шаг %s не завершился за %v", "step %s did not finish within %v"},
	MsgErrExecArtifact:    {"артефакт %q не найден: %v", "artifact %q not found: %v"},
	MsgErrExecVerify:      {"проверка %q не прошла: %s", "verification %q failed: %s"},
	MsgErrExecPlan:        {"не удалось упорядочить план: %v", "failed to order the plan: %v"},
	MsgExecDone:           {"Исполнение завершено за %s: выполнено %d из %d шагов (в плане %d)", "Execution finished in %s: %d of %d steps done (%d in plan)"},
}
