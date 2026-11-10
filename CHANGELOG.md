# Changelog

All notable changes to this project are documented here. Format loosely
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
versioning follows [SemVer](https://semver.org/).

## v1.9.0

### Added
- Keyless external provider: `-key none`, `-key ""`, and `"openaiKey": "none"` in the config file make requests to the provider carry **no** `Authorization` header (field `OpenAIKeyless`). `-doctor` reports `not required (no key sent)` for this mode.

### Changed
- `/model` menu is dynamic when the config file declares profiles: profile switching uses `switch-to:<alias>` in the model value, with the startup profile on Opus and other profiles on Sonnet/Fable/Custom. **Known cosmetic issue:** the `switch-to:` prefix is visible in standard slots; a cleaner layout is planned for 1.9.1.
- Per-profile `parallel` and `temp` are honoured; a CLI `-model` resets an inherited `parallel` to `"1"` unless `-parallel` is given explicitly.
- `fit`, `temp`, `reasoning`, `reasoningBudget`, `cacheTypeK`, `cacheTypeV`, `logVerbosity`, `specType`, `specDraftModel`, `specDraftNMax`, `specDraftNGL` are passed through to llama-server and are all reflected in the flags tables and Config file field lists.
- `config.example.json` and the `install.sh` heredoc JSON now include `"temp": "0.2"`, `"fit": "on"`, `"reasoning": "on"`, `"reasoningBudget": "2048"` in every profile.
- Version bump to `1.9.0`.

### Fixed
- Documentation drift: `-temp`, `-top-p`, `-cache-type-k/v`, `-fit`, `-lv`, `-reasoning`, `-reasoning-budget`, `-spec-*` were missing from the flags tables; `temp`, `topP`, `cacheTypeK/V`, `fit`, `logVerbosity`, `reasoning`, `reasoningBudget`, `spec*` were missing from the Config fields lists. Restored in both READMEs.
- `install.sh` example header used `--version v1.8.0`; now generic `vX.Y.Z`.

## v1.8.0

The state of the project before this changelog was introduced. See git
history and GitHub releases for details.