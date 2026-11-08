Phase A — Foundation: catalog, env vars, flags

Implement the infrastructure for -auto mode: the flag, the catalog type,
the environment variables for Claude Code. No swapping yet, no menu changes.
Files you may edit

    internal/cli/parse.go (+ up to 30 lines)

    internal/cli/modelcatalog.go (new, up to 120 lines)

    internal/cli/build.go (+ up to 40 lines)

    internal/config/config.go (+ up to 20 lines)

    internal/config/config_file.go (+ up to 25 lines)

    internal/i18n/messages.go (+ up to 10 new keys)

    internal/cli/modelcatalog_test.go (new)

    internal/cli/build_test.go (extend)

    internal/cli/parse_test.go (extend)

Tasks

A.1 — Flag -auto

In parse.go, add:

fs.BoolVar(&cfg.AutoMode, "auto", false, "auto mode: expose all profiles, hot-swap on model change")
fs.StringVar(&cfg.ProfilesDefault, "planner", "", "planner profile for -auto (default: profile with default: true, else root)")

Add AutoMode bool and ProfilesDefault string fields to config.Config.

A.2 — Extend FileConfig

In config_file.go, add to the FileConfig struct:

Description *string json:"description,omitempty"
Default *bool json:"default,omitempty"

Add to config.Config:

ProfileDescriptions map[string]string // alias -> description, filled in AutoMode
Profiles map[string]*Config // filled in AutoMode

A.3 — Populate profiles when -auto

In ParseFlagsWithHome, at the end, if cfg.AutoMode:

    For each profile in the config file's Profiles:

        Start from a copy of cfg (root config).

        ApplyFileConfig(copy, profileConfig).

        Store in cfg.Profiles[name].

    Fill cfg.ProfileDescriptions from each profile's description.

The root config itself is cfg (unchanged) — it is available as a fallback.

A.4 — BuildModelCatalog

New file modelcatalog.go:

type ModelEntry struct {
Alias string
ProfileName string // "" for root
ModelPath string
Parallel string
CtxSize string
Description string
Default bool
IsRoot bool
}

func BuildModelCatalog(cfg *config.Config) []ModelEntry
func DescribeCatalog(entries []ModelEntry) string

BuildModelCatalog:

    If cfg.Profiles is empty, return [{IsRoot: true, Alias: cfg.Alias, ...}].

    Else: one entry per profile plus one entry for root (if root alias
    differs from all).

DescribeCatalog renders a compact text block, one line per entry, e.g.:

    alias=qwen3.8-auto profile=auto parallel=1 ctx=131072 default=true. Planner: decomposes the task...

    alias=qwen3.8-flash profile=coder parallel=1 ctx=262144. Heavy coding...

    ...

A.5 — Env vars in BuildClaudeEnv

In build.go, append these to the env slice (both modes, always present):

LLAMA_RUNNER_ALIAS=<cfg.Alias>
LLAMA_RUNNER_PARALLEL=<cfg.Parallel>
LLAMA_RUNNER_PROFILE=<cfg.Profile> (empty for root)
LLAMA_RUNNER_CONFIG=<cfg.ConfigPath>
LLAMA_RUNNER_AUTO=1 or 0

A.6 — Validation

In parse.go after flag.Parse:

    -auto + -server -> error exit 2 (MsgErrAutoServerConflict).

    -planner X without -auto -> error exit 2 (MsgErrPlannerWithoutAuto).

A.7 — Tests

    TestBuildModelCatalog_Empty — no profiles, returns 1 root entry.

    TestBuildModelCatalog_FourProfiles — 4 entries, all fields populated.

    TestDescribeCatalog_Renders — output contains all aliases.

    TestBuildClaudeEnv_ParallelEnv — LLAMA_RUNNER_PARALLEL=1 present.

    TestBuildClaudeEnv_AutoFlag — LLAMA_RUNNER_AUTO=1 when AutoMode.

    TestParseFlags_AutoServerConflict — exit code 2.

    TestParseFlags_PlannerWithoutAuto — exit code 2.

Acceptance

    go build ./... ok.

    go test -race ./internal/cli/... ./internal/config/... ok.

    Existing tests in build_test.go unchanged in behaviour (pass without edits).

    llama-runner -auto -doctor runs and shows catalog entries.