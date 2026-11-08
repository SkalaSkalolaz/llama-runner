Phase D.1 — runAuto skeleton and planner resolution

Create the entry point for -auto mode. In this phase runAuto runs the
pipeline but only uses the planner profile (no swaps yet from real
requests). The proxy is wired with the swapper, but manual swaps are not
yet triggered because there is nothing to test end-to-end until D.2.
Files you may edit

    internal/app/automode.go (new, up to 180 lines)

    internal/app/run.go (+ up to 15 lines — the if cfg.AutoMode branch)

    internal/app/automode_test.go (new, up to 150 lines)

    internal/i18n/messages.go (+ up to 6 keys)

Tasks

D.1.1 — runAuto

func runAuto(cfg *config.Config, home string, stdout, stderr io.Writer,
cmdRunner runner.CommandRunner, version string) int

Steps:

    Pick planner profile:

        cfg.ProfilesDefault != "" -> that profile.

        Else find profile with Default: true in cfg.Profiles.

        Else root config (cfg itself).

    Log catalog (i18n): "auto mode: N profiles, planner=<alias>".

    spec, code := SpecForConfig(plannerCfg, home, stdout, stderr);
    code != 0 -> return code.

    Start server (copy the relevant bits from run() — or refactor into a
    helper; do not modify run() beyond the early branch).

    Start proxy with proxy.Options{Swapper: profileSwapper}.

    Wait for proxy health.

    Build claudeEnv, claudeArgs (auto args include --append-system-prompt).

    Start claude, wait, return 0.

D.1.2 — run.go branch

if cfg.AutoMode {
return runAuto(cfg, home, stdout, stderr, cmdRunner, version)
}

Place after -doctor / -update handling, before the -server branch (which
-auto conflicts with, validated in Phase A).

D.1.3 — Tests

    TestRunAuto_StartsWithPlanner — recordingRunner, first command is
    planner's alias.

    TestRunAuto_PlannerOverride — -planner coder -> starts coder.

    TestRunAuto_PlannerFromDefault — profile with default: true starts first.

    TestRunAuto_ClaudeArgs — args include --append-system-prompt <catalog>.

    TestRunAuto_NoProfiles — error (exit 2) with message.

    TestRun_AutoDisabled_Unchanged — without -auto, existing path (this is
    the key backward-compat assertion).

Acceptance

    go test -race ./internal/app/... ok.

    TestRun_AutoDisabled_Unchanged proves AutoMode == false -> old path.