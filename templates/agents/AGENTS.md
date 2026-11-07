# AGENTS.md

Shared instructions for Claude Code across all projects. A project-level
`CLAUDE.md` / `AGENTS.md` may extend or override them — on conflict, the
project file wins.

## Detecting the stack

Before the first edit in an unfamiliar project, detect the stack from root markers:

- `go.mod` → Go;
- `package.json` → Node/TypeScript;
- `Cargo.toml` → Rust;
- `pyproject.toml` / `requirements.txt` → Python;
- `pom.xml` / `build.gradle` → Java/Kotlin;
- otherwise read the README and the CI config.

If the stack is unclear or multiple stacks are present, ask instead of guessing.

## Code-writing rules

- One coherent responsibility per module. This is the main principle; it
  matters more than any numeric limit.
- File size — target 300–500 lines. If a file consistently exceeds the upper
  bound, decompose by functional purpose. Do not sacrifice logical cohesion
  or readability just to hit a number.
- Split by purpose, not by line count. Two 400-line files with different
  responsibilities are better than one 800-line file mixing them.
- Make changes in small, logically complete steps. One step — one reason;
  do not mix refactoring and new functionality.
- Do not change public APIs without necessity. If a signature, exported
  identifier, or file format changes, flag it explicitly.
- Follow the style of the surrounding code. Do not reformat other people's
  files «to your taste»; for new code, use the project conventions.

## Go (if the stack is Go)

- Formatting — `gofmt` only (no manual alignment).
- Naming — Effective Go and Go Code Review Comments: short names in narrow
  scopes, MixedCaps, no `GetFoo` for getters.
- Errors — return, don't panic; wrap with `%w`; check with `errors.Is/As`,
  not string comparison.
- Context — first argument; `context.Background()` only at the top level.
- Goroutines — always with explicit termination (channel, context, defer);
  do not leave dangling goroutines.
- Packages — small, one responsibility; `internal/` for non-public API.
- Dependencies — do not add new ones without a clear need; if you do,
  explain why the standard library is not enough.
- Do not disable the linter or add `//nolint` without explanation.

## Other languages

- Conventions — from README, linter, and CI. If none exist — the language
  standard (PEP 8 for Python, Prettier/ESLint for JS/TS, rustfmt for Rust, etc.).
- Do not mix styles in one file: if a file is already written in one style,
  continue in it.
- Do not add `# noqa`, `@ts-ignore`, `eslint-disable` without explanation.

## Looking up external information

When local knowledge is insufficient, search the internet — but first make
sure the answer isn't already local, and always pin the version you are
looking for.

When to search: `undefined: X` / `no such method` / `cannot use Y as Z`;
`deprecated:` in warnings; an unfamiliar error message; breaking changes
when updating a dependency; CVE/security advisory; non-obvious stdlib or
framework behaviour; protocol and format specifications.

### Look locally first (do not search if the answer is found):

- `go doc <pkg>` and `go doc <pkg>.<Symbol>` — docs of the installed version.
- `grep` in `vendor/` (if present) and in the repository itself.
- README, docs/, CHANGELOG.md, CONTRIBUTING.md.
- `go list -m -versions <module>` — which versions exist.
- `git log` / `git blame` on the dependency if it is vendored or forked.
- Existing tests and golden files — they record actual behaviour.

Source order: official docs of the pinned version (`pkg.go.dev/<pkg>@<version>`)
→ changelog / release notes → GitHub issues by exact error text →
security advisories (`pkg.go.dev/vuln`, `osv.dev`, `govulncheck`) → Stack
Overflow and blogs (only if the above did not answer; flag as less reliable).

### Search rules:

- Version first, then search. Read `go.mod` (or `package.json`, `Cargo.toml`,
  `pyproject.toml`), pin the version, search for it. Do not quote docs of the
  master branch if you are on v1.2.3.
- Quote the URL. Do not invent links: if the URL does not open, it does not
  exist.
- Check the date. A 2019 article about an API that has changed three times can
  do harm.
- Verify against the code. For breaking changes, the changelog is not enough:
  check signatures with `go doc`, run tests, make sure it compiles.
- Conflict «docs vs code» — trust the project's code and tests, but mention
  the discrepancy in your answer.
- Do not send secrets or private code to search. Phrase queries generically:
  error text, API name, package version. Tokens, keys, internal service names,
  proprietary code fragments — never in a query.

Tools: `mcp-search-server` (if started via llama-runner); the built-in
WebSearch/WebFetch of Claude Code; `curl` — only for direct requests to
official sources (`pkg.go.dev`, `proxy.golang.org`, `raw.githubusercontent.com`),
not for searching; `govulncheck ./...` for CVEs.

If the internet is unavailable or search is blocked: say so explicitly;
offer alternatives (`go doc`, local docs, `vendor/`, ask the user); do not
invent APIs from memory — better to ask.

Once found: briefly summarise (what, where, which version, what it changes
in the code). If a dependency must be updated — do not update silently:
propose it, show the `go.mod`/`go.sum` diff (or equivalent), wait for
confirmation. If you found a CVE — report it explicitly, even if not asked.

## Tests

- Every new and changed function is covered by unit tests. For existing
  functions — no worse than current coverage.
- Tests are part of the change, not a separate task. When adding
  functionality, changing behaviour, or fixing a bug, add tests in the same
  edit.
- Do not weaken or delete tests just to make the suite green. A failing test
  — fix the cause. An outdated test — update it deliberately and explain why.
- Do not touch golden files (`testdata/**` and the like) without explicit
  instruction. Regenerate via the built-in flag (`-update` or equivalent)
  and show the diff.

## Checks before handing over

Before saying «done», run the checks for the project stack and show the
result.

### Go (default):

- `gofmt -l .` — empty;
- `go build ./...` — no errors;
- `go vet ./...` — no warnings;
- `go test -race -cover ./...` — all packages ok.

Other languages: take commands from README or CI (`.github/workflows/*`,
Makefile, `package.json` scripts, `pyproject.toml`). If none exist — the
language standard (`pytest`, `npm test`, `cargo test`, etc.). In your answer,
explicitly list which checks you ran.

Common: if a check is not applicable (no Go, no tests, no linter), say so
explicitly, do not stay silent. Do not disable the linter or add suppressions
(`//nolint`, `# noqa`, `@ts-ignore`) without explanation.

## What not to touch / Security

- Never commit changes without a direct user request.
- Never commit `.env`, `*.key`, `*.pem`, `credentials`, tokens, or any other
  secrets. If a secret accidentally ends up in the diff — stop and report.
- Do not change dependency lock files without explicit instruction:
  `go.mod`/`go.sum` (Go), `package-lock.json`/`pnpm-lock.yaml` (Node),
  `Cargo.lock` (Rust), `poetry.lock`/`requirements.txt` (Python), etc.
  Updating dependencies is a separate task.
- Do not touch `.github/workflows/**`, `Dockerfile`, `Makefile`, `Justfile`
  and other infrastructure files without explicit instruction.
- Do not run commands that modify the system outside the project working
  directory: `sudo`, global installs, editing `~/.config`, `~/.ssh`, etc.
- Do not start long commands without warning (building the whole project,
  running the full test suite with `-race` on a large repository) — first say
  how long it may take.

## Commits

- Never commit changes without a direct user request.
- If asked — commit only what belongs to the task; do not mix in unrelated
  edits.
- Message — Conventional Commits: `type(scope): subject`. Body — what and
  why; footer — links to issues, if any.
- Do not `git push`, `--amend`, `--force`, `rebase` without an explicit request.

## Main rule

Never commit changes without a direct user request.