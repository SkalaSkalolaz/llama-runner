package orchestrator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SkalaSkalolaz/llama-runner/internal/cli"
	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
	"github.com/SkalaSkalolaz/llama-runner/internal/planner"
	"github.com/SkalaSkalolaz/llama-runner/internal/proc"
	"github.com/SkalaSkalolaz/llama-runner/internal/runner"
)

// outputCap bounds how much of a step's own output is kept for reporting;
// summaryCap bounds the one-line summary Run prints per step.
const (
	outputCap  = 256 * 1024
	summaryCap = 300
)

// runStep executes one plan step: claude -p against the currently loaded
// model, then the artifact and verify checks. Every failure mode comes
// back as StepResult{OK: false} — Run decides what it means for the plan.
func runStep(ctx context.Context, step planner.Step, opts Options, baseURL string) StepResult {
	res := StepResult{Step: step, Alias: step.Alias}
	ctx, cancel := context.WithTimeout(ctx, opts.StepTimeout)
	defer cancel()

	claudePath, err := lookClaude(opts)
	if err != nil {
		res.Err = err
		return res
	}
	dir, err := stepWorkDir(opts)
	if err != nil {
		res.Err = err
		return res
	}
	pc := stepConfig(opts, step.Alias)
	cmd := opts.Runner.Command(claudePath, "-p", stepPrompt(opts.Task, step), "--model", pc.Alias)
	cmd.Env = cli.BuildClaudeEnv(pc, baseURL, os.Environ())
	cmd.Dir = dir
	var stdout, stderr cappedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		res.Err = err
		return res
	}
	tracked := proc.Track(cmd)
	select {
	case <-tracked.Done:
	case <-ctx.Done():
		// Kill the step's claude; the tool subprocesses it spawned are
		// Claude Code's own children and die when their parent does.
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-tracked.Done
		res.Err = i18n.Errorv(i18n.MsgErrExecStepTimeout, step.ID, opts.StepTimeout)
		return res
	}
	if waitErr := tracked.Wait(); waitErr != nil {
		res.Err = fmt.Errorf("%w; output: %s", waitErr, tail(stderr.String()))
		return res
	}
	res.Output = summary(stdout.String())
	if err := verifyStep(ctx, step, dir, opts); err != nil {
		res.Err = err
		return res
	}
	res.OK = true
	return res
}

// stepConfig returns the merged profile config behind alias, shaped for
// ONE step's environment: mode, task and the catalog are cleared so the
// child sees exactly its own model (the per-slot context, a static menu),
// not the orchestrator's contracts.
func stepConfig(opts Options, alias string) *config.Config {
	src := opts.Cfg
	if pc, ok := opts.Profiles[alias]; ok && pc != nil {
		src = pc
	}
	cp := *src
	cp.Mode = ""
	cp.Task = ""
	cp.PlanOnly = false
	cp.Profiles = nil
	cp.ProfileDescriptions = nil
	return &cp
}

// lookClaude resolves the claude executable the same way run() does.
func lookClaude(opts Options) (string, error) {
	claudePath := config.ExpandHome(opts.Cfg.ClaudeBin, opts.Home)
	resolved, lookErr := opts.Runner.LookPath(claudePath)
	if lookErr == nil {
		return resolved, nil
	}
	if filepath.IsAbs(claudePath) {
		return claudePath, nil
	}
	return "", i18n.Errorv(i18n.MsgClaudeNotFound, opts.Cfg.ClaudeBin, lookErr)
}

// stepWorkDir is where claude -p runs: -claude-dir when set, otherwise the
// launcher's own working directory.
func stepWorkDir(opts Options) (string, error) {
	if opts.Cfg.ClaudeDir != "" {
		abs, err := filepath.Abs(config.ExpandHome(opts.Cfg.ClaudeDir, opts.Home))
		if err != nil {
			return "", i18n.Errorv(i18n.MsgErrClaudeDirAbs, err)
		}
		info, statErr := os.Stat(abs)
		if statErr != nil {
			return "", i18n.Errorv(i18n.MsgErrClaudeDirUnavail, abs, statErr)
		}
		if !info.IsDir() {
			return "", i18n.Errorv(i18n.MsgErrClaudeDirUnavail, abs, errors.New("not a directory"))
		}
		return abs, nil
	}
	wd, _ := os.Getwd()
	return wd, nil
}

