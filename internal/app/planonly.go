package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/cli"
	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
	"github.com/SkalaSkalolaz/llama-runner/internal/orchestrator"
	"github.com/SkalaSkalolaz/llama-runner/internal/planner"
	"github.com/SkalaSkalolaz/llama-runner/internal/preflight"
	"github.com/SkalaSkalolaz/llama-runner/internal/repomap"
	"github.com/SkalaSkalolaz/llama-runner/internal/runner"
)

// planTimeout bounds the planning conversation (both -plan-only and the
// planning phase of an execution run). Generous on purpose: on a hybrid
// CPU/GPU offload two prefills of a few thousand tokens can take minutes.
const planTimeout = 15 * time.Minute

// catalogModels describes the profile catalog to the planner. The per-slot
// context (not the raw -c) is published because that is what one agent
// actually gets; the list is sorted by alias so the prompt is stable.
func catalogModels(cfg *config.Config) []planner.Model {
	out := make([]planner.Model, 0, len(cfg.Profiles))
	for alias, pc := range cfg.Profiles {
		m := planner.Model{Alias: alias, Description: cfg.ProfileDescriptions[alias]}
		if pc != nil {
			m.Context = cli.ContextTokensPerSlot(pc)
			if n, err := strconv.Atoi(strings.TrimSpace(pc.Parallel)); err == nil && n > 0 {
				m.Parallel = n
			}
		}
		out = append(out, m)
	}
	if len(out) == 0 {
		// Auto mode declared on the root config without any profiles:
		// the planner is the only model there is.
		out = append(out, planner.Model{
			Alias: cfg.Alias, Context: cli.ContextTokensPerSlot(cfg), Parallel: 1,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out
}

// buildPlan asks the planner model for a validated plan. Progress goes to
// stderr; false means the plan could not be built and the caller must stop.
// With -repo-map the working directory is scanned and described to the
// planner; with -preflight every candidate plan is checked before it is
// accepted, and check errors feed the same repair round as invalid JSON.
// With both flags off the behaviour is byte-identical to the previous version.
func buildPlan(ctx context.Context, cfg *config.Config, home, baseURL string,
	cmdRunner runner.CommandRunner, stdout, stderr io.Writer) (planner.Plan, bool) {
	fmt.Fprint(stderr, i18n.TF(i18n.MsgPlanBuilding, cfg.Alias))
	opts := planner.Options{
		BaseURL: baseURL,
		Alias:   cfg.Alias,
		Models:  catalogModels(cfg),
	}
	if cfg.RepoMap || cfg.Preflight {
		dir, derr := orchestrator.WorkDir(cfg, home)
		if derr != nil {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgErrPlanBuild, derr))
			return planner.Plan{}, false
		}
		if cfg.RepoMap {
			info, serr := repomap.Scan(dir)
			if serr != nil {
				fmt.Fprint(stderr, i18n.TF(i18n.MsgWarnRepoMap, serr))
			} else {
				opts.Project = info.Prompt()
				if cfg.Verbose && opts.Project != "" {
					fmt.Fprint(stderr, i18n.TF(i18n.MsgRepoMapLine, dir, strings.Join(info.Stacks, ", ")))
				}
			}
		}
		if cfg.Preflight {
			opts.ExtraCheck = func(p planner.Plan) error {
				rep := preflight.Check(ctx, p, dir, cmdRunner)
				for _, f := range rep.Findings {
					fmt.Fprint(stderr, i18n.TF(i18n.MsgPreflightFinding, f.StepID, f.Message))
				}
				return rep.Err()
			}
		}
	}
	p := planner.New(opts)
	res, err := p.Build(ctx, cfg.Task)
	if err != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgErrPlanBuild, err))
		return planner.Plan{}, false
	}
	switch {
	case res.Fallback:
		fmt.Fprint(stderr, i18n.TF(i18n.MsgPlanFallback, res.LastErr))
	case res.Repaired:
		fmt.Fprint(stderr, i18n.TF(i18n.MsgPlanRepaired))
	}
	return res.Plan, true
}

// runPlanOnly implements -plan-only: ask the planner for a plan, print it
// as JSON and return. The plan is the artifact the executor consumes, so
// it is emitted machine-readably; diagnostics go to stderr and never
// contaminate it.
//
// Exit codes: 0 — a plan was produced (a fallback plan is a WARNING, not a
// failure); 1 — the planner could not be reached or answered unusably.
func runPlanOnly(ctx context.Context, cfg *config.Config, home, baseURL string,
	cmdRunner runner.CommandRunner, stdout, stderr io.Writer) int {
	plan, ok := buildPlan(ctx, cfg, home, baseURL, cmdRunner, stdout, stderr)
	if !ok {
		return 1
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgErrPlanBuild, err))
		return 1
	}
	fmt.Fprint(stdout, i18n.TF(i18n.MsgPlanHeader, len(plan.Steps)))
	fmt.Fprintln(stdout, string(data))
	return 0
}
