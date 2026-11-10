#!/bin/sh
# llama-runner installer.
# POSIX sh, no bashisms. Does not require Go on the target machine.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/SkalaSkalolaz/llama-runner/master/install.sh | sh
#   curl -fsSL .../install.sh | sh -s -- --version vX.Y.Z --with-agents
#
# See --help for all options.

set -eu

REPO="SkalaSkalolaz/llama-runner"
BIN_NAME="llama-runner"

# ---- defaults (overridable via env or flags) ----
VERSION="${LLAMA_RUNNER_VERSION:-latest}"
DEST_DIR="${LLAMA_RUNNER_BIN_DIR:-$HOME/.local/bin}"
MODEL_DIR="${LLAMA_RUNNER_MODEL_DIR:-$HOME/GGUF}"
CACHE_DIR="${LLAMA_RUNNER_CACHE_DIR:-$HOME/.cache/llama-runner}"
CONFIG_DIR="$HOME/.config/llama-runner"
AGENTS_DIR="$HOME/.config/agents"

# tri-state: "" = ask (TTY) / no (non-TTY), "yes", "no"
WANT_AGENTS=""
WANT_CHAT_TEMPLATE="yes"
WANT_CONFIG="yes"
WANT_DEFAULT_MODEL=""
DOWNLOAD_MODEL_URL=""
ALLOW_UNVERIFIED_HASH=0
ASSUME_YES=0

# Embedded SHA-256 for released versions. Updated by
# scripts/update-embedded-hashes.py on every release; do not edit by hand.
# Format: VERSION<TAB>OS<TAB>ARCH<TAB>SHA256
EMBEDDED_HASHES='
v1.9.0	windows	amd64	e11b52a3ba88711ad6b2b4b759aa6e98772b2bb6cb6b8c056517be3bd66b232d
v1.9.0	linux	arm64	da5edf3b484582c367ef49173296cdcbc3326cc2f1426c176e2d79593717ab53
v1.9.0	linux	amd64	475b0b630253acef1059e2d0958da2ad34996e3883422d9f7be5c04c827fb371
v1.9.0	darwin	arm64	88e30e31ee248751751e78f9d7bb653019ee72a7d4312ba016944c1ae16c62fd
v1.9.0	darwin	amd64	47fc2475a131d96b3bad2bc5d26ca744236fd74f082804add49aefa13b1dd9ce
v1.8.0	windows	amd64	621bba0e77366008d483af70a114f6aaedd68e95661b960423173404f7ed2583
v1.8.0	linux	arm64	c1ff3336a341cf107fbe3d64629bef1da0b12d56f3fbfe52af0f99a3a2be822c
v1.8.0	linux	amd64	571324fbd379ae0f75ce3dad383ff0cd575203e975844e629c22b397ba630965
v1.8.0	darwin	arm64	6c7bec48577528bb023ac4f4df09cc3fd18b90d0748564844968099335299d10
v1.8.0	darwin	amd64	d478fb8e4b189227bf21ebec7f199236e604ab9cff28f04a5960307666cbfcc4
'

# Default Qwen model for a fresh install (small, ~400 MB).
# The Hugging Face file name is lowercase; download_default_model renames
# the fetched file to the PascalCase name config.json refers to.
DEFAULT_MODEL_URL="https://huggingface.co/Qwen/Qwen2.5-0.5B-Instruct-GGUF/resolve/main/qwen2.5-0.5b-instruct-q4_k_m.gguf"
DEFAULT_MODEL_NAME="Qwen2.5-0.5B-Instruct-Q4_K_M.gguf"

# exe_suffix is ".exe" on Windows (Git Bash / MSYS / Cygwin), "" elsewhere.
exe_suffix=""

# ---------------------------------------------------------------- utilities

usage() {
  cat <<EOF
Usage: install.sh [options]

  --version <tag>          Install a specific version (default: latest)
  --dir <path>             Binary install dir (default: \$HOME/.local/bin)
  --model-dir <path>       Model directory (default: \$HOME/GGUF)
  --cache-dir <path>       Cache dir (default: \$HOME/.cache/llama-runner)
  --download-model <url>   Download a specific GGUF model into --model-dir
  --with-default-model     Download the small default Qwen model
  --no-default-model       Do not offer to download any model
  --with-agents            Install ~/.config/agents/AGENTS.md and workflow-policy.md
                           and add @-references to ~/.claude/CLAUDE.md
  --no-agents              Skip agent files
  --with-chat-template     Install qwen-fixed-chat-template.jinja (default)
  --no-chat-template       Skip the chat template file
  --no-config              Do not write ~/.config/llama-runner/config.json
  --allow-unverified-hash  For 'latest' without an embedded hash: trust sha256sums.txt
  -y, --yes                Non-interactive: accept all defaults
  -h, --help               This help

Environment variables:
  LLAMA_RUNNER_VERSION, LLAMA_RUNNER_BIN_DIR, LLAMA_RUNNER_MODEL_DIR,
  LLAMA_RUNNER_CACHE_DIR
EOF
}

