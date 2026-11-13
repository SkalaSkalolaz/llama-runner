# Changelog

All notable changes to this project are documented here. Format loosely
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
versioning follows [SemVer](https://semver.org/).

## v1.10.1

### Added
- Per-step and total execution timing. `StepResult.Duration` records how
  long each step took (including verification, on every exit path), and
  `Result.Duration` records the whole run. `formatDuration` renders
  `< 1s` as milliseconds (`"250ms"`), `1s–1min` as seconds with one
  decimal (`"3.4s"`, truncated, never rounded up), `1min–1h` as
  `"Xm Ys"`, and `≥ 1h` as `"Xh Ym Zs"`; negative durations clamp to
  zero.
- Prior-context propagation between steps. Each step's prompt now
  includes the results of every step it transitively depends on (via
  `depends_on`), in plan order (oldest first), deduplicated, and capped
  at `priorContextCap = 8` entries (the oldest are dropped). Every prior
  entry carries the step ID, role, model alias, status, goal, artifact,
  and either the step's summary or its error text. Steps without
  dependencies get a prompt byte-identical to the previous version. The
  context lives in `Run`'s in-memory `byID` map and survives every model
  swap — swapping replaces the server, not the orchestrator's state.
- Artifact content embedding. A prior step's artifact whose name reads
  like a path is embedded into the next step's prompt between
  `--- begin artifact <path> ---` and `--- end artifact <path> ---`
  markers, capped at `artifactContentCap = 4096` bytes with a `…`
  suffix. Binary artifacts (a zero byte in the leading
  `artifactSampleSize = 512` bytes), missing files and directories are
  skipped silently — the step is never failed because an artifact could
  not be read.
- Prose artifacts are explicitly marked in the prior block as
  `artifact (description, NOT a file path): …`, so the next step does
  not try to read a file whose name contains spaces. This covers the
  case where the planner put `"findings report"` into the artifact
  field instead of a real path.
- Human-readable plan listing in `-task` and interactive `-p auto`
  modes. Before execution the plan is printed as a header
  (`Plan (N steps):`), an optional summary line, a per-model aggregate
  sorted by alias, and one block per step: sequence number, `[id]`,
  role, alias joined by ` · `, then the `goal:`, `artifact:`,
  `verify:` and `depends_on:` fields (label padded to 10 characters
  including the colon; optional fields printed only when set; one blank
  line between step blocks). JSON output is now emitted only by
  `-plan-only`, where it remains byte-identical for downstream `jq`.
- Per-line wall-clock timestamps on the orchestrator output. Every line
  produced by `orchestrator.Run` — model swaps, waves, step starts,
  step results, the final summary — is prefixed with `HH:MM:SS `
  (24-hour local time). The planning phase, the plan listing, and the
  `-plan-only` JSON are deliberately prefix-free so machine processing
  and `sed -n '/^{/,$p' | jq` keep working. Partial lines without a
  trailing newline are flushed with their own prefix when the run
  returns.
- Planner prompt rule for review steps. When a later step must fix
  problems found by a review step, the review step writes its findings
  to a report file and its `verify` only checks that the report exists
  (e.g. `test -s review-report.md`); the fix step depends on the review
  step and reads the report file. This stops review steps from failing
  `verify` on failing tests and lets the fixing step run.

### Fixed
- Step summaries no longer lose useful information when the model ends
  its output with a short sign-off. The summary now takes the LAST
  CONTENTFUL paragraph of stdout — one with at least
  `summaryMinRunes = 40` runes after whitespace collapsing — falling
  back to the last paragraph when no paragraph qualifies, then to
  stderr when stdout carries no usable text at all. Internal newlines
  collapse to single spaces; long paragraphs are truncated to
  `summaryCap = 300` runes with an ellipsis. Previously a model that
  ended with `"Done."` produced the summary `"Done."` and hid the
  actual result.

### Changed
- `verify` now runs with its own full `StepTimeout` budget derived from
  the parent (run) context, independently of how long the step itself
  took. A step that spent 29 of its 30 minutes inside the model no
  longer leaves `go test` with one minute of budget; `Ctrl-C` still
  cancels both because the parent context is the run's context.
- The i18n message templates `MsgExecStepOK`, `MsgExecStepFail` and
  `MsgExecDone` now carry the measured duration in their arguments, so
  the terminal shows `step N done in 3.4s`,
  `step N failed in 1.2s: <reason>` and
  `Execution finished in 9.7s: 3 of 3 steps done (3 in plan)`. New
  keys (`MsgPlanSummary`, `MsgPlanModels`, `MsgPlanStepLine`,
  `MsgPlanGoal`, `MsgPlanArtifact`, `MsgPlanVerify`, `MsgPlanDepends`,
  `MsgPlanModelCount`) back the plan listing; their ru and en variants
  use identical format specifier sequences, enforced by the existing
  catalog test.

## v1.10.0

### Added
- Auto mode orchestration: a profile with `"mode": "auto"` declares the
  planner; the planner decomposes a task into steps, assigns each step to
  a catalog profile, and `internal/orchestrator` executes the plan,
  hot-swapping llama-server between models. Two entry points:
  `-task "..."` (headless) and interactive (`-p auto` without `-task`).
- `-plan-only` prints the plan JSON to stdout with all diagnostics on
  stderr, so `-plan-only > plan.json` produces parseable output.
- `proxy.Swapper.CurrentURL()` — the proxy now asks the swapper for the
  live URL instead of caching it, so an executor-driven swap is visible
  to the proxy immediately.

### Fixed
- `MsgServerReady` no longer prints "starting Claude Code" in headless
  auto mode where Claude Code is not started; `MsgServerReadyAuto` is
  printed instead.
- `buildPlan` progress (`MsgPlanBuilding`, `MsgPlanRepaired`) moved from
  stdout to stderr.

### Changed
- `examples/config.example.json` and the `install.sh` heredoc now include
  a reference `auto` profile (`Qwen3.5-9B-Agentic-Coder-Q5_K_M.gguf`).
- `AGENTS.md` and `workflow-policy.md` updated to describe auto mode and
  the hot-swap behaviour (the "model fixed at launch" rule now applies to
  non-auto mode only).

## v1.9.0

### Added
- Keyless external provider: `-key none`, `-key ""`, and `"openaiKey": "none"` in the config file make requests to the provider carry **no** `Authorization` header (field `OpenAIKeyless`). `-doctor` reports `not required (no key sent)` for this mode.

### Changed
- `/model` menu is dynamic when the config file declares profiles: profile switching uses `switch-to:<alias>` in the model value, with the startup profile on Opus and other profiles on Sonnet/Fable/Custom. **Known cosmetic issue:** the `switch-to:` prefix is visible in standard slots; a cleaner layout is planned for 1.9.1.
- Per-profile `parallel` and `temp` are honoured; a CLI `-model` resets an inherited `parallel` to `"1"` unless `-parallel` is given explicitly.
- `fit`, `temp`, `reasoning`, `reasoningBudget`, `cacheTypeK`, `cacheTypeV`, `logVerbosity`, `specType`, `specDraftModel`, `specDraftNMax`, `specDraftNGL` are passed through to llama-server and are all reflected in the flags tables and Config file field lists.
- `config.example.json` and the `install.sh` heredoc JSON now include `"temp": "0.2"`, `"fit": "on"`, `"reasoning": "on"`, `"reasoningBudget": "2048"` in every profile.
- Version bump to `1.9.0`.

### Fixed
- Documentation drift: `-temp`, `-top-p`, `-cache-type-k/v`, `-fit`, `-lv`, `-reasoning`, `-reasoning-budget`, `-spec-*` were missing from the flags tables; `temp`, `topP`, `cacheTypeK/V`, `fit`, `logVerbosity`, `reasoning`, `reasoningBudget`, `spec*` were missing from the Config fields lists. Restored in both READMEs.
- `install.sh` example header used `--version v1.8.0`; now generic `vX.Y.Z`.

## v1.8.0

The state of the project before this changelog was introduced. See git
history and GitHub releases for details.