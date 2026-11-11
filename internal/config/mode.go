package config

import "strings"

// ModeAuto is the value of the config-file field "mode" that selects the
// autonomous orchestrator: the profile it is declared on is the planner,
// the rest of the catalog are the workers it dispatches to.
//
// The mode is a property of a PROFILE, not of the run and not of a name:
// a profile called "auto" without this field is an ordinary profile, and a
// profile called anything else WITH this field is a planner. It is never
// inherited from the root section into the profiles (see
// cli.ParseFlagsWithHome), so declaring the root as a planner does not turn
// every worker into a planner too.
const ModeAuto = "auto"

// AutoMode reports whether cfg selects the autonomous orchestrator mode.
// The comparison is case- and space-tolerant because the value is typed by
// hand into a JSON file. Nil-safe: a missing config is not auto mode.
func (c *Config) AutoMode() bool {
	return c != nil && strings.EqualFold(strings.TrimSpace(c.Mode), ModeAuto)
}

// ValidMode reports whether mode is a value the launcher understands:
// empty (the ordinary single-model mode) or ModeAuto. Anything else is a
// typo the user must hear about — silently ignoring "autom" would look
// exactly like a working auto mode that never orchestrates anything.
func ValidMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", ModeAuto:
		return true
	}
	return false
}
