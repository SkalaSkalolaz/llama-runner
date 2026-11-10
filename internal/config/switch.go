// Package config defines the wire contract shared by the launcher and the
// proxy for the "explicit switch to another profile" marker.
package config

// SwitchPrefix marks a /model value as "explicitly switch the local
// llama-server to this profile".
//
// Standard /model slots (Opus/Sonnet/Haiku/Fable) carry the primary model
// BARE: Claude Code uses these slots for background traffic (session
// titles, compaction, subagents) and the proxy routes such requests to
// whichever server is currently running — never restarting it.
//
// Only values that start with this prefix trigger a server restart. The
// launcher emits them in the custom /model slots (see
// cli.buildDynamicMenu); the proxy interprets them in handleMessages (see
// proxy.handleSwitch).
const SwitchPrefix = "switch-to:"