log()  { printf '  %s\n' "$*" >&2; }
warn() { printf 'note: %s\n' "$*" >&2; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

# ask_yes_no PROMPT DEFAULT
# Returns 0 for yes, 1 for no. Uses /dev/tty when available, so it works
# under `curl … | sh`. Falls back to DEFAULT in non-interactive mode.
ask_yes_no() {
  prompt="$1"; default="$2"
  if [ ! -t 0 ] && [ ! -e /dev/tty ]; then
    if [ "$default" = "y" ]; then
      return 0
    else
      return 1
    fi
  fi
  printf '%s [%s]: ' "$prompt" "$default" >&2
  if [ -e /dev/tty ]; then
    read -r ans < /dev/tty || ans=""
  else
    read -r ans || ans=""
  fi
  [ -z "$ans" ] && ans="$default"
  case "$ans" in
    [yY]*) return 0 ;;
    *)     return 1 ;;
  esac
}

is_russian_locale() {
  for v in "${LC_ALL:-}" "${LANG:-}"; do
    case "$v" in
      ""|"C"|"POSIX"|"C.UTF-8") continue ;;
    esac
    case "$v" in
      ru*) return 0 ;;
      *)   return 1 ;;
    esac
  done
  return 1
}

# ---------------------------------------------------------------- args

while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --dir) DEST_DIR="$2"; shift 2 ;;
    --model-dir) MODEL_DIR="$2"; shift 2 ;;
    --cache-dir) CACHE_DIR="$2"; shift 2 ;;
    --download-model) DOWNLOAD_MODEL_URL="$2"; shift 2 ;;
    --with-default-model) WANT_DEFAULT_MODEL="yes"; shift ;;
    --no-default-model)   WANT_DEFAULT_MODEL="no"; shift ;;
    --with-agents)        WANT_AGENTS="yes"; shift ;;
    --no-agents)          WANT_AGENTS="no"; shift ;;
    --with-chat-template) WANT_CHAT_TEMPLATE="yes"; shift ;;
    --no-chat-template)   WANT_CHAT_TEMPLATE="no"; shift ;;
    --no-config)          WANT_CONFIG="no"; shift ;;
    --allow-unverified-hash) ALLOW_UNVERIFIED_HASH=1; shift ;;
    -y|--yes) ASSUME_YES=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; die "unknown argument: $1" ;;
  esac
done

# ---------------------------------------------------------------- platform

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux|darwin) ;;
  mingw*|msys*|cygwin*) os=windows ;;
  *) die "unsupported OS: $os (supported: linux, darwin, windows/git-bash)" ;;
esac
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $arch (supported: amd64, arm64)" ;;
esac
if [ "$os" = "windows" ]; then
  exe_suffix=".exe"
fi

# ---------------------------------------------------------------- version

if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)
  [ -n "$VERSION" ] || die "cannot resolve the latest version"
fi
ver_no_v=${VERSION#v}

case "$os" in
  linux|darwin) asset="${BIN_NAME}_${ver_no_v}_${os}_${arch}.tar.gz" ;;
  windows)      asset="${BIN_NAME}_${ver_no_v}_${os}_${arch}.zip" ;;
esac

base="https://github.com/$REPO/releases/download/$VERSION"
raw_base="https://raw.githubusercontent.com/$REPO/$VERSION"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# ---------------------------------------------------------------- download

log "Downloading $asset"
curl -fsSL "$base/$asset" -o "$tmp/$asset"

# ---------------------------------------------------------------- integrity

expected_hash=$(printf '%s\n' "$EMBEDDED_HASHES" \
  | awk -F '\t' -v v="$VERSION" -v o="$os" -v a="$arch" \
      '$1==v && $2==o && $3==a {print $4; exit}')

if [ -n "$expected_hash" ]; then
  actual_hash=$(sha256sum "$tmp/$asset" | awk '{print $1}')
  if [ "$actual_hash" != "$expected_hash" ]; then
    echo "checksum mismatch for $asset" >&2
    echo "  expected: $expected_hash" >&2
    echo "  actual:   $actual_hash" >&2
    exit 1
  fi
  log "checksum OK (embedded)"
else
  if [ "$ALLOW_UNVERIFIED_HASH" -eq 1 ] || [ "$ASSUME_YES" -eq 1 ]; then
    warn "no embedded hash for $VERSION/$os/$arch; falling back to sha256sums.txt"
    curl -fsSL "$base/sha256sums.txt" -o "$tmp/sha256sums.txt"
    ( cd "$tmp" && grep " $asset\$" sha256sums.txt | sha256sum -c - )
  else
    cat >&2 <<MSG
