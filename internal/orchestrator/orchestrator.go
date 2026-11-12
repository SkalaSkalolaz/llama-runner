// Package orchestrator executes a validated planner.Plan on the local
// llama-server, one model at a time. Steps assigned to the same model run
// concurrently, up to that model's configured slots; steps assigned to
// different models never overlap, because the server holds exactly one model
// at a time and switching it is the expensive part.
//
// Scheduling (M3). Each round the scheduler looks at the steps whose
// dependencies are done ("ready") and picks a model: the one already loaded
// if it has ready steps, otherwise the model of the earliest ready step. It
// then runs that model's ready steps in waves of at most `parallel` — the
// slot count the profile declares and llama-server was started with (both
// read the same field of the same profile config, so they cannot diverge).
// During a headless run the executor is the server's only client, so the
// configured slot count IS the free slot count; no live occupancy query is
// needed. Steps that become ready for the same model while waves run are
// picked up before any swap, which keeps the number of model switches
// minimal without ever violating dependencies.
package orchestrator

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
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
	// Profiles maps an alias to the merged profile config, used to build
	// each step's Claude Code environment and to read its slot count.
	// May be nil.
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
	StepTimeout    time.Duration
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
	// Failed is true when any executed step failed, a swap failed, or the
	// run could not even start (bad plan order).
	Failed bool
}

// Run executes the plan in model-grouped waves (see the package doc) and
// returns what happened. The first failing step stops the scheduling, but
// the wave it belongs to is allowed to finish — its steps are independent
// by construction, and killing a claude -p mid-write would leave
// half-written artifacts. Cancelling ctx stops the run between waves and
// kills the steps in flight. Result.Steps is always in plan order,
// whatever the completion order of concurrent steps was.
func Run(ctx context.Context, plan planner.Plan, opts Options) Result {
	steps, err := plan.Order()
	if err != nil {
		fmt.Fprint(opts.Stderr, i18n.TF(i18n.MsgErrExecPlan, err))
		return Result{Failed: true}
	}
	if opts.StepTimeout <= 0 {
		opts.StepTimeout = DefaultStepTimeout
	}
	number := make(map[string]int, len(steps))
	for i, s := range steps {
		number[s.ID] = i + 1
	}
	currentAlias := opts.Cfg.Alias
	if opts.Swapper != nil {
		currentAlias = opts.Swapper.CurrentAlias()
	}
	currentURL := opts.BaseURL

	byID := make(map[string]StepResult, len(steps))
	failed := false
	for len(byID) < len(steps) {
		if ctx.Err() != nil {
			failed = true
			break
		}
		// Ready = every dependency already executed. Steps are scanned in
		// topological order, so the slice is deterministic.
		var ready []planner.Step
		for _, s := range steps {
			if _, done := byID[s.ID]; done {
				continue
			}
			allDone := true
			for _, d := range s.DependsOn {
				if _, ok := byID[d]; !ok {
					allDone = false
					break
				}
			}
			if allDone {
				ready = append(ready, s)
			}
		}
		if len(ready) == 0 {
			// Cannot happen for a Validate'd plan (no cycles, no dangling
			// deps); guard anyway — a scheduler deadlock must be loud.
			fmt.Fprint(opts.Stderr, i18n.TF(i18n.MsgErrExecPlan,
				i18n.Errorv(i18n.MsgErrPlanCycle)))
			failed = true
			break
		}

		// Model choice: the loaded one when it has ready steps (no swap),
		// otherwise the model of the earliest ready step.
		target := ""
		for _, s := range ready {
			if strings.EqualFold(s.Alias, currentAlias) {
				target = currentAlias
				break
			}
		}
		if target == "" {
			target = ready[0].Alias
			if opts.Swapper != nil && !strings.EqualFold(target, currentAlias) {
				fmt.Fprint(opts.Stdout, i18n.TF(i18n.MsgExecSwap, currentAlias, target))
				newURL, swapErr := opts.Swapper.SwapTo(ctx, target)
				if swapErr != nil {
					byID[ready[0].ID] = StepResult{
						Step: ready[0], Alias: currentAlias, Err: swapErr,
					}
					failed = true
					fmt.Fprint(opts.Stderr, i18n.TF(i18n.MsgErrExecSwap, target, swapErr))
					break
				}
				currentAlias, currentURL = target, newURL
			} else {
				// No swapper (single-model catalog): bookkeeping only.
				currentAlias = target
			}
		}

		// One wave: as many ready steps of the target model as it has
		// slots. Slots come from the same profile field the server was
		// started with (--parallel), so the executor never overbooks.
		slots := modelParallel(opts.Profiles, target)
		var wave []planner.Step
		for _, s := range ready {
			if strings.EqualFold(s.Alias, target) {
				wave = append(wave, s)
				if len(wave) == slots {
					break
				}
			}
		}
		fmt.Fprint(opts.Stdout, i18n.TF(i18n.MsgExecWave, target, len(wave), slots))
		for _, s := range wave {
			fmt.Fprint(opts.Stdout, i18n.TF(i18n.MsgExecStepStart,
				number[s.ID], len(steps), s.ID, s.Role, s.Alias))
		}
		for _, r := range runWave(ctx, wave, opts, currentURL) {
			byID[r.Step.ID] = r
			if r.OK {
				if r.Output != "" {
					fmt.Fprintf(opts.Stdout, "  %s\n", r.Output)
				}
				fmt.Fprint(opts.Stdout, i18n.TF(i18n.MsgExecStepOK, r.Step.ID))
			} else {
				failed = true
				fmt.Fprint(opts.Stderr, i18n.TF(i18n.MsgExecStepFail, r.Step.ID, r.Err))
			}
		}
		if failed {
			break
		}
	}

	out := Result{Steps: make([]StepResult, 0, len(byID)), Failed: failed}
	done := 0
	for _, s := range steps {
		r, ok := byID[s.ID]
		if !ok {
			continue
		}
		out.Steps = append(out.Steps, r)
		if r.OK {
			done++
		}
	}
	fmt.Fprint(opts.Stdout, i18n.TF(i18n.MsgExecDone, done, len(out.Steps), len(steps)))
	return out
}

// runWave starts every step of the wave in its own goroutine — one claude -p
// per step, all against the same llama-server, whose --parallel slots
// multiplex the requests — and waits for the whole wave to finish. Results
// are indexed by wave position, which is plan order, so reporting stays
// deterministic whatever the completion order.
func runWave(ctx context.Context, wave []planner.Step, opts Options, baseURL string) []StepResult {
	results := make([]StepResult, len(wave))
	var wg sync.WaitGroup
	for i, step := range wave {
		wg.Add(1)
		go func(i int, s planner.Step) {
			defer wg.Done()
			results[i] = runStep(ctx, s, opts, baseURL)
		}(i, step)
	}
	wg.Wait()
	return results
}

// modelParallel returns the slot count the profile behind alias declares —
// the hard cap of steps that may run on that model at once. Missing, empty
// or unparsable values degrade to 1, matching the launcher-wide convention
// for unknown models.
func modelParallel(profiles map[string]*config.Config, alias string) int {
	if pc, ok := profiles[alias]; ok && pc != nil {
		if n, err := strconv.Atoi(strings.TrimSpace(pc.Parallel)); err == nil && n > 0 {
			return n
		}
	}
	return 1
}
