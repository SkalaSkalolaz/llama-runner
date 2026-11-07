# llama-runner

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://go.dev/)
[![Tests](https://img.shields.io/badge/tests-135%2B-brightgreen.svg)](#development)
[![Race](https://img.shields.io/badge/race-clean-brightgreen.svg)](#development)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](LICENSE)

[![CI](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml/badge.svg)](https://github.com/SkalaSkalolaz/llama-runner/actions/workflows/ci.yml)

**English** · [Русский](README_RU.md)

A local launcher: starts `llama-server` with your GGUF model and automatically opens
**Claude Code** wired to that server. When Claude Code exits, the server shuts down.

## Install

One-liner — downloads a pre-built binary (no Go required):

```sh
curl -fsSL https://raw.githubusercontent.com/SkalaSkalolaz/llama-runner/master/install.sh | sh
```

With options (agents, model download, exact version):

```sh
curl -fsSL https://raw.githubusercontent.com/SkalaSkalolaz/llama-runner/master/install.sh \
  | sh -s -- --version v1.8.0 --with-agents --with-default-model
```

`install.sh --help` lists every flag. What the installer does:

- places the binary in `~/.local/bin/llama-runner` (`llama-runner.exe` on Windows/Git Bash; override with `--dir`);
- creates `~/GGUF/`, `~/.cache/llama-runner/`, `~/.config/llama-runner/` (mode 0700) and writes `~/.config/llama-runner/config.json` if it does not exist;
- verifies the binary against a SHA-256 hash embedded in `install.sh` for that version; for latest without an embedded hash it falls back to the release's `sha256sums.txt` — and only with an explicit `--allow-unverified-hash` (or `-y`);
- optionally installs `~/.config/agents/AGENTS.md` and `~/.config/agents/workflow-policy.md` (selected by console locale: `.ru.md` for Russian, plain `.md` otherwise) and appends the matching `@-references` to `~/.claude/CLAUDE.md` (existing content is preserved, duplicates avoided);
- optionally downloads a small default Qwen model (`Qwen2.5-0.5B-Instruct-Q4_K_M.gguf`, ~400 MB) — pass `--with-default-model`, or say "y" at the prompt, or skip with `--no-default-model`;
- checks for `llama-server`, `claude` and `mcp-search-server` and prints warnings if they are not on PATH (it does not install them automatically).

The installer never runs sudo, never touches paths outside `$HOME`, and never overwrites an existing `~/.config/llama-runner/config.json`.

**Note:** `mcp-search-server` (used by `-webui-mcp-proxy`) is a separate project — https://github.com/SkalaSkalolaz/mcp-search-server. Install it separately if you need web search. Any other MCP server (including a remote one) can be wired via `-mcp-config`.

## Quick start (from source)

Building from source requires Go 1.26. For pre-built binaries, see [Install](#install) above.

```bash
git clone https://github.com/SkalaSkalolaz/llama-runner.git
cd llama-runner
go build -o llama-runner ./cmd/llama-runner

./llama-runner                  # llama-server + Claude Code (default)
./llama-runner -server          # llama-server only (legacy behavior)
./llama-runner -webui-mcp-proxy # + search MCP server and CORS proxy
```

## Features

- **Automatic startup:** checks that the model exists, starts the server, waits for
  `/health` to pass and opens Claude Code.
- **One-shot installer:** `curl … | sh` with SHA-256-verified pre-built binaries for linux/darwin/windows × amd64/arm64, optional global agent files and a small default model.
- **Hybrid mode:** a built-in proxy routes Claude Code requests between the local
  `llama-server` and an external OpenAI-compatible API. The local model uses the
  native Anthropic API at `/v1/messages`, with Anthropic-specific features (thinking,
  cache_control, tool_use/tool_result) preserved verbatim. Token authentication via
  `-proxy-token`.
- **MCP proxy:** `-webui-mcp-proxy` starts `mcp-search-server`, generates a temporary
  MCP config and passes CORS through. With `-mcp-strict` — only this MCP, no third-party
  servers from `~/.claude.json`.
- **llama.cpp updates:** `-update` runs `git pull --ff-only` → `cmake configure/build/install`.
- **Speculative decoding:** `-spec-type`, `-spec-draft-model`, `-spec-draft-n-max`,
  `-spec-draft-ngl`.
- **Multimodality:** `-mmproj` attaches a vision projector.
- **Model alias:** `-alias` sets `--alias` in llama-server (its WebUI card) and
  `ANTHROPIC_DEFAULT_*_MODEL` in Claude Code (its `/model` entry). Without
  `-alias` the name follows the model actually chosen: derived from the GGUF
  file name given on the command line (`gpt-oss-20b-Q5_K_M.gguf` →
  `gpt-oss-20b-Q5_K_M`), else the config file/profile alias, else `local`.
  A profile keeps its own alias, since it brings its own model too.
- **Chat template:** `-chat-template "auto"` (picked by model file name, e.g.
  `qwen-fixed-chat-template.jinja`), an explicit path, or `""`.
- **Reasoning control:** `-reasoning` and `-reasoning-budget` (works only for models
  with reasoning support).
- **Logs with rotation:** llama-server output goes to a file (by default
  `~/.cache/llama-runner/llama-server.log`) and is rotated by size.
- **Config file:** `~/.config/llama-runner/config.json` (or `-config <path>`). Priority:
  CLI > file > built-in defaults. `-config ""` disables reading it. Secrets (`openaiKey`,
  `proxyToken`) are supported in the file, but env / `credentials` / a flag are recommended.
- **Profiles:** `-p coder` — short presets from `profiles: {…}` in the config
  file. A profile supplies the model, alias and other settings of the one
  local model this run serves (see «The `/model` menu`).
- **Per-model parallel:** parallel is set per-profile; `-p coder` uses the
  profile value, `-model <other.gguf>` resets it to `"1"`.
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
| `-alias` | derived from `-model`, else config file/profile, else `local` | Model alias — llama-server's `--alias` and `ANTHROPIC_DEFAULT_*_MODEL` |
| `-c`, `-b`, `-ub` | `65536` / `2048` / `1024` | Context / batch / micro-batch |
| `-fa` | `auto` | Flash attention |
| `-parallel` | `1` | Number of parallel slots; can be set per-profile in config file |
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
| `-key` | | API key for the external provider (see «Getting the provider key»); `none` or `""` — use the provider **without** a key |
| `-model-openai` | | Model name at the external provider |
| `-openai-path` | `/chat/completions` | Path suffix appended to `-openai` when forwarding; `""` — no suffix (OVH Kepler, VLM.run) |
| `-proxy-addr` | `127.0.0.1:8082` | Address of the local routing proxy |
| `-proxy-token` | _(empty — no check)_ | Token for incoming proxy requests |
| `-proxy-allow-remote` | `false` | Allow a non-loopback `-proxy-addr` without a warning |

### Config, diagnostics, updates

| Flag | Default | Description |
|------|---------|-------------|
| `-config` | `~/.config/llama-runner/config.json` | Path to the config file; `""` — don't read |
| `-download` | _(not set)_ | Download a model before launch: `<dir><URL>` as one argument, e.g. `~/GGUF/https://huggingface.co/org/repo/resolve/main/model.gguf`. Sharded models are expanded to the full set. After the download, `-model` points at the fetched file; a CLI `-model` is ignored. Not compatible with `-update`. |
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

- the local model alias **or** the token `llama-server` (including picking the
  custom entry from the `/model` menu) → the local
  llama-server (the request goes verbatim to `/v1/messages`, the native Anthropic
  API; no OpenAI conversion — Anthropic-specific features such as thinking,
  cache_control, tool_use/tool_result are preserved as-is);
- **everything else** (`sonnet`, `haiku`, `opus`, any other name) → the external provider.

The local model alias is also added to the interactive `/model` menu via
`ANTHROPIC_CUSTOM_MODEL_OPTION` so it can be selected with arrow keys.

```bash
./llama-runner \
  -openai https://api.orcarouter.ai/v1 \
  -key $ORCAROUTER_API_KEY \
  -model-openai deepseek/deepseek-v4-flash-free
```

A provider that needs no authentication works the same way — pass `-key none`
(see «Running the provider without a key»).

By default requests are forwarded to `<openai>/chat/completions`. Some
providers (e.g. OVH Kepler `https://oai.endpoints.kepler.ai.cloud.ovh.net/v1`,
VLM.run) serve the endpoint at the bare base URL and reject the extra
`/chat/completions` suffix. Pass `-openai-path ""` to send requests to
`<openai>` as-is, or `-openai-path /custom/path` to use a different suffix.

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

### Running the provider without a key

Some OpenAI-compatible endpoints need no authentication. Say so explicitly:

```bash
./llama-runner \
  -openai https://oai.endpoints.kepler.ai.cloud.ovh.net/v1 \
  -model-openai Qwen3.6-27B \
  -openai-path "" \
  -key none
```

- `-key none` (any case, spaces ignored) and `-key ""` make the request to the
  provider carry **no `Authorization` header at all** — not an empty one, not
  `Bearer ` — the same request a bare `curl -H 'Content-Type: application/json'
  …/chat/completions` sends. Everything else (Anthropic ↔ OpenAI conversion,
  streaming, routing by model name, the local proxy and its `-proxy-token`)
  works unchanged.
- This is a decision, not missing data: with `-key none` neither
  `LLAMA_RUNNER_OPENAI_KEY`, `OPENAI_API_KEY` nor the credentials file is
  consulted, so a machine that has a key exported still calls the provider
  unauthenticated when told to.
- `"openaiKey": "none"` in the config file or in a profile does the same thing —
  that is how a keyless provider is stored as a preset. An **empty** `openaiKey`
  in the file still means "not configured, look in the environment".
- A value reading `none` found in the environment or in the credentials file is
  used verbatim as the key: those hold keys, not per-run intent. The trade-off
  is that a provider key which literally reads `none` cannot be passed on the
  command line.
- Omitting `-key` keeps its old meaning (search the environment, then the
  credentials file); `-doctor` prints `OpenAI key: not required (no key sent)`
  for the keyless mode, so it is never mistaken for a forgotten key.

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
`openaiKey`, `modelOpenai`, `openaiPath`, `proxyAddr`, `proxyToken`, `proxyAllowRemote`,
`llamaSrc`, `installPrefix`, `cuda`, `cudaArch`, `dir`, `verbose`, `lmLazy`,
`mcpConfig`, `download`).

```json
{
  "host": "127.0.0.1",
  "port": "8080",
  "jinja": true,
  "readyTimeout": "180s",
  "tools": "all",
  "parallel": "1",

  "logFile": "~/.cache/llama-runner/llama-server.log",
  "logMaxMB": 50,
  "logKeep": 5,

  "chatTemplate": "auto",
  "chatTemplateFallback": "~/GGUF/qwen-fixed-chat-template.jinja",

  "proxyAddr": "127.0.0.1:8082",

  "model": "~/GGUF/Qwen2.5-0.5B-Instruct-Q4_K_M.gguf",
  "alias": "qwen2.5-0.5b",
  "c": "32768",
  "b": "2048",
  "ub": "512",
  "fa": "auto",

  "profiles": {
    "small": {
      "model": "~/GGUF/Qwen2.5-0.5B-Instruct-Q4_K_M.gguf",
      "alias": "qwen2.5-0.5b",
      "c": "32768",
      "b": "2048",
      "ub": "512",
      "fa": "auto",
      "parallel": "1"
    },
    "a3b": {
      "model": "~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf",
      "alias": "qwen3.8-a3b",
      "c": "262144",
      "b": "4196",
      "ub": "1024",
      "fa": "auto",
      "parallel": "1"
    }
  }
}
```

`install.sh` writes exactly this file on a fresh install (see `examples/config.example.json`); edit `model` and `profiles` to match your own GGUF files.

Notes:

- a missing config file at the default path is fine; an explicitly given
  `-config <path>` without a file is an error;
- `-p <name>` without a config file or with an unknown name is an error (not silent);
- secrets (`openaiKey`, `proxyToken`) in the file are technically supported, but
  env or `~/.config/llama-runner/credentials` is recommended;
- `openaiPath` is the path suffix appended to `openai` (default
  `"/chat/completions"`); `"openaiPath": ""` sends requests to the bare
  `openai` URL — what providers like OVH Kepler and VLM.run expect.
- `parallel` can be set per-profile; if the model is overridden via `-model` on
  the CLI, the inherited parallel resets to `"1"`, except in the case of an
  explicit `-parallel N`.

### The `/model` menu

In local mode all five positions (Opus, Sonnet, Haiku, Fable and the custom
entry) carry the single local model — the one selected with `-model` / `-p`,
or the config default if neither was given.

In hybrid mode (`-openai` + `-model-openai` + a key or `-key none`) the four
standard positions carry the external model, and the custom entry carries the
local model. Picking the custom entry routes the request to llama-server
through the proxy. The request goes verbatim to `/v1/messages` (the native
Anthropic API); no OpenAI conversion is applied — this avoids SSE-header delays
under `-reasoning auto` / `-reasoning-budget` that would otherwise trigger a
~5-minute retry-loop in Claude Code.

Config profiles are still selected with `-p <name>` (they apply the profile's
model, alias and other settings as the one running local model), but they are
no longer advertised separately in the menu.

Menu variables inherited from the environment (`ANTHROPIC_DEFAULT_*_MODEL`,
`ANTHROPIC_CUSTOM_MODEL_OPTION*`) are removed before the launcher publishes
its own, so a nested run never shows ghost entries from an outer session.

## Auto-downloading models

The installer can optionally fetch the small default Qwen model for you (see [Install](#install)); the `-download` flag below is for choosing your own model.

The `-download` flag fetches a GGUF model from an HTTP(S) URL into a local
directory and then launches the launcher as usual, with the downloaded
file as the model. The value is a single argument of the form
`<directory><URL>` — the split point is the first `http://` or `https://`
occurrence:

```bash
./llama-runner -download ~/GGUF/https://huggingface.co/AtomicChat/DeepSeek-V4.1-Flash-1017-Uncensored-GSQ-RCO-GGUF/resolve/main/DeepSeek-V4.1-Flash-1017-GSQ-RCO-IQ1_M-00001-of-00004.gguf
```

### Sharded models

Hugging Face serves large GGUFs as numbered shards
(`*-00001-of-00004.gguf`). Passing any shard of such a set downloads the
**whole** set (all `*-NNNNN-of-MMMMM.gguf` siblings) and points
`llama-server` at shard 1 — llama.cpp loads the rest itself. Passing a
shard other than the first produces a warning and still uses shard 1 as
the entry point.

### Resumable transfers

Files are written to `<name>.partial` and renamed onto `<name>` on
completion, so an interrupted run never leaves a "complete" file
half-written. A leftover `.partial` is resumed with a `Range` request on
the next run. The downloader checks `HEAD` first: a file already present
with the expected size is reused without any transfer.

### Gated repositories

For gated Hugging Face repositories, set `HF_TOKEN` or
`HUGGING_FACE_HUB_TOKEN` in the environment. The token is sent as a
`Bearer` header and never printed.

### Interaction with other flags

- `-model` on the CLI is ignored when `-download` is set (the fetched
  file wins); `-v` prints a warning.
- `-update` combined with `-download` is an error (exit 2): `-update`
  never uses a model.
- The model alias is derived from the downloaded GGUF file name, exactly
  as if `-model` had pointed at it.

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

# Download a model from Hugging Face and launch with it
./llama-runner -download ~/GGUF/https://huggingface.co/AtomicChat/DeepSeek-V4.1-Flash-1017-Uncensored-GSQ-RCO-GGUF/resolve/main/DeepSeek-V4.1-Flash-1017-GSQ-RCO-IQ1_M-00001-of-00004.gguf

# Diagnostics without starting anything
./llama-runner -doctor
```

## Environment variables passed to Claude Code

`ANTHROPIC_BASE_URL` (server or proxy address),
`ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_API_KEY` — the value of `-proxy-token`,
or `local-llama` if no token is set,
`ANTHROPIC_DEFAULT_{OPUS,SONNET,HAIKU,FABLE}_MODEL` and `ANTHROPIC_CUSTOM_MODEL_OPTION*`
(the whole `/model` menu — see «The `/model` menu»; in local mode every
position carries the local alias, in hybrid mode the standard positions carry
the external model and the custom entry the local alias). Variables of
these families inherited from the environment are dropped first, so nothing
leaks from an outer llama-runner session.
`CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`.

The alias behind those three variables is resolved once per run (`cli.ResolveAlias`),
so llama-server's `--alias`, its WebUI card and Claude Code's `/model` entry can
never name different models. `-doctor` prints the resolved value together with its
origin: `CLI`, `derived from model`, `profile`, `config file` or `default`.

Note: if llama-runner's stdout/stderr is not a TTY (started from a pipe,
`… | tee`), Claude Code's stdio is redirected to `/dev/tty` so the TUI stays
interactive.

## Repository layout

| Path | Contents |
|------|----------|
| `cmd/llama-runner/` | executable entry point |
| `internal/` | implementation packages (see comments in each) |
| `templates/` | files installed into the user's `$HOME` by `install.sh` |
| `templates/agents/` | `AGENTS.md`, `workflow-policy.md` (EN and RU variants) |
| `templates/claude/CLAUDE.md` | reference snippet; `install.sh` writes the same two `@`-references directly |
| `templates/chat-templates/` | `qwen-fixed-chat-template.jinja` |
| `packaging/` | distro-facing assets (not installed by `install.sh`) |
| `packaging/systemd/` | `llama-runner.service` |
| `packaging/man/` | `llama-runner.1` |
| `packaging/completions/` | bash and zsh completions |
| `examples/` | `config.example.json` — reference config written by `install.sh` |
| `scripts/` | `update-embedded-hashes.py` — CI helper for `install.sh`'s hash table |
| `install.sh` | one-shot installer (also served via `raw.githubusercontent.com`) |
| `AGENTS.md`, `CLAUDE.md` | instructions for AI agents working on this repo |

`AGENTS.md` in the repository root is for AI agents working on llama-runner itself. The files under `templates/agents/` are installed by `install.sh` into `~/.config/agents/` for **other** projects; they are deliberately separate.

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

`install.sh` is checked with shellcheck in CI in addition to the Go pipeline. Python helpers under `scripts/` are excluded from that step by design.
---

[BSD 3-Clause License](LICENSE) · Copyright © 2026 Skala Skalolaz
