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
	"time"

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
	// summaryMinRunes is the minimum length of a "contentful" paragraph:
	// shorter tails like "Done." do not deserve to be the step's summary.
	summaryMinRunes = 40
	// artifactContentCap bounds how much of a prior artifact is embedded
	// into the next step's prompt; artifactSampleSize is the leading
	// fragment checked for binary content (zero bytes).
	artifactContentCap = 4096
	artifactSampleSize = 512
)

// runStep executes one plan step: claude -p against the currently loaded
// model, then the artifact and verify checks. Every failure mode comes
// back as StepResult{OK: false} — Run decides what it means for the plan.
// The step's duration is measured through the named result, so EVERY exit
// path (including failures) reports how long the step took.
func runStep(ctx context.Context, step planner.Step, opts Options, baseURL string,
	steps []planner.Step, byID map[string]StepResult) (res StepResult) {
	start := time.Now()
	res = StepResult{Step: step, Alias: step.Alias}
	defer func() { res.Duration = time.Since(start) }()

	stepCtx, cancel := context.WithTimeout(ctx, opts.StepTimeout)
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
	prior := buildPriorBlock(priorContext(step, steps, byID), dir)
	cmd := opts.Runner.Command(claudePath, "-p", stepPrompt(opts.Task, step, prior), "--model", pc.Alias)
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
	case <-stepCtx.Done():
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
	res.Output = pickSummary(stdout.String(), stderr.String())
	// Verify gets the PARENT context, not stepCtx: it must receive its
	// own full timeout budget regardless of what claude already spent.
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
// user's task is embedded verbatim. prior, when non-empty, carries the
// results of the steps this one depends on; with an empty prior the prompt
// is byte-identical to the pre-prior version.
func stepPrompt(task string, step planner.Step, prior string) string {
	var b strings.Builder
	b.WriteString("You are one agent in an automated pipeline that executes a plan step by step.\n")
	b.WriteString("Overall task:\n" + task + "\n")
	b.WriteString("Your step:\n")
	b.WriteString("- role: " + step.Role + "\n")
	b.WriteString("- goal: " + step.Goal + "\n")
	if step.Artifact != "" {
		b.WriteString("- artifact to produce: " + step.Artifact + "\n")
	}
	if step.Verify != "" {
		b.WriteString("- it will be verified by: " + step.Verify + "\n")
	}
	if prior != "" {
		b.WriteString("\n" + prior)
	}
	b.WriteString("\nDo exactly this step, working in the current directory. ")
	b.WriteString("When done, print one short paragraph: what you changed and where the artifact is.\n")
	return b.String()
}

// verifyStep checks the step's outcome: the artifact file must exist (when
// the artifact reads like a path), and the verify command must exit 0 (when
// given). The verify command gets its OWN full StepTimeout budget derived
// from the parent context: a step that spent 29 of its 30 minutes in the
// model must not leave go test with one minute. Ctrl-C still propagates,
// because ctx is the run's context.
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
	vctx, cancel := context.WithTimeout(ctx, opts.StepTimeout)
	defer cancel()
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
	case <-vctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-tracked.Done
		return vctx.Err()
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

// summary compresses a step's output into one printable line: the LAST
// CONTENTFUL paragraph (at least summaryMinRunes after collapsing), or the
// last paragraph when none is contentful. Internal newlines collapse to
// single spaces; the result is capped at summaryCap runes.
func summary(s string) string {
	paras := splitParagraphs(s)
	if len(paras) == 0 {
		return ""
	}
	pick := collapseLine(paras[len(paras)-1])
	for i := len(paras) - 1; i >= 0; i-- {
		if line := collapseLine(paras[i]); len([]rune(line)) >= summaryMinRunes {
			pick = line
			break
		}
	}
	r := []rune(pick)
	if len(r) > summaryCap {
		return string(r[:summaryCap]) + "…"
	}
	return pick
}

// pickSummary prefers stdout; stderr is consulted only when stdout carries
// no usable text at all.
func pickSummary(stdout, stderr string) string {
	if s := summary(stdout); s != "" {
		return s
	}
	return summary(stderr)
}

// splitParagraphs splits s into paragraphs separated by blank lines
// (a blank line may contain whitespace). Empty paragraphs are dropped.
func splitParagraphs(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var out, cur []string
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.Join(cur, "\n"))
			cur = nil
		}
	}
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return out
}

// collapseLine flattens a paragraph into one line: every run of whitespace
// (including newlines) becomes a single space, ends are trimmed.
func collapseLine(p string) string {
	return strings.Join(strings.Fields(p), " ")
}

// readArtifact returns the content of a prior step's artifact for embedding
// into the next prompt, or "" when nothing should be embedded: prose
// artifacts, missing files, directories and binary content (a zero byte in
// the leading artifactSampleSize fragment) are all skipped WITHOUT failing
// the step. Text is capped at artifactContentCap bytes with an ellipsis.
func readArtifact(artifact, dir string) string {
	if artifact == "" || !looksLikePath(artifact) {
		return ""
	}
	p := artifact
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		return ""
	}
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf, err := io.ReadAll(io.LimitReader(f, artifactContentCap+1))
	if err != nil {
		return ""
	}
	sample := buf
	if len(sample) > artifactSampleSize {
		sample = sample[:artifactSampleSize]
	}
	if bytes.IndexByte(sample, 0) >= 0 {
		return "" // binary
	}
	if len(buf) > artifactContentCap {
		return string(buf[:artifactContentCap]) + "…"
	}
	return string(buf)
}

var _ io.Writer = (*cappedBuffer)(nil)
