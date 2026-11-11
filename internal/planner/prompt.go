package planner

import (
	"fmt"
	"sort"
	"strings"
)

// Model describes one profile of the catalog to the planner. It is a plain
// value type so the package stays independent of config/cli: the caller
// (internal/app) fills it from the merged profile configs.
type Model struct {
	Alias       string
	Description string
	// Context is the per-slot window one agent gets on this model
	// (ctx-size / parallel); 0 means unknown.
	Context int
	// Parallel is the model's slot count — the hard limit of agents that
	// may run on it at once; 0 means unknown.
	Parallel int
}

// systemPrompt renders the planner's instructions.
//
// It is deliberately English regardless of the console locale: this is
// model input, not user-facing text, and a fixed language keeps the schema
// adherence of a small local planner predictable. The catalog is sorted by
// alias so the prompt is byte-stable across runs (map order would otherwise
// change it and defeat prompt caching).
func systemPrompt(models []Model, plannerAlias string) string {
	sorted := append([]Model(nil), models...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Alias < sorted[j].Alias })

	var b strings.Builder
	b.WriteString("You are the planning model of a local multi-model runner.\n")
	b.WriteString("Decompose the user's task into steps that other local models will execute.\n\n")
	b.WriteString("Available models (alias | description | context per agent | parallel slots):\n")
	for _, m := range sorted {
		desc := m.Description
		if desc == "" {
			desc = "no description"
		}
		ctx := "unknown"
		if m.Context > 0 {
			ctx = fmt.Sprintf("%d", m.Context)
		}
		par := "unknown"
		if m.Parallel > 0 {
			par = fmt.Sprintf("%d", m.Parallel)
		}
		mark := ""
		if strings.EqualFold(m.Alias, plannerAlias) {
			mark = " (this planner)"
		}
		fmt.Fprintf(&b, "- %s%s | %s | %s | %s\n", m.Alias, mark, desc, ctx, par)
	}
	b.WriteString(`
Rules:
- Assign every step to one alias from the list; pick the model whose description fits the step.
- Only one model is loaded at a time and switching is expensive: group steps by model, do not alternate models step by step.
- "parallel" is the hard limit of agents running at once on that model; steps on the same model may run in parallel only if independent.
- Express ordering with depends_on; steps without dependencies may run in parallel.
- Roles are adaptive: architect, coder, tester, reviewer, researcher, author, editor, ...
- Every step must name a verifiable artifact (file, report, test) and how to verify it.
- Keep goals short and concrete. At most `)
	fmt.Fprintf(&b, "%d", MaxSteps)
	b.WriteString(` steps.

Answer with JSON only — no prose, no markdown fences:
{"summary":"...","steps":[{"id":"1","role":"coder","goal":"...","alias":"...","depends_on":[],"artifact":"...","verify":"..."}]}
`)
	return b.String()
}

// repairPrompt is the follow-up user message after an invalid answer. It
// quotes the exact validation error so the model can fix the specific
// problem instead of re-inventing the whole plan.
func repairPrompt(err error) string {
	return "Your answer is not a valid plan: " + err.Error() +
		"\nReturn the corrected plan as JSON only, with the same schema. No prose, no markdown fences."
}
