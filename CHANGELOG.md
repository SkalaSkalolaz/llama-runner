# Changelog

All notable changes to this project are documented here. Format loosely
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
versioning follows [SemVer](https://semver.org/).

## Unreleased

### Added
- `install.sh`: one-shot installer with `curl … | sh`, embedded SHA-256
  verification, optional agent files, optional default model download.
- `.github/workflows/release.yml`: builds pre-built binaries for
  linux/darwin/windows × amd64/arm64 and publishes a GitHub release
  with `sha256sums.txt`.
- `templates/agents/`: neutral AGENTS.md and workflow-policy.md in EN and RU.
- `templates/claude/CLAUDE.md`: snippet that pulls the global agent files
  into `~/.claude/CLAUDE.md`.
- `templates/chat-templates/qwen-fixed-chat-template.jinja`: fallback chat
  template installed into the model directory.
- `packaging/`: systemd unit, man page, shell completions.
- `scripts/update-embedded-hashes.sh`: CI helper that keeps `install.sh`'s
  hash table in sync with the released artifacts.

## v1.8.0

The state of the project before this changelog was introduced. See git
history and GitHub releases for details.