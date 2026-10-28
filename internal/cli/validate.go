package cli

import (
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// ValidateConfig checks cross-field consistency of a resolved config.Config before
// any process is started. It returns the first problem found:
//
//   - -ready-timeout must be strictly positive;
//   - -host must be non-blank;
//   - -port must be a number in 1..65535;
//   - -mcp-addr must be non-blank and, when the MCP proxy is enabled, a
//     valid host:port pair;
//   - -reasoning, when set, must be one of on, off, auto;
//   - -reasoning-budget, when set, must be an integer >= -1
//     (-1 = unlimited, 0 = disabled, N = token budget);
//   - the external OpenAI provider is all-or-nothing: supplying any of
//     -openai, -key or -model-openai requires all three, -openai must be a
//     parseable absolute URL, and -proxy-addr (if set) must be a valid
//     host:port pair.
func ValidateConfig(cfg *config.Config) error {
	if cfg.ReadyTimeout <= 0 {
		return i18n.Errorv(i18n.MsgErrReadyTimeoutPositive)
	}
	if strings.TrimSpace(cfg.Host) == "" {
		return i18n.Errorv(i18n.MsgErrHostEmpty)
	}
	portNum, err := strconv.Atoi(cfg.Port)
	if err != nil || portNum < 1 || portNum > 65535 {
		return i18n.Errorv(i18n.MsgErrPortBad, cfg.Port)
	}
	if strings.TrimSpace(cfg.McpAddr) == "" {
		return i18n.Errorv(i18n.MsgErrMcpAddrEmpty)
	}
	if cfg.McpProxy {
		if _, _, err := net.SplitHostPort(cfg.McpAddr); err != nil {
			return i18n.Errorv(i18n.MsgErrMcpAddrHostPort, cfg.McpAddr)
		}
	}

	if cfg.Reasoning != "" {
		switch cfg.Reasoning {
		case "on", "off", "auto":
		default:
			return i18n.Errorv(i18n.MsgErrReasoning, cfg.Reasoning)
		}
	}
	if cfg.ReasoningBudget != "" {
		n, err := strconv.Atoi(cfg.ReasoningBudget)
		if err != nil || n < -1 {
			return i18n.Errorv(i18n.MsgErrReasoningBudget, cfg.ReasoningBudget)
		}
	}
	if cfg.OpenAIBaseURL != "" || cfg.OpenAIKey != "" || cfg.OpenAIModel != "" {
		if cfg.OpenAIBaseURL == "" {
			return i18n.Errorv(i18n.MsgErrOpenaiRequired)
		}
		if cfg.OpenAIKey == "" {
			return i18n.Errorv(i18n.MsgErrKeyRequired)
		}
		if cfg.OpenAIModel == "" {
			return i18n.Errorv(i18n.MsgErrModelOpenaiRequired)
		}
		if _, err := url.ParseRequestURI(cfg.OpenAIBaseURL); err != nil {
			return i18n.Errorv(i18n.MsgErrOpenaiURLBad, cfg.OpenAIBaseURL, err)
		}
		if cfg.ProxyAddr != "" {
			if _, _, err := net.SplitHostPort(cfg.ProxyAddr); err != nil {
				return i18n.Errorv(i18n.MsgErrProxyAddrBad, cfg.ProxyAddr)
			}
		}
	}
	return nil
}

// ResolveChatTemplate turns the raw -chat-template value into the path (or
// "") actually used.
//
// Any value other than the literal "auto" — an explicit path or an empty
// string meaning "no external template" — is returned unchanged. For "auto"
// the model file name is inspected: if its base name contains "qwen"
// (case-insensitively), the fallback path is selected; otherwise "" is
// returned, letting llama-server use the template embedded in the GGUF.
//
// fallback is the auto-mode target path, configurable via
// config.Config.ChatTemplateFallback (previously hardcoded), so it is supplied by
// the caller rather than being a constant here.
func ResolveChatTemplate(raw, modelPath, fallback string) string {
	if raw != "auto" {
		return raw
	}
	if strings.Contains(strings.ToLower(filepath.Base(modelPath)), "qwen") {
		return fallback
	}
	return ""
}

// ValidateTemplatePath expands raw (which may start with "~") to an absolute
// path and checks that it refers to a regular file, returning that absolute
// path.
//
// An empty raw is always valid and yields ("", nil). When the file is
// missing (or is not a regular file), behaviour depends on explicit:
// a template the user named via -chat-template/chatTemplate is a hard error,
// while an auto-detected one degrades quietly to ("", nil) so llama-server
// falls back to the template embedded in the GGUF.
func ValidateTemplatePath(raw, home string, explicit bool) (string, error) {
	if raw == "" {
		return "", nil
	}
	p, err := filepath.Abs(config.ExpandHome(raw, home))
	if err != nil {
		return "", i18n.Errorf(i18n.MsgErrTemplateAbs, err)
	}
	info, statErr := os.Stat(p)
	if statErr != nil || !info.Mode().IsRegular() {
		if explicit {
			return "", i18n.Errorv(i18n.MsgErrTemplateMissing, p, statErr)
		}
		return "", nil
	}
	return p, nil
}
