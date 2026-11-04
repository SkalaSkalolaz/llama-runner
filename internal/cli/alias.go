package cli

import (
	"path/filepath"
	"strings"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// This module owns one responsibility: deciding the model NAME that the rest
// of the launcher publishes — `--alias` for llama-server (and therefore the
// card in its WebUI) and ANTHROPIC_DEFAULT_*_MODEL for Claude Code (and
// therefore the entry Claude Code shows in /model). It used to be a single
// line in run() ("if nothing set the alias, call it local"), which let a
// stale alias from the config file survive a deliberate `-model` override:
// the requested GGUF was loaded, but every name the user saw still described
// a different model.

// DefaultAlias is the name used when no alias was configured anywhere and no
// model was chosen on the command line. Kept as "local" for backward
// compatibility with the pre-alias-defaulting behaviour.
const DefaultAlias = "local"

// ggufExtension is the model-file suffix stripped when a name is derived from
// a GGUF path. Matched case-insensitively; it is pure ASCII, so removing it
// by byte count is safe for UTF-8 file names.
const ggufExtension = ".gguf"

// DeriveAliasFromModel turns a GGUF path into the display name for the model
// it points at: the base file name without the .gguf suffix.
//
// The quantization tag is deliberately KEPT (gpt-oss-20b-Q5_K_M, not
// gpt-oss-20b): Q5_K_M, IQ3_XXS, UD-Q4_K_XL and friends really are different
// models by quality and speed, and any heuristic for cutting them off
// eventually misfires on names such as ...-00001-of-00002.
//
// Path separators and a leading "~" are irrelevant here — only the last path
// component is used, so the caller may pass the raw config value (no
// ExpandHome needed) or an already-expanded absolute path; both give the same
// result. Empty input, and paths whose last component is ".", ".." or a bare
// ".gguf", yield "": the caller decides what a derivation failure means.
func DeriveAliasFromModel(modelPath string) string {
	base := filepath.Base(strings.TrimRight(modelPath, "/"))
	if base == "." || base == ".." {
		return ""
	}
	trimmed := base
	if strings.HasSuffix(strings.ToLower(trimmed), ggufExtension) {
		trimmed = trimmed[:len(trimmed)-len(ggufExtension)]
	}
	// A name consisting only of the extension (".gguf") has no stem.
	return trimmed
}

// ResolveAlias computes the effective model alias for cfg and the origin of
// that choice. Precedence (see the config file/flag layers in ParseFlagsWithHome
// and config.Config's *Set fields):
//
//  1. -alias on the command line  → cfg.Alias as given;
//  2. -model on the command line  → name derived from the chosen GGUF, which
//     intentionally overrides a file/profile
//     alias: the user picked the model, the
//     old name no longer describes it;
//  3. alias from the config file or the selected profile → cfg.Alias;
//  4. otherwise                    → DefaultAlias ("local").
//
// Rule 2 does NOT fire for a model coming from a profile: a profile is
// self-contained (its own model and alias), and -p coder must keep showing
// "qwen3.8-coder", not a name re-derived from the file path.
//
// The second return value is an i18n key naming the origin, so -doctor can
// state where the effective alias came from; it is a key, not localised text,
// because ResolveAlias runs before the user-visible report is assembled.
// Rule 2 also falls through to the next rule when the model path has no usable
// stem (for example an empty -model), so the alias never becomes empty.
func ResolveAlias(cfg *config.Config) (string, string) {
	if cfg.AliasFromCLI && cfg.Alias != "" {
		return cfg.Alias, i18n.MsgAliasSrcCLI
	}
	if cfg.ModelSet {
		derived := DeriveAliasFromModel(cfg.Model)
		if derived != "" {
			return derived, i18n.MsgAliasSrcDerived
		}
	}
	if cfg.AliasSet && cfg.Alias != "" {
		if cfg.AliasFromProfile {
			return cfg.Alias, i18n.MsgAliasSrcProfile
		}
		return cfg.Alias, i18n.MsgAliasSrcFile
	}
	return DefaultAlias, i18n.MsgAliasSrcDefault
}
