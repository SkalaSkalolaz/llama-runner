package app

import (
	"os"
	"testing"
)

// TestMain pins the default console locale to English for the test binary
// so tests without an explicit LC_ALL assert against the English catalog
// regardless of the developer's ambient LANG. Tests that want Russian pin
// LC_ALL=ru themselves (LC_ALL wins over LANG in IsRussianLocale).
func TestMain(m *testing.M) {
	os.Setenv("LC_ALL", "C")
	os.Setenv("LANG", "C")
	os.Exit(m.Run())
}
