package app

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/download"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// This module owns one responsibility: the -download pre-step of run() —
// turning the composite "<dir><url>" value into a fetched model on disk and
// re-pointing the config at it. Everything transport-related lives in
// internal/download; this file only wires it into the launch flow and the
// exit-code convention.

// hfTokenEnv / hfTokenEnvAlt are the environment variables carrying a
// Hugging Face access token for gated repositories (the names follow the
// huggingface_hub convention). The value is passed to the downloader as
// data and never printed.
const (
	hfTokenEnv    = "HF_TOKEN"
	hfTokenEnvAlt = "HUGGING_FACE_HUB_TOKEN"
)

// downloadModel executes the -download pre-step for run(): parse the
// composite spec, fetch the model (single file or Hugging Face shard set)
// into the target directory, and re-point cfg at the downloaded entry
// point. Progress goes to stdout, errors to stderr.
//
// On success cfg.Model becomes the absolute entry-point path (the first
// shard for a multi-file model — llama.cpp loads the rest itself),
// cfg.ModelSet stays true (the -download flag set it in cli.ParseFlags, so
// the alias derives from the new GGUF and per-model parallel resets
// apply), and cfg.Download is cleared so the fetch cannot trigger twice in
// one process.
//
// Returns 0 on success, otherwise the exit code run() must return:
// 2 — the -download value is malformed, 1 — the fetch failed.
func downloadModel(cfg *config.Config, home string, stdout, stderr io.Writer, version string) int {
	dir, urlStr, err := download.ParseSpec(cfg.Download, home)
	if err != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgErrResolved, err))
		return 2
	}
	if cfg.Verbose && cfg.DownloadOverridesModel {
		fmt.Fprint(stdout, i18n.TF(i18n.MsgWarnDownloadOverrides))
	}
	token := os.Getenv(hfTokenEnv)
	if token == "" {
		token = os.Getenv(hfTokenEnvAlt)
	}
	res, err := download.Fetch(context.Background(), download.Options{
		Dir:      dir,
		URL:      urlStr,
		Progress: stdout,
		Verbose:  cfg.Verbose,
		Version:  version,
		Token:    token,
	})
	if err != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgErrResolved, err))
		return 1
	}
	cfg.Model = res.EntryPoint
	cfg.ModelSet = true
	cfg.Download = ""
	return 0
}