refusing to continue: no embedded hash for $VERSION/$os/$arch.

Pass one of:
  --version <known-tag>       use a tag whose hash is embedded in this script
  --allow-unverified-hash     trust the sha256sums.txt downloaded from the release
MSG
    exit 1
  fi
fi

# ---------------------------------------------------------------- extract

case "$os" in
  linux|darwin) tar -xzf "$tmp/$asset" -C "$tmp" ;;
  windows)      unzip -q "$tmp/$asset" -d "$tmp" ;;
esac
if [ ! -f "$tmp/$BIN_NAME$exe_suffix" ]; then
  die "binary not found inside $asset"
fi

# ---------------------------------------------------------------- install binary

mkdir -p "$DEST_DIR"
install -m 0755 "$tmp/$BIN_NAME$exe_suffix" "$DEST_DIR/$BIN_NAME$exe_suffix"
log "Installed $DEST_DIR/$BIN_NAME$exe_suffix ($VERSION)"

# ---------------------------------------------------------------- dirs

mkdir -p "$MODEL_DIR" "$CACHE_DIR" "$CONFIG_DIR"
chmod 0700 "$MODEL_DIR" "$CACHE_DIR" "$CONFIG_DIR" 2>/dev/null || true

# ---------------------------------------------------------------- config

write_config() {
  md_esc=$(printf '%s' "$MODEL_DIR" | sed 's/[#&\\]/\\&/g')
  cd_esc=$(printf '%s' "$CACHE_DIR" | sed 's/[#&\\]/\\&/g')
  sed -e "s#~/GGUF/#${md_esc}/#g" \
      -e "s#~/.cache/llama-runner/#${cd_esc}/#g" \
      > "$CONFIG_DIR/config.json" <<'JSON'
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
      "parallel": "1",
      "temp": "0.2",
      "fit": "on",
      "reasoning": "on",
      "reasoningBudget": "2048"
    },
    "a3b": {
      "model": "~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf",
      "alias": "qwen3.8-a3b",
      "c": "262144",
      "b": "4196",
      "ub": "1024",
      "fa": "auto",
      "parallel": "1",
      "temp": "0.2",
      "fit": "on",
      "reasoning": "on",
      "reasoningBudget": "2048"
    }
  }
}
JSON
  chmod 0600 "$CONFIG_DIR/config.json"
  log "Wrote $CONFIG_DIR/config.json"
}

if [ "$WANT_CONFIG" = "yes" ]; then
  if [ -f "$CONFIG_DIR/config.json" ]; then
    log "Keeping existing $CONFIG_DIR/config.json"
  else
    write_config
  fi
fi

# ---------------------------------------------------------------- chat template

if [ "$WANT_CHAT_TEMPLATE" = "yes" ]; then
  tpl_url="$raw_base/templates/chat-templates/qwen-fixed-chat-template.jinja"
  tpl_dst="$MODEL_DIR/qwen-fixed-chat-template.jinja"
  if [ -f "$tpl_dst" ]; then
    log "Keeping existing $tpl_dst"
  else
    log "Downloading chat template"
    if curl -fsSL "$tpl_url" -o "$tpl_dst"; then
      chmod 0644 "$tpl_dst"
    else
      warn "could not download chat template; llama-server will use the template embedded in the GGUF"
      rm -f "$tpl_dst"
    fi
  fi
fi

# ---------------------------------------------------------------- agents

install_agents() {
  mkdir -p "$AGENTS_DIR" "$HOME/.claude"
  if is_russian_locale; then
    suffix=".ru.md"
  else
    suffix=".md"
  fi
  for name in AGENTS workflow-policy; do
    url="$raw_base/templates/agents/${name}${suffix}"
    dst="$AGENTS_DIR/${name}.md"
    if [ -f "$dst" ]; then
      log "Keeping existing $dst"
      continue
    fi
    log "Downloading $name$suffix → $dst"
    if ! curl -fsSL "$url" -o "$dst"; then
      warn "failed to download $url"
      return 1
    fi
    chmod 0644 "$dst"
  done

  # Merge @-references into ~/.claude/CLAUDE.md without duplicates.
  claude_md="$HOME/.claude/CLAUDE.md"
  tmp_md="$claude_md.tmp.$$"
  {
    echo '@~/.config/agents/AGENTS.md'
    echo '@~/.config/agents/workflow-policy.md'
    if [ -f "$claude_md" ]; then
      grep -vF -e '@~/.config/agents/AGENTS.md' \
              -e '@~/.config/agents/workflow-policy.md' \
              "$claude_md" || true
    fi
  } > "$tmp_md"
  mv "$tmp_md" "$claude_md"
  chmod 0644 "$claude_md"
  log "Updated $claude_md"
}

