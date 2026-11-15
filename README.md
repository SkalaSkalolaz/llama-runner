# README.md

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://go.dev/)
[![Tests](https://img.shields.io/badge/tests-135%2B-brightgreen.svg)](#development)
[![Race](https://img.shields.io/badge/race-clean-brightgreen.svg)](#development)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](LICENSE)
[![CI](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml/badge.svg)](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml)

**English** · [Русский](README_RU.md)

A local launcher: starts `llama-server` with your GGUF model and automatically opens
**Claude Code** wired to that server. When Claude Code exits, the server shuts down.

## Install

One-liner (no Go required):
```sh
curl -fsSL https://raw.githubusercontent.com/SkalaSkalolaz/llama-runner/master/install.sh | sh
```

With options:
```sh
curl -fsSL .../install.sh | sh -s -- --version v1.9.0 --with-agents --with-default-model
```

The installer places the binary in `~/.local/bin/`, creates config directories,
verifies SHA-256, optionally installs agent files and downloads a small default
Qwen model (~400 MB). It never runs sudo or touches paths outside `$HOME`.

**Note:** `mcp-search-server` (for `-webui-mcp-proxy`) is separate — https://github.com/SkalaSkalolaz/mcp-search-server

## Quick Start

```bash
git clone https://github.com/SkalaSkalolaz/llama-runner.git
cd llama-runner
go build -o llama-runner ./cmd/llama-runner
./llama-runner                  # llama-server + Claude Code (default)
./llama-runner -server          # llama-server only
./llama-runner -webui-mcp-proxy # + search MCP server
```

## Features

- **Auto mode:** a profile with `"mode": "auto"` becomes the planner — it decomposes
  tasks, picks models from `profiles`, and the orchestrator executes with hot-swapping
- **Git guard:** `-git-guard` creates a run branch, checkpoints per wave, rolls back on failure
- **Step retries:** `-max-retries N` (0..5) retries failed steps with error context
- **Project map:** `-repo-map` adds working directory context to the planner prompt
- **Plan preflight:** `-preflight` checks verify commands and artifacts before execution
- **Hybrid mode:** proxy routes between local llama-server (native Anthropic API) and
  external OpenAI-compatible API
- **Auto-download:** `-download` fetches GGUF models (single or sharded) before launch
- **MCP proxy:** `-webui-mcp-proxy` starts search server with CORS proxy
- **Speculative decoding:** `-spec-type`, `-spec-draft-model`, etc.
- **Multimodality:** `-mmproj` for vision projector
- **Model alias:** `-alias` sets llama-server `--alias` and Claude Code `/model` entry
- **Chat template:** `-chat-template "auto"` or explicit path
- **Reasoning control:** `-reasoning` and `-reasoning-budget`
- **Logs with rotation:** size-based rotation in `~/.cache/llama-runner/`
- **Config file:** `~/.config/llama-runner/config.json` with profiles
- **Per-model parallel:** set per-profile, CLI `-model` resets to `"1"`
- **`-doctor`:** diagnostics without spawning processes
- **Clean shutdown:** Ctrl-C/SIGTERM with SIGTERM → SIGKILL escalation

## Flags

### Server and llama.cpp
| Flag | Default | Description |
|------|---------|-------------|
| `-model` | `~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf` | GGUF path |
| `-bin` | `~/.local/bin/llama-server` | llama-server path |
| `-host` / `-port` | `127.0.0.1` / `8080` | Server address |
| `-alias` | derived from `-model` or config | Model alias |
| `-c` / `-b` / `-ub` | `65536` / `2048` / `1024` | Context / batch / micro-batch |
| `-parallel` | `1` | Parallel slots (per-profile) |
| `-ngl` | — | GPU layers |
| `-mmproj` | — | Multimodal projector |
| `-reasoning` / `-reasoning-budget` | — | Reasoning mode / budget |
| `-spec-*` | — | Speculative decoding |
| `-chat-template` | `auto` | Chat template (`auto` / path / `""`) |
| `-fa` / `-fit` / `-lv` | — | Flash attention / fit mode / log verbosity |

### Claude Code
| Flag | Default | Description |
|------|---------|-------------|
| `-server` | | llama-server only |
| `-claude-dir` | current dir | Claude working directory |
| `-ready-timeout` | `180s` | `/health` timeout |
| `-log-file` | `~/.cache/llama-runner/llama-server.log` | Log file |
| `-log-max-mb` / `-log-keep` | `50` / `5` | Log rotation |
| `-git-guard` | `false` | Git safety net (auto mode) |
| `-max-retries` | `0` | Step retries (0..5) |
| `-stream-output` | `false` | Live step output to terminal |
| `-resume` | `false` | Resume last run from journal |

### External Provider (Hybrid)
| Flag | Default | Description |
|------|---------|-------------|
| `-openai` | | OpenAI-compatible API URL |
| `-key` | | API key (`none` or `""` = no auth) |
| `-model-openai` | | External model name |
| `-openai-path` | `/chat/completions` | Path suffix (`""` for OVH/VLM.run) |
| `-proxy-addr` | `127.0.0.1:8082` | Local proxy address |
| `-proxy-token` | | Proxy auth token |

### Config and Diagnostics
| Flag | Default | Description |
|------|---------|-------------|
| `-config` | `~/.config/llama-runner/config.json` | Config file (`""` = don't read) |
| `-download` | | Download model: `<dir><URL>` |
| `-p` | | Profile name |
| `-task` | | Task for planner (auto mode) |
| `-plan-only` | | Build plan only, no execution |
| `-repo-map` | `false` | Project map in planner prompt |
| `-preflight` | `false` | Check plan before execution |
| `-doctor` | | Diagnostics only |
| `-update` | | Update llama.cpp and exit |

## Hybrid Mode

Routes Claude Code requests between local llama-server and external OpenAI-compatible API:
- Local model alias or `llama-server` → local llama-server (native Anthropic API, verbatim)
- Everything else → external provider

```bash
./llama-runner \
  -openai https://api.orcarouter.ai/v1 \
  -key $ORCAROUTER_API_KEY \
  -model-openai deepseek/deepseek-v4-flash-free
```

## Provider Key

Priority: `-key` flag → `LLAMA_RUNNER_OPENAI_KEY` → `OPENAI_API_KEY` →
`~/.config/llama-runner/credentials` file.

**Keyless mode:** `-key none` or `-key ""` sends no `Authorization` header
(for providers like OVH Kepler).

## Config File

`~/.config/llama-runner/config.json` with priority: CLI > config > defaults.

```json
{
  "host": "127.0.0.1",
  "port": "8080",
  "model": "~/GGUF/Qwen2.5-0.5B-Instruct-Q4_K_M.gguf",
  "alias": "qwen2.5-0.5b",
  "profiles": {
    "auto": {
      "mode": "auto",
      "model": "~/GGUF/Qwen3.5-9B-Agentic-Coder-Q5_K_M.gguf",
      "alias": "auto",
      "parallel": "1",
      "description": "Planner: task decomposition"
    },
    "small": {
      "model": "~/GGUF/Qwen2.5-0.5B-Instruct-Q4_K_M.gguf",
      "alias": "qwen2.5-0.5b",
      "parallel": "1"
    }
  }
}
```

### Profiles

| Profile | Alias | Model | Context | Parallel | Purpose |
|---------|-------|-------|---------|----------|---------|
| `auto` | `auto` | Qwen3.5-9B-Agentic-Coder | 32768 | 1 | Planner |
| `small` | `qwen2.5-0.5b` | Qwen2.5-0.5B-Instruct | 32768 | 1 | Lightweight tasks |
| `a3b` | `qwen3.8-a3b` | Qwen3.8-35B-A3B | 262144 | 1 | Complex tasks |

### The `/model` Menu

With `profiles` section, the menu is dynamic:
- **Opus** (Default): `switch-to:<startup-alias>`
- **Sonnet / Fable**: `switch-to:<other-profile>`
- **Haiku**: bare primary model (never triggers swap)
- **Custom 1**: `switch-to:<third-profile>`

Only `switch-to:` entries restart llama-server.

## Auto Orchestration

A profile with `"mode": "auto"` becomes the planner. The orchestrator executes
the plan with hot-swapping between models.

**Headless:** `llama-runner -p auto -task "refactor config"`

**Dry-run:** `llama-runner -p auto -task "..." -plan-only > plan.json`

**Interactive:** `llama-runner -p auto` (task → plan → execute loop)

**Safety nets:**
- `-git-guard` — branch + checkpoint per wave + rollback
- `-max-retries N` — retry failed steps with error context
- `-repo-map` — project context in planner prompt
- `-preflight` — verify commands and artifacts before execution
- `-stream-output` — live step output to terminal
- `-resume` — continue last run from journal

## Auto-Download Models

```bash
./llama-runner -download ~/GGUF/https://huggingface.co/org/repo/resolve/main/model.gguf
```

Sharded models (`*-00001-of-00004.gguf`) download the entire set. Resumable via
`.partial` files. Gated repos: set `HF_TOKEN` or `HUGGING_FACE_HUB_TOKEN`.

## Examples

```bash
# Server + Claude Code
./llama-runner -alias qwen2.5-0.5b -claude-dir ~/my-project

# Hybrid mode with proxy token
./llama-runner -openai https://api.orcarouter.ai/v1 \
  -key $ORCAROUTER_API_KEY -model-openai deepseek/deepseek-v4-flash-free \
  -proxy-token $(openssl rand -hex 16)

# Auto mode with all safety nets
./llama-runner -p auto -git-guard -max-retries 2 -repo-map -preflight \
  -task "refactor the config package"

# Download model and launch
./llama-runner -download ~/GGUF/https://huggingface.co/org/repo/resolve/main/model.gguf

# Diagnostics
./llama-runner -doctor
```

## Environment Variables

Passed to Claude Code:
- `ANTHROPIC_BASE_URL` — server/proxy address
- `ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_API_KEY` — proxy token or `local-llama`
- `ANTHROPIC_DEFAULT_{OPUS,SONNET,HAIKU,FABLE}_MODEL` — `/model` menu
- `ANTHROPIC_CUSTOM_MODEL_OPTION*` — custom menu entries
- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`
- `CLAUDE_CODE_MAX_CONTEXT_TOKENS` — per-slot context

The alias is resolved once per run (`cli.ResolveAlias`), so llama-server
`--alias`, WebUI card, and Claude Code `/model` always match.

## Repository Layout

| Path | Contents |
|------|----------|
| `cmd/llama-runner/` | Executable entry point |
| `internal/` | Implementation packages |
| `templates/` | Files installed by `install.sh` |
| `templates/agents/` | `AGENTS.md`, `workflow-policy.md` (EN/RU) |
| `examples/` | `config.example.json` |
| `install.sh` | One-shot installer |
| `AGENTS.md`, `CLAUDE.md` | Instructions for AI agents |

## Development

```bash
go build ./...
go vet ./...
gofmt -l .
go test -race -cover ./...
```

CI: gofmt → build → vet → `go test -race -cover`. Binary version injected via `-ldflags`.

---

[BSD 3-Clause License](LICENSE) · Copyright © 2026 Skala Skalaz