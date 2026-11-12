package orchestrator

import (
	"os"
	"testing"
)

// TestMain pins the console locale to English for the whole test binary:
// assertions match English catalog templates, and the developer's ambient
// LANG must not flip them (LC_ALL=C alone is not enough — IsRussianLocale
// falls through to LANG). Same pattern as planner, app and download.
func TestMain(m *testing.M) {
	os.Setenv("LC_ALL", "C")
	os.Setenv("LANG", "C")
	os.Exit(m.Run())
}