case "$WANT_AGENTS" in
  yes) install_agents ;;
  no)  log "Skipping agent files (--no-agents)" ;;
  "")
    if [ "$ASSUME_YES" -eq 1 ]; then
      log "Skipping agent files (non-interactive)"
    elif [ -t 0 ] || [ -e /dev/tty ]; then
      if ask_yes_no "Install global AGENTS.md / workflow-policy.md and add @-references to ~/.claude/CLAUDE.md?" "y"; then
        install_agents
      else
        log "Skipping agent files"
      fi
    else
      log "Skipping agent files (no TTY)"
    fi ;;
esac

# ---------------------------------------------------------------- optional model

download_default_model() {
  dst="$MODEL_DIR/$DEFAULT_MODEL_NAME"
  if [ -f "$dst" ]; then
    log "Default model already present: $dst"
    return 0
  fi
  log "Downloading $DEFAULT_MODEL_NAME (~400 MB) via llama-runner -download"
  if ! "$DEST_DIR/$BIN_NAME$exe_suffix" -download "$MODEL_DIR/$DEFAULT_MODEL_URL"; then
    warn "model download failed; run later: $BIN_NAME$exe_suffix -download $MODEL_DIR/$DEFAULT_MODEL_URL"
    return 1
  fi
  # The downloader names files by the URL basename (lowercase); rename the
  # fresh fetch onto the PascalCase name config.json refers to.
  lower_name=$(basename "$DEFAULT_MODEL_URL")
  lower_path="$MODEL_DIR/$lower_name"
  if [ -f "$lower_path" ] && [ "$lower_name" != "$DEFAULT_MODEL_NAME" ]; then
    mv "$lower_path" "$dst"
  fi
}

case "$WANT_DEFAULT_MODEL" in
  yes) download_default_model ;;
  no)  : ;;
  "")
    if [ -n "$DOWNLOAD_MODEL_URL" ]; then
      log "Downloading custom model via llama-runner -download"
      if ! "$DEST_DIR/$BIN_NAME$exe_suffix" -download "$MODEL_DIR/$DOWNLOAD_MODEL_URL"; then
        warn "custom model download failed; run later: $BIN_NAME$exe_suffix -download $MODEL_DIR/$DOWNLOAD_MODEL_URL"
      fi
    elif [ "$ASSUME_YES" -eq 1 ]; then
      :
    elif [ -t 0 ] || [ -e /dev/tty ]; then
      if ask_yes_no "Download the default small Qwen model (~400 MB) now?" "n"; then
        download_default_model
      fi
    fi ;;
esac

# ---------------------------------------------------------------- checks

if ! command -v llama-server >/dev/null 2>&1; then
  warn "'llama-server' was not found in PATH."
  printf '       Install llama.cpp, or let llama-runner do it:\n' >&2
  printf '         %s -update\n' "$DEST_DIR/$BIN_NAME$exe_suffix" >&2
fi

if ! command -v claude >/dev/null 2>&1; then
  warn "'claude' (Claude Code) was not found in PATH."
  printf '       Without it llama-runner still works in server mode (-server),\n' >&2
  printf '       but the interactive Claude Code workflow is unavailable.\n' >&2
  printf '       Install later with: npm i -g @anthropic-ai/claude-code\n' >&2
fi

if ! command -v mcp-search-server >/dev/null 2>&1; then
  warn "'mcp-search-server' was not found in PATH."
  printf '       -webui-mcp-proxy needs it; see https://github.com/SkalaSkalolaz/mcp-search-server\n' >&2
  printf '       Any other MCP server (including a remote one) can be wired via -mcp-config.\n' >&2
fi

case ":$PATH:" in
  *":$DEST_DIR:"*) ;;
  *) warn "add $DEST_DIR to PATH (e.g. export PATH=\"$DEST_DIR:\$PATH\")" ;;
esac

if [ ! -f "$MODEL_DIR/$DEFAULT_MODEL_NAME" ] && [ -z "$(ls -A "$MODEL_DIR" 2>/dev/null)" ]; then
  warn "no model found in $MODEL_DIR yet. Options:"
  printf '         %s -download %s/%s\n' "$DEST_DIR/$BIN_NAME$exe_suffix" "$MODEL_DIR" "$DEFAULT_MODEL_URL" >&2
  printf '         or point -model / config.json at your own GGUF.\n' >&2
fi

# ---------------------------------------------------------------- summary

cat >&2 <<EOF

Installed: $DEST_DIR/$BIN_NAME$exe_suffix ($VERSION)
Config:    $CONFIG_DIR/config.json
Models:    $MODEL_DIR
Cache:     $CACHE_DIR

Verify:
  $DEST_DIR/$BIN_NAME$exe_suffix -version
  $DEST_DIR/$BIN_NAME$exe_suffix -doctor

Start:
  $DEST_DIR/$BIN_NAME$exe_suffix

EOF