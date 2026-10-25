# llama-runner

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://go.dev/)
[![Tests](https://img.shields.io/badge/tests-40%2B-brightgreen.svg)](#development)
[![Race](https://img.shields.io/badge/race-clean-brightgreen.svg)](#development)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](LICENSE)

[![CI](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml/badge.svg)](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml)

**English** · [Русский](README_RU.md)

A local launcher: starts `llama-server` with your GGUF model and automatically opens
**Claude Code** attached to that server. When Claude Code exits, the server shuts down.

## Quick start

```bash
go build -o llama-runner .

./llama-runner                  # llama-server + Claude Code (default)
./llama-runner -server          # llama-server only (legacy behaviour)
./llama-runner -webui-mcp-proxy # + search MCP server and CORS proxy
```

## Development

```bash
go build ./...
go vet ./...
go test -v ./...
go test -race ./...
```

## Features

- **Automatic startup:** checks that the model exists, brings the server up, waits for
  `/health` to be ready and opens Claude Code.
- **MCP proxy:** `-webui-mcp-proxy` starts `mcp-search-server`, generates a temporary MCP
  config and passes through CORS. With `-mcp-strict` — only this MCP, nothing third-party
  from `~/.claude.json`.
- **llama.cpp updates:** `-update` runs `git pull` → `cmake configure/build/install`.
- **Speculative decoding:** `-spec-type`, `-spec-draft-model`, `-spec-draft-n-max`,
  `-spec-draft-ngl` flags.
- **Multimodality:** `-mmproj` attaches a projector.
- **Model alias:** `-alias` sets `--alias` in llama-server and `ANTHROPIC_DEFAULT_*_MODEL`
  in Claude Code.
- **Chat template:** `-chat-template "auto"` (inferred from the file name, e.g.
  `qwen-fixed-chat-template.jinja`), an explicit path, or `""`.
- **Reasoning depth control:** `-reasoning` and `-reasoning-budget` (only works for models
  that support reasoning).
- **Config file:** `~/.config/llama-runner/config.json` (or `-config <path>`). Priority:
  CLI > file > built-in defaults. `-config ""` disables. Secrets (`openaiKey`,
  `proxyToken`) can be stored in the file, but env / `credentials` / flags are preferred.
- **Profiles:** `-p coder` — short presets from `profiles: {…}` in the config.
- **`-doctor`:** print resolved paths, version and final command without launching anything.

## Main flags

| Flag | Default | Description |
|------|---------|-------------|
| `-model` | `~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf` | Path to the GGUF file |
| `-bin` | `~/.local/bin/llama-server` | Path to llama-server |
| `-host:port` | `127.0.0.1:8080` | Server address and port |
| `-alias` | `local` | Model alias |
| `-c, -b, -ub` | `65536/2048/1024` | Context / batch / micro-batch |
| `-jinja` | `true` | Jinja templates |
| `-ngl` | _(not passed)_ | Number of layers offloaded to GPU |
| `-fa` | `auto` | Flash attention |
| `-parallel` | `1` | Number of parallel slots |
| `-ready-timeout` | `180s` | Readiness wait timeout |
| `-reasoning` | _(not passed)_ | Reasoning mode: `on`/`off`/`auto` |
| `-reasoning-budget` | _(not passed)_ | Token budget for reasoning: `-1`/`0`/`N` |
| `-v` | | Print the command before running it |
| `-version` | | Print the version |
| `-config` | `~/.config/llama-runner/config.json` | Path to the config file; `""` — do not read |
| `-p` | _(not set)_ | Profile from the config file |
| `-doctor` | | Diagnostics without running |

## Hybrid mode: llama-server + external OpenAI API

You can use a local llama-server and an external OpenAI-compatible service
(OrcaRouter, OpenRouter, DeepSeek, …) at the same time.
In Claude Code, switch models with `/model`:

- `llama-server` (or picking it from the `/model` menu) → local llama-server
- **Everything else** (`sonnet`, `haiku`, `opus`, any other name) → external provider

`llama-server` is also added to the interactive `/model` menu via
`ANTHROPIC_CUSTOM_MODEL_OPTION` so it can be selected with the arrow keys.

```bash
./llama-runner \
  -openai https://api.orcarouter.ai/v1 \
  -key $ORCAROUTER_API_KEY \
  -model-openai deepseek/deepseek-v4-flash-free
```

## Examples

```bash
# Server + Claude Code in a project directory
./llama-runner -alias qwen3-coder -claude-dir ~/my-project

# Server only, on a different port
./llama-runner -server -port 9090

# Server + Claude Code with web search
./llama-runner -webui-mcp-proxy

# Without extra MCP servers from ~/.claude.json
./llama-runner -webui-mcp-proxy -mcp-strict

# With LiLiCorr (speculative decoding)
./llama-runner -server -model ~/models/nemotron-bolt/NVIDIA-Nemotron-3.5-Bolt-74B-A9B-24GB.gguf \
  -mmproj ~/models/nemotron-bolt/mmproj-NVIDIA-Nemotron-3.5-Bolt-74B-A9B-24GB.gguf \
  -spec-type draft-dflash -spec-draft-model ~/models/...-LiLiCorr.gguf \
  -spec-draft-n-max 6 -spec-draft-ngl 999 -ngl 999 \
  -c 98304 -b 2048 -ub 512 -cache-type-k q8_0 -cache-type-v q8_0 \
  -temp 1.0 -top-p 0.95 -fa on -jinja -chat-template ""

# Update llama.cpp (Pascal architecture) and start
./llama-runner -update -cuda-arch 61 && ./llama-runner -server -model ~/GGUF/model.gguf

# Profile from the config file   
./llama-runner -p coder

# Diagnostics without running
./llama-runner -docto
```
## Config File

By default, ~/.config/llama-runner/config.json is read. Value priority:
CLI > config > built-in defaults. The -config "" flag disables reading the file
(behavior as before M6).
```json
{
  "model": "~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf",
  "host": "127.0.0.1",
  "port": "8080",
  "jinja": true,
  "readyTimeout": "180s",
  "chatTemplate": "auto",
  "chatTemplateFallback": "~/GGUF/qwen-fixed-chat-template.jinja",
  "profiles": {
    "coder": {
      "model": "~/GGUF/qwen3-coder.gguf",
      "alias": "qwen3-coder",
      "c": "98304"
    },
    "fast": {
      "model": "~/GGUF/llama3.2-3b.gguf",
      "c": "16384",
      "b": "512"
    }
  }
}
```

## Environment variables passed to Claude Code

`ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY=local-llama`, `ANTHROPIC_AUTH_TOKEN=local-llama`,
`ANTHROPIC_DEFAULT_*_MODEL` (with `-alias`), `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`.

---

[BSD 3-Clause License](LICENSE) · Copyright © 2026 Skala Skalolaz
