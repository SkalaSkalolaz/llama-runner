package i18n

import (
	"os"
	"testing"
)

// TestIsRussianLocale fixes the documented precedence: LC_ALL wins, otherwise
// LANG decides, and LANGUAGE is deliberately ignored (it selects translations
// of gettext messages, not the console locale llama-runner reports).
func TestIsRussianLocale(t *testing.T) {
	origLang := os.Getenv("LANG")
	origLcAll := os.Getenv("LC_ALL")
	origLangEnv := os.Getenv("LANGUAGE")
	defer func() {
		os.Setenv("LANG", origLang)
		os.Setenv("LC_ALL", origLcAll)
		os.Setenv("LANGUAGE", origLangEnv)
	}()

	tests := []struct {
		name     string
		lcAll    string
		lang     string
		language string
		want     bool
	}{
		{"empty all", "", "", "", false},
		{"LANG ru", "ru_RU.UTF-8", "", "", true},
		{"LC_ALL ru", "ru_RU.UTF-8", "en_US.UTF-8", "ru_RU:ru", true},
		{"LANG=en, LANGUAGE=ru (ignored)", "en_US.UTF-8", "", "ru_RU:ru", false},
		{"LANGUAGE=ru only (not locale)", "", "", "ru_RU:ru", false},
		{"LC_ALL=en, LANG=ru", "en_US.UTF-8", "ru_RU.UTF-8", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("LC_ALL", tt.lcAll)
			os.Setenv("LANG", tt.lang)
			os.Setenv("LANGUAGE", tt.language)
			if got := IsRussianLocale(); got != tt.want {
				t.Errorf("IsRussianLocale() = %v, want %v", got, tt.want)
			}
		})
	}
}
