# llama-runner

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://go.dev/)
[![Tests](https://img.shields.io/badge/tests-135%2B-brightgreen.svg)](#development)
[![Race](https://img.shields.io/badge/race-clean-brightgreen.svg)](#development)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](LICENSE)

[![CI](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml/badge.svg)](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml)

**English** · [Русский](README_RU.md)

A local launcher: starts `llama-server` with your GGUF model and automatically opens
**Claude Code** wired to that server. When Claude Code exits, the server shuts down.

## Quick start

```bash
go build -o llama-runner ./cmd/llama-runner

./llama-runner                  # llama-server + Claude Code (default)
./llama-runner -server          # llama-server only (legacy behavior)
./llama-runner -webui-mcp-proxy # + search MCP server and CORS proxy
```

## Features

- **Automatic startup:** checks that the model exists, starts the server, waits for
  `/health` to pass and opens Claude Code.
- **Hybrid mode:** a built-in proxy routes Claude Code requests between the local
  `llama-server` and an external OpenAI-compatible API (with Anthropic → OpenAI
  request conversion and streaming back). Token authentication via `-proxy-token`.
- **MCP proxy:** `-webui-mcp-proxy` starts `mcp-search-server`, generates a temporary
  MCP config and passes CORS through. With `-mcp-strict` — only this MCP, no third-party
  servers from `~/.claude.json`.
- **llama.cpp updates:** `-update` runs `git pull --ff-only` → `cmake configure/build/install`.
- **Speculative decoding:** `-spec-type`, `-spec-draft-model`, `-spec-draft-n-max`,
  `-spec-draft-ngl`.
- **Multimodality:** `-mmproj` attaches a vision projector.
- **Model alias:** `-alias` sets `--alias` in llama-server and
  `ANTHROPIC_DEFAULT_*_MODEL` in Claude Code.
- **Chat template:** `-chat-template "auto"` (picked by model file name, e.g.
  `qwen-fixed-chat-template.jinja`), an explicit path, or `""`.
- **Reasoning control:** `-reasoning` and `-reasoning-budget` (works only for models
  with reasoning support).
- **Logs with rotation:** llama-server output goes to a file (by default
  `~/.cache/llama-runner/llama-server.log`) and is rotated by size.
- **Config file:** `~/.config/llama-runner/config.json` (or `-config <path>`). Priority:
  CLI > file > built-in defaults. `-config ""` disables reading it. Secrets (`openaiKey`,
  `proxyToken`) are supported in the file, but env / `credentials` / a flag are recommended.
- **Profiles:** `-p coder` — short presets from `profiles: {…}` in the config file.
- **`-doctor`:** show resolved paths, version and the final command without spawning
  any processes.
- **Clean shutdown:** Ctrl-C/SIGTERM stops claude, the server and MCP with
  SIGTERM → SIGKILL escalation; a repeated signal escalates faster.

## Flags

### Server and llama.cpp

| Flag | Default | Description |
|------|---------|-------------|
| `-model` | `~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf` | Path to the GGUF file |
| `-bin` | `~/.local/bin/llama-server` | Path to llama-server |
| `-host` / `-port` | `127.0.0.1` / `8080` | Server address and port |
| `-alias` | `local` (in Claude Code mode) | Model alias |
| `-c`, `-b`, `-ub` | `65536` / `2048` / `1024` | Context / batch / micro-batch |
| `-fa` | `auto` | Flash attention |
| `-parallel` | `1` | Number of parallel slots |
| `-tools` | `all` | Tools mode |
| `-jinja` | `true` | Jinja templates |
| `-chat-template` | `auto` | `auto` / `""` / path to a `.jinja` file |
| `-ngl` | _(not passed)_ | Number of layers offloaded to GPU |
| `-mmproj` | _(not passed)_ | Multimodal projector |
| `-temp`, `-top-p` | _(not passed)_ | Sampling |
| `-cache-type-k`, `-cache-type-v` | _(not passed)_ | KV-cache types |
| `-fit` | _(not passed)_ | `--fit` mode (`on`/`off`) |
| `-lv` | _(not passed)_ | llama.cpp log verbosity |
| `-lm-lazy` | `false` | Lazy mmap mode |
| `-reasoning` | _(not passed)_ | Reasoning mode: `on`/`off`/`auto` |
| `-reasoning-budget` | _(not passed)_ | Token budget for reasoning: `-1`/`0`/`N` |
| `-spec-type` | _(not passed)_ | Spec-decoding type (`draft-mtp`, `draft-dflash`) |
| `-spec-draft-model` | _(not passed)_ | Path to the draft model |
| `-spec-draft-n-max`, `-spec-draft-ngl` | _(not passed)_ | Draft model parameters |
| `-dir` | current directory | Working directory of llama-server |

### Claude Code

| Flag | Default | Description |
|------|---------|-------------|
| `-server` | | llama-server only, no Claude Code |
| `-claude-bin` | `claude` | Path to the Claude Code binary |
| `-claude-dir` | current directory | Working directory of Claude Code |
| `-ready-timeout` | `180s` | Timeout for waiting on `/health` |
| `-log-file` | `~/.cache/llama-runner/llama-server.log` | llama-server log file |
| `-quiet-server` | `true` | Redirect llama-server logs to the file |
| `-log-max-mb` | `50` | Max log size in MB before rotation; `0` — no rotation |
| `-log-keep` | `5` | Number of log archives to keep; `0` — no archives (truncate) |
| `-webui-mcp-proxy` | `false` | CORS proxy + `mcp-search-server` |
| `-mcp-addr` | `127.0.0.1:8090` | Address of `mcp-search-server` |
| `-mcp-config` | temp file | Where to write the MCP config |
| `-mcp-strict` | `false` | Pass `--strict-mcp-config` |

### External provider (hybrid mode)

| Flag | Default | Description |
|------|---------|-------------|
| `-openai` | | URL of the OpenAI-compatible API |
| `-key` | | API key for the external provider (see «Getting the provider key») |
| `-model-openai` | | Model name at the external provider |
| `-proxy-addr` | `127.0.0.1:8082` | Address of the local routing proxy |
| `-proxy-token` | _(empty — no check)_ | Token for incoming proxy requests |
| `-proxy-allow-remote` | `false` | Allow a non-loopback `-proxy-addr` without a warning |

### Config, diagnostics, updates

| Flag | Default | Description |
|------|---------|-------------|
| `-config` | `~/.config/llama-runner/config.json` | Path to the config file; `""` — don't read |
| `-p` | _(not set)_ | Profile from the config file |
| `-doctor` | | Diagnostics without spawning processes |
| `-v` | | Print the command before running it |
| `-version` | | Print the version |
| `-update` | | Update llama.cpp (`git pull --ff-only` + build + install) and exit |
| `-llama-src` | `~/src/llama.cpp` | Path to the llama.cpp sources |
| `-install-prefix` | `~/.local` | Install prefix |
| `-cuda` | `true` | Build with CUDA |
| `-cuda-arch` | `61` | `-DCMAKE_CUDA_ARCHITECTURES` |

## Hybrid mode: llama-server + external OpenAI API

You can use the local llama-server and an external OpenAI-compatible service
(OrcaRouter, OpenRouter, DeepSeek, …) at the same time. A local proxy is started
(by default `127.0.0.1:8082`, flag `-proxy-addr`), and Claude Code's
`ANTHROPIC_BASE_URL` is pointed at it.

Routing by model name in Claude Code (`/model`):

- exactly `llama-server` (including picking it from the `/model` menu) → the local
  llama-server (the request is converted Anthropic → OpenAI, the stream is converted
  back);
- **everything else** (`sonnet`, `haiku`, `opus`, any other name) → the external provider.

`llama-server` is also added to the interactive `/model` menu via
`ANTHROPIC_CUSTOM_MODEL_OPTION` so it can be selected with arrow keys.

```bash
./llama-runner \
  -openai https://api.orcarouter.ai/v1 \
  -key $ORCAROUTER_API_KEY \
  -model-openai deepseek/deepseek-v4-flash-free
```

### Proxy protection

- `-proxy-token <token>` — the proxy checks incoming requests via
  `Authorization: Bearer` or `x-api-key`; the same token is passed to Claude Code
  in `ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_API_KEY`.
- If the proxy listens on a non-loopback address without a token, a warning is shown
  (anyone on the network could use your paid key). A set token or an explicit
  `-proxy-allow-remote` silences it.

## Getting the provider key

Key source priority:

1. the `-key` flag;
2. env `LLAMA_RUNNER_OPENAI_KEY`;
3. env `OPENAI_API_KEY`;
4. the file `~/.config/llama-runner/credentials` — a simple `KEY=VALUE`
   file (`OPENAI_KEY` or `OPENAI_API_KEY`, `#` comments supported);
   permissions `0600` are recommended (otherwise a warning is shown).

The key is resolved only when the external provider is actually requested
(at least one of `-openai` / `-key` / `-model-openai` is set).

## Config file

By default `~/.config/llama-runner/config.json` is read. Value priority:
**CLI > config > built-in defaults**. The `-config ""` flag disables reading
the file. All tunable parameters are available in the file (names closely
match the flags: `model`, `bin`, `host`, `port`, `c`, `b`, `ub`, `fa`, `ngl`,
`jinja`, `tools`, `parallel`, `temp`, `topP`, `cacheTypeK/V`, `fit`,
`logVerbosity`, `reasoning`, `reasoningBudget`, `specType`, `specDraftModel`,
`specDraftNMax`, `specDraftNGL`, `mmproj`, `chatTemplate`,
`chatTemplateFallback`, `logFile`, `logMaxMB`, `logKeep`, `quietServer`,
`mcpAddr`, `mcpStrict`, `webuiMcpProxy`, `alias`, `claudeBin`, `claudeDir`,
`serverOnly`, `readyTimeout` (duration string, e.g. `"180s"`), `openai`,
`openaiKey`, `modelOpenai`, `proxyAddr`, `proxyToken`, `proxyAllowRemote`,
`llamaSrc`, `installPrefix`, `cuda`, `cudaArch`, `dir`, `verbose`, `lmLazy`,
`mcpConfig`).

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

Notes:

- a missing config file at the default path is fine; an explicitly given
  `-config <path>` without a file is an error;
- `-p <name>` without a config file or with an unknown name is an error (not silent);
- secrets (`openaiKey`, `proxyToken`) in the file are technically supported, but
  env or `~/.config/llama-runner/credentials` is recommended.

## Logs

- In Claude Code mode, llama-server (and MCP server) output is written to
  `~/.cache/llama-runner/llama-server.log` (permissions `0600`) to avoid cluttering the TUI.
- Rotation by size: when `-log-max-mb` is reached, the file is archived; archives
  are `<file>.1` (newest) … `<file>.<log-keep>`.
  `-log-keep 0` — no archives (just truncate), `-log-max-mb 0` — rotation disabled.
- Routing proxy logs — `~/.cache/llama-runner/proxy.log`.

## Examples

```bash
# Server + Claude Code in a project folder
./llama-runner -alias qwen3-coder -claude-dir ~/my-project

# Server only on a different port
./llama-runner -server -port 9090

# Server + Claude Code with search
./llama-runner -webui-mcp-proxy

# Without extra MCP servers from ~/.claude.json
./llama-runner -webui-mcp-proxy -mcp-strict

# Hybrid mode with a proxy token
./llama-runner -openai https://api.orcarouter.ai/v1 \
  -key $ORCAROUTER_API_KEY -model-openai deepseek/deepseek-v4-flash-free \
  -proxy-token $(openssl rand -hex 16)

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

# Diagnostics without starting anything
./llama-runner -doctor
```

## Environment variables passed to Claude Code

`ANTHROPIC_BASE_URL` (server or proxy address),
`ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_API_KEY` — the value of `-proxy-token`,
or `local-llama` if no token is set,
`ANTHROPIC_DEFAULT_{SONNET,HAIKU,OPUS}_MODEL` (in local mode — from `-alias`,
in hybrid mode — the external model, with `llama-server` added via
`ANTHROPIC_CUSTOM_MODEL_OPTION`), `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`.

Note: if llama-runner's stdout/stderr is not a TTY (started from a pipe,
`… | tee`), Claude Code's stdio is redirected to `/dev/tty` so the TUI stays
interactive.

## Development

```bash
go build ./...
go vet ./...
gofmt -l .
go test -v ./...
go test -race ./...
```

CI (GitHub Actions): gofmt → build → vet → `go test -race -cover`; the binary
version is injected via `-ldflags "-X main.version=$(git describe --tags --always --dirty)"`.

---

[BSD 3-Clause License](LICENSE) · Copyright © 2026 Skala Skalolaz
