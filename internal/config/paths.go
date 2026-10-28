package config

import (
	"path/filepath"
	"strings"
)

// ExpandHome replaces a leading "~" with the given home directory:
// "~" becomes home itself, "~/foo" becomes home/foo, and anything else
// (relative or absolute paths, "~user" forms, embedded tildes) is returned
// unchanged. The home value is injected by the caller rather than looked up
// here, which keeps the function pure and testable. It is shared by the config
// loader, the run loop and the doctor report, so "~"-relative values coming
// from flags and from the config file are expanded identically.
func ExpandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
