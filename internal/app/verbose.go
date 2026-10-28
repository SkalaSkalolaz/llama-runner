package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// PrintClaudeDiagnostics prints the -v block describing the Claude Code launch:
// the executable and its working directory, the base URL Claude Code will talk
// to, and the ANTHROPIC_* model variables actually present in the child's
// environment.
//
// The model variables are read back from claudeEnv rather than derived from
// cfg.Alias, because in hybrid mode (external provider configured) all three
// ANTHROPIC_DEFAULT_* point at the external model while the local alias stays
// only in ANTHROPIC_CUSTOM_MODEL_OPTION — printing cfg.Alias there would make
// -v lie about what the child process really sees. The proxy token is never
// echoed, only its presence.
func PrintClaudeDiagnostics(stdout io.Writer, cfg *config.Config, claudePath, workDir, baseURL string, claudeEnv []string) {
	fmt.Fprint(stdout, i18n.TF(i18n.MsgClaudeRunLine, claudePath, workDir))
	fmt.Fprint(stdout, "  ", i18n.TF(i18n.MsgAnthropicBaseURL, baseURL))
	if cfg.ProxyToken != "" {
		fmt.Fprint(stdout, "  ", i18n.TF(i18n.MsgAnthropicTokenHidden))
	}
	for _, want := range []string{
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_CUSTOM_MODEL_OPTION",
	} {
		for _, kv := range claudeEnv {
			if strings.HasPrefix(kv, want+"=") {
				fmt.Fprintf(stdout, "  %s\n", kv)
				break
			}
		}
	}
	fmt.Fprintln(stdout)
}
