package doctor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/SkalaSkalolaz/llama-runner/internal/cli"
	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
	"github.com/SkalaSkalolaz/llama-runner/internal/runner"
)

// RunDoctor implements -doctor: it prints a read-only diagnostic report —
// version, effective config file and profile, the effective model alias with
// its origin, resolved existence-checked paths for the model, llama-server,
// mmproj, draft model and chat template,
// the claude executable (PATH lookup or absolute path), and where the OpenAI
// key came from — and finally the exact llama-server command line that would
// be executed, with paths normalised the same way run() normalises them
// (ExpandHome + filepath.Abs), so the report cannot diverge from reality.
//
// No child processes are spawned. Exit codes:
//
//	0 — everything checked out;
//	1 — a path is missing, is a directory, or claude was not found;
//	2 — the configuration itself is invalid (ValidateConfig failed).
func RunDoctor(cfg *config.Config, home string, cmdRunner runner.CommandRunner, version string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stdout, "llama-runner doctor")
	fmt.Fprint(stdout, i18n.TF(i18n.MsgDoctorVersion, version))

	cfgPath := cfg.ConfigPath
	if cfgPath == "" {
		cfgPath = config.DefaultConfigPath(home)
	}
	absCfg := config.ExpandHome(cfgPath, home)
	if _, err := os.Stat(absCfg); err == nil {
		fmt.Fprint(stdout, i18n.TF(i18n.MsgDoctorConfig, absCfg))
	} else {
		fmt.Fprint(stdout, i18n.TF(i18n.MsgDoctorConfigMiss, absCfg))
	}
	if cfg.Profile != "" {
		fmt.Fprint(stdout, i18n.TF(i18n.MsgDoctorProfile, cfg.Profile))
	}

	// The name llama-server will advertise (--alias, and with it the card in
	// its WebUI) and the name Claude Code will show in /model come from one
	// place, so the report states both the value and where it came from.
	// Resolving here rather than trusting cfg.Alias keeps a standalone
	// -doctor (tests, or any future caller that skips Run's resolution)
	// honest: the printed value is the one the printed command line uses.
	alias, aliasSrc := cli.ResolveAlias(cfg)
	fmt.Fprint(stdout, i18n.TF(i18n.MsgDoctorAlias, alias, i18n.T(aliasSrc)))

	if err := cli.ValidateConfig(cfg); err != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgErrDoctorConfig, err))
		return 2
	}

	code := 0
	checkPath := func(labelKey, path string) {
		label := i18n.T(labelKey)
		if path == "" {
			fmt.Fprint(stdout, i18n.TF(i18n.MsgDoctorNotSet, label))
			return
		}
		abs, err := filepath.Abs(config.ExpandHome(path, home))
		if err != nil {
			fmt.Fprint(stdout, i18n.TF(i18n.MsgDoctorPathErr, label, path, err))
			code = 1
			return
		}
		info, err := os.Stat(abs)
		if err != nil {
			fmt.Fprint(stdout, i18n.TF(i18n.MsgDoctorNotFound, label, abs, err))
			code = 1
			return
		}
		if info.IsDir() {
			fmt.Fprint(stdout, i18n.TF(i18n.MsgDoctorIsDir, label, abs))
			code = 1
			return
		}
		fmt.Fprint(stdout, i18n.TF(i18n.MsgDoctorOk, label, abs))
	}

	checkPath(i18n.MsgLblModel, cfg.Model)
	checkPath(i18n.MsgLblServerBin, cfg.ServerBin)
	checkPath(i18n.MsgLblMmproj, cfg.Mmproj)
	checkPath(i18n.MsgLblSpecDraft, cfg.SpecDraftModel)

	templateRaw := cli.ResolveChatTemplate(cfg.ChatTemplate, cfg.Model, cfg.ChatTemplateFallback)
	checkPath(i18n.MsgLblChatTemplate, templateRaw)

	if !cfg.ServerOnly {
		claudePath := config.ExpandHome(cfg.ClaudeBin, home)
		if resolved, err := cmdRunner.LookPath(claudePath); err == nil && resolved != "" {
			fmt.Fprint(stdout, i18n.TF(i18n.MsgDoctorOk, i18n.T(i18n.MsgLblClaude), resolved))
		} else if filepath.IsAbs(claudePath) {
			checkPath(i18n.MsgLblClaude, claudePath)
		} else {
			fmt.Fprint(stdout, i18n.TF(i18n.MsgDoctorClaudeNoPath, i18n.T(i18n.MsgLblClaude), claudePath))
			code = 1
		}
	}

	keySrc := i18n.T(i18n.MsgNotSet)
	if cfg.OpenAIKey != "" {
		switch {
		case os.Getenv("LLAMA_RUNNER_OPENAI_KEY") != "":
			keySrc = i18n.T(i18n.MsgKeySrcEnvRunner)
		case os.Getenv("OPENAI_API_KEY") != "":
			keySrc = i18n.T(i18n.MsgKeySrcEnvOpenai)
		default:
			keySrc = i18n.T(i18n.MsgKeySrcSet)
		}
	}
	fmt.Fprint(stdout, i18n.TF(i18n.MsgOpenaiKeyLine, i18n.T(i18n.MsgLblOpenaiKey), keySrc))

	// Normalize to the same form as actually passed to run():
	// ExpandHome + filepath.Abs. Otherwise doctor prints ~/... ,
	// but the process gets /home/...
	expandAbs := func(p string) string {
		if p == "" {
			return ""
		}
		expanded := config.ExpandHome(p, home)
		if abs, err := filepath.Abs(expanded); err == nil {
			return abs
		}
		return expanded
	}
	// BuildServerArgs reads cfg.Alias, so the report must show the resolved
	// name, not whatever the file happened to contain. The copy keeps
	// doctor read-only: the caller's cfg is left untouched.
	aliasCfg := *cfg
	aliasCfg.Alias = alias
	serverArgs := cli.BuildServerArgs(&aliasCfg, cli.ServerPaths{
		Model:    expandAbs(cfg.Model),
		Mmproj:   expandAbs(cfg.Mmproj),
		Draft:    expandAbs(cfg.SpecDraftModel),
		Template: expandAbs(templateRaw),
	})
	fmt.Fprint(stdout, i18n.TF(i18n.MsgDoctorServerCmd, expandAbs(cfg.ServerBin), strings.Join(serverArgs, " ")))

	return code
}
