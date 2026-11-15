package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
	"github.com/SkalaSkalolaz/llama-runner/internal/planner"
)

// failureKind classifies why a step failed. Retry decisions use the kind,
// never the error text.
type failureKind int

const (
	failNone     failureKind = iota
	failSetup                // claude lookup, work dir, Start
	failTimeout              // step or verify timeout
	failExec                 // claude exited non-zero
	failArtifact             // artifact missing
	failVerify               // verify command failed
)

func retryable(k failureKind) bool {
	return k == failExec || k == failArtifact || k == failVerify
}

// modelRetries returns the retry limit for a step: the value from the step's
// profile, or the root config's MaxRetries when the profile is absent.
// The result is clamped to 0..5.
func modelRetries(opts Options, alias string) int {
	max := opts.Cfg.MaxRetries
	if pc, ok := opts.Profiles[alias]; ok && pc != nil {
		if pc.MaxRetries > 0 {
			max = pc.MaxRetries
		}
	}
	if max < 0 {
		max = 0
	}
	if max > 5 {
		max = 5
	}
	return max
}

// runStepWithRetry executes a step and its retries. Duration is the total
// wall-clock time across all attempts; Attempts is the number of attempts
// (1 means it succeeded on the first try).
func runStepWithRetry(ctx context.Context, step planner.Step, opts Options,
	baseURL string, steps []planner.Step, byID map[string]StepResult) StepResult {
	start := time.Now()
	max := modelRetries(opts, step.Alias)
	var res StepResult
	for attempt := 1; ; attempt++ {
		retry := ""
		if attempt > 1 {
			retry = retryBlock(attempt-1, max, res)
		}
		res = runStep(ctx, step, opts, baseURL, steps, byID, retry)
		res.Attempts = attempt
		if res.OK || attempt > max || !retryable(res.kind) || ctx.Err() != nil {
			res.Duration = time.Since(start)
			return res
		}
		fmt.Fprint(opts.Stdout, i18n.TF(i18n.MsgExecStepRetry, attempt, max, step.ID, res.Err))
	}
}

// retryBlock builds the English prompt block injected before a retry attempt.
// It is model input, not user-facing text.
func retryBlock(attempt, max int, prev StepResult) string {
	var b strings.Builder
	b.WriteString("Previous attempt of this step FAILED. This is retry ")
	b.WriteString(fmt.Sprintf("%d", attempt))
	b.WriteString(" of ")
	b.WriteString(fmt.Sprintf("%d", max))
	b.WriteString(".\n")
	b.WriteString("Failure: ")
	b.WriteString(capRunes(prev.Err.Error(), 1200))
	b.WriteString("\n")
	if prev.Output != "" {
		b.WriteString("Your previous summary: ")
		b.WriteString(capRunes(prev.Output, 600))
		b.WriteString("\n")
	}
	b.WriteString("Fix that specific problem. Keep the work already done, do not start over,\n")
	b.WriteString("and do not change the verification command.\n")
	return b.String()
}

// capRunes truncates s to at most n runes, appending "…" when truncated.
func capRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return string(r)
}
