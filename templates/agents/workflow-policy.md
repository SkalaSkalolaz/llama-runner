## Model-aware workflow

Before planning, detect the active model from the `ANTHROPIC_DEFAULT_HAIKU_MODEL`
environment variable (llama-runner sets it to the model alias; after alias
resolution this is the GGUF file name without the extension). In your first
message print the line:

    Active model: <name>, cap: <N>

and follow the corresponding cap. If the model name is empty or unrecognised,
use the default cap **2**.

| Model (substring in the name) | Edit cap | Read cap | Notes |
|---|---|---|---|
| Qwen3.8-Flash-Next-UD-Q4_K_XL | 2 | 2 | Strongest but slowest: trust its reasoning, do not duplicate the review with a subagent, do one thorough pass. |
| Qwen3.8-35B-A3B-Distill.APEX-I-MiniPlus-V2.1 | 2 | 3 | Balanced distilled MoE: verify with tests; avoid long contexts — split tasks. |
| Qwen3-Coder-30B-A3B-Instruct | 3 | 3 | Coding model: TDD and multi-file refactors are allowed. Edits are still sequential. |
| Qwen3.8-35B-A3B-Q4_K_M | 2 | 3 | Base MoE Q4: verify with tests; split long tasks and avoid context bloat. |
| (empty / unknown) | 2 | 2 | Safe default. |

### Hard rules for all models

- **Edits are always sequential**, never parallel: llama-server runs with
  `-parallel 1`; parallel agents just queue up and waste time.
- If a task, by your estimate, needs more agents than the cap — **stop and ask
  the user**, do not exceed the limit silently.
- A cap in a specific message («use at most N agents») overrides this table for
  that task.
- If you cannot read `ANTHROPIC_DEFAULT_HAIKU_MODEL` — treat the model as unknown
  and apply cap 2.
- Respect the read cap just like the edit cap: if a task needs more, split it
  into steps instead of reading everything at once.