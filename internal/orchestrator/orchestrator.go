// Package orchestrator executes a validated planner.Plan on the local
// llama-server: one step at a time, switching the loaded model between
// steps when the plan asks for a different catalog profile. It is the
// "hands" of auto mode — the planner package is the "brain" that produced
// the plan, and internal/app decides when to call it.
//
// Milestone scope: strictly sequential execution in topological order; the
// first failure stops the run. Parallel waves within a model's slots and
// model-affinity batching are later milestones.
package orchestrator

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
	"github.com/SkalaSkalolaz/llama-runner/internal/planner"
	"github.com/SkalaSkalolaz/llama-runner/internal/runner"
)

// DefaultStepTimeout bounds a single step (claude -p plus its verify
// command). Generous on purpose: a coding step on a local model can take
// tens of minutes; the whole run is bounded by the number of steps.
const DefaultStepTimeout = 30 * time.Minute

// Swapper switches the local llama-server between catalog profiles. In
// production it is app.profileSwapper; a nil Swapper disables switching —
// every step then runs on whatever model was loaded at startup.
type Swapper interface {
	// SwapTo loads the profile behind alias and returns the new
	// llama-server base URL. It must not return before the new server is
	// ready to serve.
	SwapTo(ctx context.Context, alias string) (baseURL string, err error)
	// CurrentAlias names the loaded model.
	CurrentAlias() string
}

// Options configures one Run.
type Options struct {
	// Cfg is the planner's own (auto) profile config; it doubles as the
	// step config when the catalog has no entry for a step's alias.
	Cfg *config.Config
	// Profiles maps an alias to its merged profile config, used to build
	// each step's Claude Code environment. May be nil.
	Profiles map[string]*config.Config
	// Home is the user home directory (path expansion).
	Home string
	// Runner spawns claude and verify shells; injected in tests.
	Runner runner.CommandRunner
	// Task is the user's original task text, given to every step.
	Task string
	// BaseURL is the llama-server base URL loaded with the planner model.
	BaseURL string
	// Swapper switches models between steps; nil disables switching.
	Swapper Swapper
	// StepTimeout bounds one step; 0 means DefaultStepTimeout.
	StepTimeout time.Duration
	Stdout, Stderr io.Writer
}

// StepResult reports how one step ended.
type StepResult struct {
	Step planner.Step
	// Alias is the model that executed the step.
	Alias string
	// OK is true when the step ran, produced its artifact and passed its
	// verify command.
	OK bool
	// Output carries a one-line summary of the step's own output.
	Output string
	// Err is set when OK is false.
	Err error
}

// Result summarises a run.
type Result struct {
	Steps []StepResult
	// Failed is true when any executed step failed or the run could not
	// even start (bad plan order, a swap failure).
	Failed bool
}

// Run executes the plan sequentially and returns what happened. Steps are
// reordered topologically (dependencies first); the first failing step
// stops the run — later steps normally build on earlier artifacts, so
// continuing would only stack damage. With a nil Swapper every step runs
// on the startup model. Cancelling ctx stops the run between and during
// steps.
func Run(ctx context.Context, plan planner.Plan, opts Options) Result {
	out := Result{}
	steps, err := plan.Order()
	if err != nil {
		fmt.Fprint(opts.Stderr, i18n.TF(i18n.MsgErrExecPlan, err))
		return Result{Failed: true}
	}
	if opts.StepTimeout <= 0 {
		opts.StepTimeout = DefaultStepTimeout
	}
	currentAlias := opts.Cfg.Alias
	if opts.Swapper != nil {
		currentAlias = opts.Swapper.CurrentAlias()
	}
	currentURL := opts.BaseURL
	for i, step := range steps {
		if err := ctx.Err(); err != nil {
			out.Failed = true
			fmt.Fprint(opts.Stderr, i18n.TF(i18n.MsgExecStepFail, step.ID, err))
			break
		}
		fmt.Fprint(opts.Stdout, i18n.TF(i18n.MsgExecStepStart,
			i+1, len(steps), step.ID, step.Role, step.Alias))
		// Switch models only when the plan asks for another one: swaps
		// are the expensive part of auto mode.
		if opts.Swapper != nil && !strings.EqualFold(step.Alias, currentAlias) {
			fmt.Fprint(opts.Stdout, i18n.TF(i18n.MsgExecSwap, currentAlias, step.Alias))
			newURL, swapErr := opts.Swapper.SwapTo(ctx, step.Alias)
			if swapErr != nil {
				out.Steps = append(out.Steps, StepResult{
					Step: step, Alias: currentAlias, Err: swapErr,
				})
				out.Failed = true
				fmt.Fprint(opts.Stderr, i18n.TF(i18n.MsgErrExecSwap, step.Alias, swapErr))
				break
			}
			currentAlias, currentURL = step.Alias, newURL
		}
		sr := runStep(ctx, step, opts, currentURL)
		out.Steps = append(out.Steps, sr)
		if !sr.OK {
			out.Failed = true
			fmt.Fprint(opts.Stderr, i18n.TF(i18n.MsgExecStepFail, step.ID, sr.Err))
			break
		}
		if sr.Output != "" {
			fmt.Fprintf(opts.Stdout, "  %s\n", sr.Output)
		}
		fmt.Fprint(opts.Stdout, i18n.TF(i18n.MsgExecStepOK, step.ID))
	}
	done := 0
	for _, sr := range out.Steps {
		if sr.OK {
			done++
		}
	}
	fmt.Fprint(opts.Stdout, i18n.TF(i18n.MsgExecDone, done, len(out.Steps), len(steps)))
	return out
}