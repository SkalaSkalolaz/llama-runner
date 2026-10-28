package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/SkalaSkalolaz/llama-runner/internal/cli"
	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// ResolvedPaths holds the absolute paths validated for launch, ready to be
// handed to llama-server: the server binary and model are always set on the
// success path; Mmproj, Draft and Template are "" when not configured or,
// for Template only, when an auto-detected template turned out to be
// unavailable.
type ResolvedPaths struct {
	ServerBin string
	Model     string
	Mmproj    string
	Draft     string
	Template  string
}

// ResolvePaths expands every configured path ("~" → home → absolute) and
// checks it, printing user-facing errors to stderr. The second return value
// is an exit code for run: 0 means every check passed and the resolved
// paths are safe to launch with.
//
// Severity rules:
//
//	2 — cannot build an absolute path, or an explicitly requested
//	    chat-template is unavailable (hard failure);
//	1 — a required or configured file does not exist (model, server binary,
//	    mmproj, draft model);
//	0 — all good; an auto-detected template that is missing only produces a
//	    warning and falls back to the template embedded in the GGUF.
func ResolvePaths(cfg *config.Config, home string, stdout, stderr io.Writer) (ResolvedPaths, int) {
	var p ResolvedPaths

	serverPath, err := filepath.Abs(config.ExpandHome(cfg.ServerBin, home))
	if err != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgErrResolved, i18n.Errorv(i18n.MsgErrBinAbs, err)))
		return p, 2
	}
	p.ServerBin = serverPath

	modelPath, err := filepath.Abs(config.ExpandHome(cfg.Model, home))
	if err != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgErrResolved, i18n.Errorv(i18n.MsgErrModelAbs, err)))
		return p, 2
	}
	p.Model = modelPath

	if _, err := os.Stat(p.ServerBin); err != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgBinNotFound, p.ServerBin, err))
		return p, 1
	}
	if _, err := os.Stat(p.Model); err != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgModelNotFound, p.Model, err))
		return p, 1
	}

	templateRaw := cli.ResolveChatTemplate(cfg.ChatTemplate, p.Model, cfg.ChatTemplateFallback)
	if cfg.Verbose && cfg.ChatTemplate == "auto" && templateRaw != "" {
		fmt.Fprint(stdout, i18n.TF(i18n.MsgTemplateAutoDetected, templateRaw))
	}
	templatePath, terr := cli.ValidateTemplatePath(templateRaw, home, cfg.ChatTemplateSet)
	if terr != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgErrResolved, terr))
		return p, 2
	}
	if templateRaw != "" && templatePath == "" && !cfg.ChatTemplateSet {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgWarnTemplateAuto, templateRaw))
	}
	p.Template = templatePath

	if cfg.Mmproj != "" {
		mmprojPath, err := filepath.Abs(config.ExpandHome(cfg.Mmproj, home))
		if err != nil {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgErrResolved, i18n.Errorv(i18n.MsgErrMmprojAbs, err)))
			return p, 2
		}
		if _, err := os.Stat(mmprojPath); err != nil {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgMmprojNotFound, mmprojPath, err))
			return p, 1
		}
		p.Mmproj = mmprojPath
	}

	if cfg.SpecDraftModel != "" {
		draftPath, err := filepath.Abs(config.ExpandHome(cfg.SpecDraftModel, home))
		if err != nil {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgErrResolved, i18n.Errorv(i18n.MsgErrDraftAbs, err)))
			return p, 2
		}
		if _, err := os.Stat(draftPath); err != nil {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgDraftNotFound, draftPath, err))
			return p, 1
		}
		p.Draft = draftPath
	}

	return p, 0
}
