package update

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
	"github.com/SkalaSkalolaz/llama-runner/internal/runner"
)

// UpdateLlamaCPP implements -update: it brings the llama.cpp installation up
// to date in four steps, streaming each tool's output to stdout (commands
// themselves are echoed only with verbose):
//
//  1. git pull --ff-only in srcPath — skipped with a notice when the current
//     branch has no upstream, in which case the existing checkout is built;
//  2. cmake configure into <srcPath>/build with CMAKE_BUILD_TYPE=Release and
//     CMAKE_INSTALL_PREFIX=installPrefix; CUDA toggled via GGML_CUDA, with
//     CMAKE_CUDA_ARCHITECTURES pinned to cudaArch or unset to let cmake
//     auto-detect;
//  3. cmake build using all logical CPUs;
//  4. cmake install into installPrefix.
//
// srcPath must be an existing directory (resolved to an absolute path
// first); every step's failure aborts the update with a wrapped error naming
// the failed step. runner is injected so tests can observe the command
// sequence without executing git or cmake.
func UpdateLlamaCPP(cmdRunner runner.CommandRunner, srcPath, installPrefix, cudaArch string,
	withCuda, verbose bool, stdout io.Writer) error {

	var err error
	srcPath, err = filepath.Abs(srcPath)
	if err != nil {
		return i18n.Errorf(i18n.MsgErrSrcAbs, err)
	}
	info, err := os.Stat(srcPath)
	if err != nil {
		return i18n.Errorf(i18n.MsgErrSrcNotFound, err, srcPath)
	}
	if !info.IsDir() {
		return i18n.Errorv(i18n.MsgErrSrcNotDir, srcPath)
	}

	// Upstream check — quiet, no noise in stderr.
	upstreamCheck := cmdRunner.Command("git", "-C", srcPath,
		"rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	upstreamCheck.Stdout = io.Discard
	upstreamCheck.Stderr = io.Discard
	if err := upstreamCheck.Run(); err != nil {
		fmt.Fprint(stdout, i18n.TF(i18n.MsgNoUpstreamSkipPull))
	} else {
		pullArgs := []string{"-C", srcPath, "pull", "--ff-only"}
		if verbose {
			fmt.Fprint(stdout, i18n.TF(i18n.MsgGitCmd, strings.Join(pullArgs, " ")))
		}
		pull := cmdRunner.Command("git", pullArgs...)
		pull.Stdout = stdout
		pull.Stderr = stdout
		if err := pull.Run(); err != nil {
			return i18n.Errorf(i18n.MsgErrGitPull, err)
		}
	}

	installAbs, err := filepath.Abs(installPrefix)
	if err != nil {
		return i18n.Errorf(i18n.MsgErrInstallPrefixAbs, err)
	}
	cmakeArgs := []string{
		"-S", srcPath,
		"-B", filepath.Join(srcPath, "build"),
		"-DCMAKE_BUILD_TYPE=Release",
		"-DCMAKE_INSTALL_PREFIX=" + installAbs,
	}
	if withCuda {
		cmakeArgs = append(cmakeArgs, "-DGGML_CUDA=ON")
		if cudaArch != "" {
			cmakeArgs = append(cmakeArgs, "-DCMAKE_CUDA_ARCHITECTURES="+cudaArch)
		} else {
			cmakeArgs = append(cmakeArgs, "-U", "CMAKE_CUDA_ARCHITECTURES")
		}
	} else {
		cmakeArgs = append(cmakeArgs, "-DGGML_CUDA=OFF", "-U", "CMAKE_CUDA_ARCHITECTURES")
	}
	if verbose {
		fmt.Fprint(stdout, i18n.TF(i18n.MsgCMakeCmd, strings.Join(cmakeArgs, " ")))
	}
	configure := cmdRunner.Command("cmake", cmakeArgs...)
	configure.Dir = srcPath
	configure.Stdout = stdout
	configure.Stderr = stdout
	if err := configure.Run(); err != nil {
		return i18n.Errorf(i18n.MsgErrCMakeConfigure, err)
	}

	jobs := runtime.NumCPU()
	fmt.Fprint(stdout, i18n.TF(i18n.MsgBuilding, jobs))
	buildArgs := []string{"--build", filepath.Join(srcPath, "build"),
		"--config", "Release", "-j", fmt.Sprintf("%d", jobs)}
	if verbose {
		fmt.Fprint(stdout, i18n.TF(i18n.MsgCMakeCmd, strings.Join(buildArgs, " ")))
	}
	build := cmdRunner.Command("cmake", buildArgs...)
	build.Dir = srcPath
	build.Stdout = stdout
	build.Stderr = stdout
	if err := build.Run(); err != nil {
		return i18n.Errorf(i18n.MsgErrCMakeBuild, err)
	}

	fmt.Fprint(stdout, i18n.TF(i18n.MsgInstalling, installPrefix))
	installArgs := []string{"--install", filepath.Join(srcPath, "build")}
	if verbose {
		fmt.Fprint(stdout, i18n.TF(i18n.MsgCMakeCmd, strings.Join(installArgs, " ")))
	}
	install := cmdRunner.Command("cmake", installArgs...)
	install.Dir = srcPath
	install.Stdout = stdout
	install.Stderr = stdout
	if err := install.Run(); err != nil {
		return i18n.Errorf(i18n.MsgErrCMakeInstall, err)
	}
	return nil
}
