package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// runDoctor implements -doctor: it prints a read-only diagnostic report —
// version, effective config file and profile, resolved existence-checked
// paths for the model, llama-server, mmproj, draft model and chat template,
// the claude executable (PATH lookup or absolute path), and where the OpenAI
// key came from — and finally the exact llama-server command line that would
// be executed, with paths normalised the same way run() normalises them
// (expandHome + filepath.Abs), so the report cannot diverge from reality.
//
// No child processes are spawned. Exit codes:
//
//	0 — everything checked out;
//	1 — a path is missing, is a directory, or claude was not found;
//	2 — the configuration itself is invalid (validateConfig failed).
func runDoctor(cfg *Config, home string, runner CommandRunner, stdout, stderr io.Writer) int {
	fmt.Fprintln(stdout, "llama-runner doctor")
	fmt.Fprint(stdout, TF(msgDoctorVersion, version))

	cfgPath := cfg.ConfigPath
	if cfgPath == "" {
		cfgPath = defaultConfigPath(home)
	}
	absCfg := expandHome(cfgPath, home)
	if _, err := os.Stat(absCfg); err == nil {
		fmt.Fprint(stdout, TF(msgDoctorConfig, absCfg))
	} else {
		fmt.Fprint(stdout, TF(msgDoctorConfigMiss, absCfg))
	}
	if cfg.Profile != "" {
		fmt.Fprint(stdout, TF(msgDoctorProfile, cfg.Profile))
	}

	if err := validateConfig(cfg); err != nil {
		fmt.Fprint(stderr, TF(msgErrDoctorConfig, err))
		return 2
	}

	code := 0
	checkPath := func(labelKey, path string) {
		label := T(labelKey)
		if path == "" {
			fmt.Fprint(stdout, TF(msgDoctorNotSet, label))
			return
		}
		abs, err := filepath.Abs(expandHome(path, home))
		if err != nil {
			fmt.Fprint(stdout, TF(msgDoctorPathErr, label, path, err))
			code = 1
			return
		}
		info, err := os.Stat(abs)
		if err != nil {
			fmt.Fprint(stdout, TF(msgDoctorNotFound, label, abs, err))
			code = 1
			return
		}
		if info.IsDir() {
			fmt.Fprint(stdout, TF(msgDoctorIsDir, label, abs))
			code = 1
			return
		}
		fmt.Fprint(stdout, TF(msgDoctorOk, label, abs))
	}

	checkPath(msgLblModel, cfg.Model)
	checkPath(msgLblServerBin, cfg.ServerBin)
	checkPath(msgLblMmproj, cfg.Mmproj)
	checkPath(msgLblSpecDraft, cfg.SpecDraftModel)

	templateRaw := resolveChatTemplate(cfg.ChatTemplate, cfg.Model, cfg.ChatTemplateFallback)
	checkPath(msgLblChatTemplate, templateRaw)

	if !cfg.ServerOnly {
		claudePath := expandHome(cfg.ClaudeBin, home)
		if resolved, err := runner.LookPath(claudePath); err == nil && resolved != "" {
			fmt.Fprint(stdout, TF(msgDoctorOk, T(msgLblClaude), resolved))
		} else if filepath.IsAbs(claudePath) {
			checkPath(msgLblClaude, claudePath)
		} else {
			fmt.Fprint(stdout, TF(msgDoctorClaudeNoPath, T(msgLblClaude), claudePath))
			code = 1
		}
	}

	keySrc := T(msgNotSet)
	if cfg.OpenAIKey != "" {
		switch {
		case os.Getenv("LLAMA_RUNNER_OPENAI_KEY") != "":
			keySrc = T(msgKeySrcEnvRunner)
		case os.Getenv("OPENAI_API_KEY") != "":
			keySrc = T(msgKeySrcEnvOpenai)
		default:
			keySrc = T(msgKeySrcSet)
		}
	}
	fmt.Fprint(stdout, TF(msgOpenaiKeyLine, T(msgLblOpenaiKey), keySrc))

	// Normalize to the same form as actually passed to run():
	// expandHome + filepath.Abs. Otherwise doctor prints ~/... ,
	// but the process gets /home/...
	expandAbs := func(p string) string {
		if p == "" {
			return ""
		}
		expanded := expandHome(p, home)
		if abs, err := filepath.Abs(expanded); err == nil {
			return abs
		}
		return expanded
	}
	serverArgs := buildServerArgs(cfg, ServerPaths{
		Model:    expandAbs(cfg.Model),
		Mmproj:   expandAbs(cfg.Mmproj),
		Draft:    expandAbs(cfg.SpecDraftModel),
		Template: expandAbs(templateRaw),
	})
	fmt.Fprint(stdout, TF(msgDoctorServerCmd, expandAbs(cfg.ServerBin), strings.Join(serverArgs, " ")))

	return code
}
