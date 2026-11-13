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
- Every step must name a verifiable artifact (file, report, test). The "verify" field must be an executable shell command — the executor runs it with sh -c and the step passes only if it exits 0. Natural-language descriptions are NOT executable and will fail.
- Match the verify command to the task's language and tooling. Read the goal and the project context: file extensions (.go/.py/.rs/.ts), explicit language mentions ("in Go", "in Python"), imports. Never emit "go build"/"go test" for a non-Go task, "python -m pytest" for a non-Python task, etc. If the language is not stated and cannot be inferred, prefer a language-neutral check: "test -f X && wc -l X" or "grep -qE 'pattern' X".
- Language-specific verify examples: Go — "go build ./... && go test ./..."; Python — "python -m pytest -q" or "python3 script.py"; Rust — "cargo build && cargo test"; TypeScript/JS — "npm test" or "npx tsc --noEmit"; shell — "bash -n script.sh"; C/C++ — "make && make test". Pick ONE that matches the task.
- Non-code tasks (README, technical writing, research reports, fiction) still need an executable verify that checks CONTENT and returns non-zero when the content is wrong or missing. Examples: "grep -qE '^#+ Chapter [0-9]' book.md", "test -s report.md && grep -qE '^## Conclusion' report.md", "test \"$(wc -l < README.md)\" -ge 30", "test \"$(wc -w < story.txt)\" -ge 400". Do NOT invent a build command for a prose artifact, and do NOT use bare "wc -l FILE" / "wc -w FILE" — they print a number and exit 0 regardless of its value, so they prove nothing.
- If the goal names a QUANTITY ("500 words", "at least N lines", "N sections", "1000-word article"), the verify MUST enforce it numerically: test "$(wc -w < story.txt)" -ge 400 (allow ~20% slack), NOT "wc -w story.txt". Same for lines: test "$(wc -l < README.md)" -ge 30. A bare "wc -w" / "wc -l" exits 0 no matter what the count is and proves nothing.
- awk and grep-pipelines: awk exits 0 even when its pattern matches nothing, so "wc -w < X | awk '$1 >= 400'" always passes. Use "test \"$(wc -w < X)\" -ge 400" (test propagates the comparison) or "grep -qE '^(4[0-9]{2}|5[0-9]{2}|600)$'" (grep -q propagates: 0 on match, 1 on no match). If you must use awk, make it explicit: "awk '{ exit ($1 >= 400) ? 0 : 1 }'".
- Verify CONTENT, not just existence: a bare "test -f X" proves nothing — pair it with a content check ("grep -qE 'func BinarySearch\\(nums \\[\\]int, target int\\) int' binary_search.go", "grep -q '^def binary_search' search.py", "test -s book.md && grep -qE '^# Chapter' book.md").
- Never mask the exit code: no "| head", no "| tail", no "|| true", no "|| (fallback that only checks file existence)". The last command's status is the verdict.
- If a later step must fix problems found by a review step, the review step must NOT fail its verify on failing tests: it writes its findings to a report file and its verify only checks that the report exists (e.g. "test -s review-report.md"); the fix step depends on the review step and reads that report file.
- If the task names several deliverables ("function and tests", "design and implementation", "README and examples"), emit one step per deliverable with depends_on — do not merge them into a single step.
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
