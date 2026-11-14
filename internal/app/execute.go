package app

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
	"github.com/SkalaSkalolaz/llama-runner/internal/orchestrator"
	"github.com/SkalaSkalolaz/llama-runner/internal/planner"
	"github.com/SkalaSkalolaz/llama-runner/internal/runner"
)

// execStepTimeout bounds one executed step (claude -p) and, separately,
// its verification command — each gets the full budget.
const execStepTimeout = 300 * time.Minute

// runExecute is the headless auto-mode run: build the plan, print the
// human-readable listing, then execute it step by step. Only the
// orchestrator's output carries time prefixes; the planning phase and the
// listing stay clean. -plan-only (runPlanOnly) keeps its JSON untouched.
func runExecute(ctx context.Context, cfg *config.Config, home, baseURL string,
	sw orchestrator.Swapper, cmdRunner runner.CommandRunner,
	stdout, stderr io.Writer) int {
	plan, ok := buildPlan(ctx, cfg, baseURL, stdout, stderr)
	if !ok {
		return 1
	}
	fmt.Fprint(stdout, i18n.TF(i18n.MsgPlanHeader, len(plan.Steps)))
	printPlan(stdout, plan)

	outW := newTimePrefixWriter(stdout)
	errW := newTimePrefixWriter(stderr)
	defer func() { _ = outW.Flush(); _ = errW.Flush() }()

	res := orchestrator.Run(ctx, plan, orchestrator.Options{
		Cfg:         cfg,
		Profiles:    cfg.Profiles,
		Home:        home,
		Runner:      cmdRunner,
		Task:        cfg.Task,
		BaseURL:     baseURL,
		Swapper:     sw,
		StepTimeout: execStepTimeout,
		Stdout:      outW,
		Stderr:      errW,
	})
	if res.Failed {
		return 1
	}
	return 0
}

// printPlan renders the human-readable plan listing: an optional summary
// line, the per-model aggregate (aliases alphabetically, "alias × N"),
// then one block per step. Field labels are padded to width 10 INCLUDING
// the colon; optional fields are printed only when set.
func printPlan(w io.Writer, plan planner.Plan) {
	if plan.Summary != "" {
		fmt.Fprintf(w, "%-10s%s\n", i18n.T(i18n.MsgPlanSummary), plan.Summary)
	}
	counts := make(map[string]int, len(plan.Steps))
	for _, s := range plan.Steps {
		counts[s.Alias]++
	}
	aliases := make([]string, 0, len(counts))
	for a := range counts {
		aliases = append(aliases, a)
	}
	sort.Strings(aliases)
	parts := make([]string, 0, len(aliases))
	for _, a := range aliases {
		parts = append(parts, fmt.Sprintf(i18n.T(i18n.MsgPlanModelCount), a, counts[a]))
	}
	fmt.Fprintf(w, "%-10s%s\n", i18n.T(i18n.MsgPlanModels), strings.Join(parts, ", "))

	for i, s := range plan.Steps {
		fmt.Fprintln(w)
		fmt.Fprintf(w, i18n.T(i18n.MsgPlanStepLine)+"\n",
			i+1, len(plan.Steps), s.ID, s.Role, s.Alias)
		fmt.Fprintf(w, "%-10s%s\n", i18n.T(i18n.MsgPlanGoal), s.Goal)
		if s.Artifact != "" {
			fmt.Fprintf(w, "%-10s%s\n", i18n.T(i18n.MsgPlanArtifact), s.Artifact)
		}
		if s.Verify != "" {
			fmt.Fprintf(w, "%-10s%s\n", i18n.T(i18n.MsgPlanVerify), s.Verify)
		}
		if len(s.DependsOn) > 0 {
			fmt.Fprintf(w, "%-10s%s\n", i18n.T(i18n.MsgPlanDepends), strings.Join(s.DependsOn, ", "))
		}
	}
}
