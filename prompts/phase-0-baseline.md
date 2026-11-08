Phase 0 — Planner: baseline snapshot

You are the Planner. You do NOT write production code in this phase. You
prepare the baseline that later phases will be verified against.
Tasks

    Snapshot current -doctor output. For each profile present in
    ~/.config/llama-runner/config.json (auto, coder, mini, agent, and the root),
    run:

    llama-runner -doctor -p <profile>

    Append the full output to docs/auto/phase-0-baseline.md under a
    ## <profile> heading.

    Record effective flags. From each -doctor output, extract and record
    in a table in the same file: --alias, --parallel, -c, --reasoning (if
    present), --reasoning-budget (if present), --temp (if present).

    This table is the "before" reference; Phase C's golden tests will
    compare against it.

    Create docs/auto/contract.md with this initial content:
    Auto-mode implementation contract

    Public signatures fixed by earlier phases. Later phases MUST NOT change
    these without an explicit ## BREAKING CHANGE section approved by the
    reviewer.
    Phase 0 (baseline)

        Baseline -doctor output for each profile: docs/auto/phase-0-baseline.md.

        Effective flags table: same file.
    Phase A

    (filled by Phase A)
    Phase B.1

    (filled by Phase B.1)
    ...

    Create scripts/auto-driver.sh — copy the reference implementation
    provided in the task description verbatim. Make it executable
    (chmod +x).

    Create prompts/ directory with all phase-*.md and reviewer.md files
    (they are provided; just ensure they exist and are readable).

Files you may create

    docs/auto/phase-0-baseline.md

    docs/auto/contract.md

    scripts/auto-driver.sh

    (verify) prompts/*.md exist

Files you may NOT touch

Anything in internal/, cmd/, go.mod, go.sum.
Acceptance

    docs/auto/phase-0-baseline.md exists with 5 ## <profile> sections.

    docs/auto/contract.md exists with the initial skeleton.

    scripts/auto-driver.sh exists and is executable.

    go build ./... still succeeds (nothing broken).