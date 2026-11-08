Phase E — Documentation

Update user-facing docs for -auto.
Files you may edit

    README.md — new section "Auto mode (-auto)" (up to 120 lines)

    README_RU.md — same, in Russian (up to 120 lines)

    CHANGELOG.md — Unreleased / Added

    examples/config.example.json — add description, default, reasoning,
    reasoningBudget, temp to example profiles

    docs/auto/README.md (new) — how the driver works, how to resume,
    how to inspect failed phases

Do NOT touch

    ~/.config/agents/*.md — user's global files, updated separately

    install.sh

    Any Go code (read-only for this phase)

Tasks

E.1 — README section

Content:

    What -auto does (planner + catalog + hot-swap).

    Example config with description and default: true.

    Limitation: 4+1 slots in Claude Code menu.

    -planner <profile> override.

    Environment vars: LLAMA_RUNNER_PARALLEL, etc.

    Reference to scripts/auto-driver.sh.

E.2 — CHANGELOG
Added

    -auto mode: catalog of profiles, hot-swap on model change,
    planner-driven system prompt, environment variables for
    per-session introspection (LLAMA_RUNNER_PARALLEL etc.).

    scripts/auto-driver.sh: sequential multi-agent implementation
    driver (used to build -auto itself).

E.3 — Driver docs

docs/auto/README.md:

    How to invoke: ./scripts/auto-driver.sh [--from PHASE] [--to PHASE]

    Env overrides: timeouts, profile, branch.

    Stop conditions: timeout, build fail, test fail, reviewer REJECT.

    Resuming: tags phase-X on each commit; --from resumes.

    Inspecting failed phase: logs/phase-{id}.log, logs/review-{id}.log.

    Rollback: git reset --hard phase-{id} (last good phase).

Acceptance

    README.md and README_RU.md describe -auto accurately — verified by
    comparing against actual llama-runner -auto -doctor output.

    examples/config.example.json is valid JSON and parses (llama-runner
    -config examples/config.example.json -doctor; model paths may not
    exist, but the config parses).