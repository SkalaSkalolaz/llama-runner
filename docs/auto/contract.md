# Auto-mode implementation contract

Public signatures fixed by earlier phases. Later phases MUST NOT change
these without an explicit "BREAKING CHANGE" section approved by the
reviewer.

## How to fill this file (instructions for each phase)

After completing your phase, append to your section:

1. Exported types: name + full definition (all fields).
2. Exported functions/methods: full signature.
3. Exported error values: name + message.
4. Files you created (path + one-line purpose).
5. Files you modified, with a one-line note on what changed.
6. Anything the next phase must NOT change without BREAKING CHANGE.

Do not paste implementation details. Only the public surface.

Example of a well-filled section:

    ## Phase A (Foundation)

    ### New types
    ```go
    type ModelEntry struct {
        Alias       string
        ProfileName string  // "" for root
        ModelPath   string
        Parallel    string
        CtxSize     string
        Description string
        Default     bool
        IsRoot      bool
    }
New functions
go

func BuildModelCatalog(cfg *config.Config) []ModelEntry
func DescribeCatalog(entries []ModelEntry) string

New config fields
go

// in config.Config:
AutoMode            bool
ProfilesDefault     string
ProfileDescriptions map[string]string
Profiles            map[string]*config.Config

New env vars (in claude env)

    LLAMA_RUNNER_ALIAS

    LLAMA_RUNNER_PARALLEL

    LLAMA_RUNNER_PROFILE

    LLAMA_RUNNER_CONFIG

    LLAMA_RUNNER_AUTO

New files

    internal/cli/modelcatalog.go — catalog builder + describer

Modified files

    internal/cli/parse.go — +30 lines, -auto/-planner flags

    internal/cli/build.go — +40 lines, env vars

    internal/config/config.go — +20 lines, new fields

    internal/config/config_file.go — +25 lines, description/default

Must not change without BREAKING CHANGE

    ModelEntry field set (next phases depend on all 8 fields)

    BuildModelCatalog signature

    BuildClaudeEnv output must contain the 5 env vars above

    Config.AutoMode/Profiles/ProfileDescriptions must remain populated by ParseFlagsWithHome

Phase 0 (baseline)

See docs/auto/phase-0-baseline.md for the effective flags table that later
phases use as golden reference.
Phase A (Foundation)

(empty — filled by Phase A)
Phase B.1 (ServerManager.Stop / SwapTo)

(empty — filled by Phase B.1)
Phase B.2 (Swapper)

(empty — filled by Phase B.2)
Phase B.3 (Proxy integration)

(empty — filled by Phase B.3)
Phase C (Menu + prompt)

(empty — filled by Phase C)
Phase D.1 (runAuto skeleton)

(empty — filled by Phase D.1)
Phase D.2 (Orchestration)

(empty — filled by Phase D.2)
Phase E (Docs)

(empty — filled by Phase E)