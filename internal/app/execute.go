package app

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
	"github.com/SkalaSkalolaz/llama-runner/internal/orchestrator"
	"github.com/SkalaSkalolaz/llama-runner/internal/runner"
)

// execStepTimeout bounds one executed step (claude -p plus verification).
const execStepTimeout = 30 * time.Minute

// runExecute is the headless auto-mode run: build the plan (M1), then
// execute it step by step (M2).
//
// Steps talk to llama-server directly, not through the proxy: the executor
// itself drives every swap and always knows the current URL, while the
// proxy only learns about swaps it mediated. A headless run has no
// interactive traffic to route, so the proxy stays out entirely. The
// interactive milestone will need a proxy that is told about swaps.
func runExecute(ctx context.Context, cfg *config.Config, home, baseURL string,
	sw orchestrator.Swapper, cmdRunner runner.CommandRunner,
	stdout, stderr io.Writer) int {
	plan, ok := buildPlan(ctx, cfg, baseURL, stdout, stderr)
	if !ok {
		return 1
	}
	fmt.Fprint(stdout, i18n.TF(i18n.MsgPlanHeader, len(plan.Steps)))
	res := orchestrator.Run(ctx, plan, orchestrator.Options{
		Cfg:         cfg,
		Profiles:    cfg.Profiles,
		Home:        home,
		Runner:      cmdRunner,
		Task:        cfg.Task,
		BaseURL:     baseURL,
		Swapper:     sw,
		StepTimeout: execStepTimeout,
		Stdout:      stdout,
		Stderr:      stderr,
	})
	if res.Failed {
		return 1
	}
	return 0
}
