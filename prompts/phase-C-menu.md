Phase C — Model menu and system prompt

Extend the /model menu for auto mode and add --append-system-prompt with
the catalog description. When AutoMode == false, behaviour is unchanged
byte-for-byte.
Files you may edit

    internal/cli/modelmenu.go (+ up to 80 lines)

    internal/cli/build.go (+ up to 40 lines)

    internal/cli/build_menu_env_test.go (extend, + up to 100 lines)

    internal/cli/build_test.go (extend, + up to 60 lines)

    internal/i18n/messages.go (+ up to 5 keys)

Tasks

C.1 — BuildModelMenu under AutoMode

When cfg.AutoMode:

    Fill 4 standard slots (Opus/Sonnet/Haiku/Fable) with the current planner
    alias (from cfg.Alias — that is what run() resolved from -p planner or
    default:true).

    Fill ANTHROPIC_CUSTOM_MODEL_OPTION_N for each non-planner profile in
    cfg.Profiles, in sorted-by-name order.

    If more than 5 profiles total — emit a warning to stderr (via a new
    helper — or log at caller level; keep BuildModelMenu pure).

When cfg.AutoMode == false — the function must produce exactly the same
[]ModelSlot as the current implementation. Add a golden test.

C.2 — BuildClaudeArgs under AutoMode

Append:

args = append(args, "--append-system-prompt",
cli.DescribeCatalog(cli.BuildModelCatalog(cfg)))

Only when cfg.AutoMode. No change otherwise.

C.3 — Golden tests

    TestBuildModelMenu_NonAuto_Unchanged — golden compare against a
    snapshot (commit testdata/modelmenu_non_auto.golden generated in this
    phase; driver does not touch goldens elsewhere, so a one-time creation
    is fine).

    TestBuildClaudeArgs_NonAuto_Unchanged — asserts output equals a fixed
    slice.

    TestBuildModelMenu_Auto_AllProfilesVisible — with 4 profiles, all 4 in
    menu (as custom options beyond index 1).

    TestBuildModelMenu_Auto_PlannerInStandardSlots — planner alias in all
    4 standard slots.

    TestBuildClaudeArgs_Auto_HasSystemPrompt — arg list contains
    --append-system-prompt <catalog>.

Acceptance

    go test -race ./internal/cli/... ok.

    The golden test proves AutoMode == false -> unchanged.