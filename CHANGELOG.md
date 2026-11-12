# Changelog

All notable changes to this project are documented here. Format loosely
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
versioning follows [SemVer](https://semver.org/).

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