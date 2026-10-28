package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// ApplyLdLibraryPath points LD_LIBRARY_PATH at the lib/ or lib64/ directory
// shipped next to the llama-server binary, because a cmake install keeps the
// shared libraries in a sibling directory of bin/. Only the first existing
// candidate is used (lib wins), and an already-present directory is not
// duplicated — otherwise -v shows the same path twice and CUDA search paths
// get shadowed. cmd.Env is materialised from the parent environment first,
// because os/exec interprets an empty Env as "no environment at all".
func ApplyLdLibraryPath(cmd *exec.Cmd, serverBin string, stdout io.Writer, verbose bool) {
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	for _, sub := range []string{"lib", "lib64"} {
		cand := filepath.Join(filepath.Dir(serverBin), "..", sub)
		abs, absErr := filepath.Abs(cand)
		if absErr != nil {
			continue
		}
		abs = filepath.Clean(abs)
		if info, statErr := os.Stat(abs); statErr != nil || !info.IsDir() {
			continue
		}
		oldLP := os.Getenv("LD_LIBRARY_PATH")
		if pathContainsSegment(oldLP, abs) {
			// Already in the environment — don't duplicate.
			if verbose {
				fmt.Fprint(stdout, i18n.TF(i18n.MsgLdLibPathAlready, oldLP, abs))
			}
		} else {
			newLP := abs
			if oldLP != "" {
				newLP = abs + ":" + oldLP
			}
			cmd.Env = append(cmd.Env, "LD_LIBRARY_PATH="+newLP)
			if verbose {
				fmt.Fprint(stdout, i18n.TF(i18n.MsgLdLibPath, newLP))
			}
		}
		break
	}
}

// SetWorkDir decides which working directory llama-server starts in and stores
// it on cmd.Dir: an explicit -dir (expanded "~", made absolute, must be an
// existing directory — otherwise this is a hard launch error) or, when unset,
// the launcher's own directory. Returns 0 on success, otherwise the exit code
// run() must return.
func SetWorkDir(cmd *exec.Cmd, cfg *config.Config, home string, stderr io.Writer) int {
	if cfg.WorkDir != "" {
		absDir, dirErr := filepath.Abs(config.ExpandHome(cfg.WorkDir, home))
		if dirErr != nil {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgErrWorkDirAbs, dirErr))
			return 1
		}
		if info, statErr := os.Stat(absDir); statErr != nil || !info.IsDir() {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgErrWorkDirUnavail, absDir, statErr))
			return 1
		}
		cmd.Dir = absDir
	} else if wd, wdErr := os.Getwd(); wdErr == nil {
		cmd.Dir = wd
	}
	return 0
}