// stepPrompt renders the instruction one agent receives. English, like the
// planner's system prompt: this is model input, not user-facing text. The
// user's task is embedded verbatim.
func stepPrompt(task string, step planner.Step) string {
	var b strings.Builder
	b.WriteString("You are one agent in an automated pipeline that executes a plan step by step.\n\n")
	b.WriteString("Overall task:\n" + task + "\n\n")
	b.WriteString("Your step:\n")
	b.WriteString("- role: " + step.Role + "\n")
	b.WriteString("- goal: " + step.Goal + "\n")
	if step.Artifact != "" {
		b.WriteString("- artifact to produce: " + step.Artifact + "\n")
	}
	if step.Verify != "" {
		b.WriteString("- it will be verified by: " + step.Verify + "\n")
	}
	b.WriteString("\nDo exactly this step, working in the current directory. ")
	b.WriteString("When done, print one short paragraph: what you changed and where the artifact is.\n")
	return b.String()
}

// verifyStep checks the step's outcome: the artifact file must exist (when
// the artifact reads like a path), and the verify command must exit 0 (when
// given). Both come from the planner and are trusted exactly as much as the
// plan itself — auto mode runs on the user's machine with the user's task.
func verifyStep(ctx context.Context, step planner.Step, dir string, opts Options) error {
	if step.Artifact != "" && looksLikePath(step.Artifact) {
		p := step.Artifact
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		if _, err := os.Stat(p); err != nil {
			return i18n.Errorv(i18n.MsgErrExecArtifact, p, err)
		}
	}
	if strings.TrimSpace(step.Verify) == "" {
		return nil
	}
	cmd := verifyCommand(opts.Runner, step.Verify)
	cmd.Dir = dir
	var out cappedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return err
	}
	tracked := proc.Track(cmd)
	select {
	case <-tracked.Done:
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-tracked.Done
		return ctx.Err()
	}
	if err := tracked.Wait(); err != nil {
		return i18n.Errorv(i18n.MsgErrExecVerify, step.Verify, tail(out.String()))
	}
	return nil
}

// verifyCommand wraps the verify string in the platform shell.
func verifyCommand(r runner.CommandRunner, verify string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return r.Command("cmd", "/C", verify)
	}
	return r.Command("sh", "-c", verify)
}

// looksLikePath guesses whether an artifact names a file path (as opposed
// to prose such as "go build passes"): no whitespace. A false positive
// merely costs an existence check; a false negative skips it — the verify
// command still guards the outcome.
func looksLikePath(s string) bool {
	return !strings.ContainsAny(s, " \t\n")
}

// cappedBuffer is an io.Writer that keeps at most outputCap bytes and
// silently discards the rest, so a chatty step cannot exhaust memory.
type cappedBuffer struct{ buf bytes.Buffer }

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := outputCap - c.buf.Len(); room > 0 {
		if len(p) > room {
			_, _ = c.buf.Write(p[:room])
		} else {
			_, _ = c.buf.Write(p)
		}
	}
	return len(p), nil
}

func (c *cappedBuffer) String() string { return c.buf.String() }

// tail returns the last line of s, trimmed — the part of a failing
// command's output most likely to explain it.
func tail(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// summary compresses a step's output into one printable line: its last
// non-empty line, capped at summaryCap runes.
func summary(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			r := []rune(line)
			if len(r) > summaryCap {
				return string(r[:summaryCap]) + "…"
			}
			return line
		}
	}
	return ""
}

var _ io.Writer = (*cappedBuffer)(nil)
