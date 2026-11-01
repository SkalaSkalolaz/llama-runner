package config

import (
	"sort"
)

// This module owns one responsibility: presenting the config file's
// "profiles" section as an ORDERED list. The section itself stays a
// map[string]FileConfig in FileConfig — profiles are overlays, and the
// precedence machinery in ApplyFileConfig needs random-access lookup by name.
// But a map has no meaningful order, and Go randomises its iteration, so
// anything that shows profiles to a user (the Claude Code /model menu, the
// -doctor report) must go through this file instead of ranging over the map.

// Profile is one entry of the "profiles" section, reduced to the identity the
// rest of the launcher needs: which profile it is, which model it names, and
// whether it brings its own display alias. Nothing here is resolved — paths
// keep their "~" and an empty Alias stays empty; turning those into a visible
// name is the job of the alias layer (see internal/cli).
type Profile struct {
	// Name is the profile key as written in the file (for example "coder").
	Name string
	// Model is the profile's model path as written, "" when the profile does
	// not set one and therefore inherits the root section's model.
	Model string
	// Alias is the profile's alias as written, "" when the profile sets none.
	Alias string
	// AliasSet reports whether the profile supplies alias itself. It is kept
	// separate from Alias != "" so a caller can tell "no alias in the file"
	// from an empty string that came from somewhere else.
	AliasSet bool
}

// OrderedProfiles flattens fc.Profiles into a deterministic slice, ordered the
// way the profiles are written in the config file — the order a user reads in
// their own config is the order they should see in the menu.
//
// When the file order is unavailable (a FileConfig assembled in code, or an
// older parse path that did not record it) names are sorted instead, so the
// result is at least stable across runs. A nil file, no profiles, or a JSON
// "profiles": null all yield nil: "no profiles" is not an error.
func OrderedProfiles(fc *FileConfig) []Profile {
	if fc == nil || len(fc.Profiles) == 0 {
		return nil
	}
	names := fc.ProfileOrder
	if len(names) == 0 {
		names = sortedProfileNames(fc.Profiles)
	}
	out := make([]Profile, 0, len(names))
	for _, name := range names {
		p, ok := fc.Profiles[name]
		if !ok {
			// ProfileOrder is taken from the same JSON object as the map, so a
			// missing key means a caller edited one of the two by hand.
			// Skipping it keeps the menu consistent with what can be applied.
			continue
		}
		prof := Profile{Name: name}
		if p.Model != nil {
			prof.Model = *p.Model
		}
		if p.Alias != nil {
			prof.Alias = *p.Alias
			prof.AliasSet = true
		}
		out = append(out, prof)
	}
	return out
}

// sortedProfileNames returns the profile keys in lexical order — the stable
// fallback used whenever the file's own ordering was not recorded.
func sortedProfileNames(profiles map[string]FileConfig) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
