package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// runDoctor prints diagnostics without spawning child processes.
// Returns exit code: 0 — all ok, 1 — path problems, 2 — invalid config.
func runDoctor(cfg *Config, home string, runner CommandRunner, stdout, stderr io.Writer) int {
	fmt.Fprintf(stdout, "llama-runner doctor\n")
	fmt.Fprintf(stdout, "  версия:         %s\n", version)

	cfgPath := cfg.ConfigPath
	if cfgPath == "" {
		cfgPath = defaultConfigPath(home)
	}
	absCfg := expandHome(cfgPath, home)
	if _, err := os.Stat(absCfg); err == nil {
		fmt.Fprintf(stdout, "  конфиг:         %s\n", absCfg)
	} else {
		fmt.Fprintf(stdout, "  конфиг:         %s (не найден — встроенные дефолты)\n", absCfg)
	}
	if cfg.Profile != "" {
		fmt.Fprintf(stdout, "  профиль:        %s\n", cfg.Profile)
	}

	if err := validateConfig(cfg); err != nil {
		fmt.Fprintf(stderr, "  ошибка конфига: %v\n", err)
		return 2
	}

	code := 0
	checkPath := func(label, path string) {
		if path == "" {
			fmt.Fprintf(stdout, "  %-15s (не задан)\n", label+":")
			return
		}
		abs, err := filepath.Abs(expandHome(path, home))
		if err != nil {
			fmt.Fprintf(stdout, "  %-15s %s (ошибка пути: %v)\n", label+":", path, err)
			code = 1
			return
		}
		info, err := os.Stat(abs)
		if err != nil {
			fmt.Fprintf(stdout, "  %-15s %s (НЕ найден: %v)\n", label+":", abs, err)
			code = 1
			return
		}
		if info.IsDir() {
			fmt.Fprintf(stdout, "  %-15s %s (ЭТО КАТАЛОГ)\n", label+":", abs)
			code = 1
			return
		}
		fmt.Fprintf(stdout, "  %-15s %s (OK)\n", label+":", abs)
	}

	checkPath("модель", cfg.Model)
	checkPath("llama-server", cfg.ServerBin)
	checkPath("mmproj", cfg.Mmproj)
	checkPath("spec-draft", cfg.SpecDraftModel)

	templateRaw := resolveChatTemplate(cfg.ChatTemplate, cfg.Model, cfg.ChatTemplateFallback)
	checkPath("chat-template", templateRaw)

	if !cfg.ServerOnly {
		claudePath := expandHome(cfg.ClaudeBin, home)
		if resolved, err := runner.LookPath(claudePath); err == nil && resolved != "" {
			fmt.Fprintf(stdout, "  %-15s %s (OK)\n", "claude:", resolved)
		} else if filepath.IsAbs(claudePath) {
			checkPath("claude", claudePath)
		} else {
			fmt.Fprintf(stdout, "  %-15s %s (НЕ найден в PATH)\n", "claude:", claudePath)
			code = 1
		}
	}

	keySrc := "не задан"
	if cfg.OpenAIKey != "" {
		switch {
		case os.Getenv("LLAMA_RUNNER_OPENAI_KEY") != "":
			keySrc = "задан (env LLAMA_RUNNER_OPENAI_KEY)"
		case os.Getenv("OPENAI_API_KEY") != "":
			keySrc = "задан (env OPENAI_API_KEY)"
		default:
			keySrc = "задан"
		}
	}
	fmt.Fprintf(stdout, "  %-15s %s\n", "OpenAI-ключ:", keySrc)

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
	fmt.Fprintf(stdout, "\nКоманда llama-server:\n  %s %s\n",
		expandAbs(cfg.ServerBin), strings.Join(serverArgs, " "))

	return code
}
