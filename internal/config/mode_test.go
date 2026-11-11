package config

import "testing"

func TestAutoMode(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want bool
	}{
		{"nil", nil, false},
		{"empty", &Config{}, false},
		{"auto", &Config{Mode: "auto"}, true},
		{"case and spaces", &Config{Mode: "  AUTO "}, true},
		{"other value", &Config{Mode: "manual"}, false},
		{"typo is not auto", &Config{Mode: "autom"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.AutoMode(); got != tt.want {
				t.Errorf("AutoMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidMode(t *testing.T) {
	for _, ok := range []string{"", "auto", "AUTO", " auto "} {
		if !ValidMode(ok) {
			t.Errorf("ValidMode(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"autom", "manual", "1", "automatic"} {
		if ValidMode(bad) {
			t.Errorf("ValidMode(%q) = true, want false", bad)
		}
	}
}

// The overlay must carry mode, and a nil field must leave it untouched.
func TestApplyFileConfig_Mode(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Mode != "" {
		t.Fatalf("DefaultConfig().Mode = %q, want empty", cfg.Mode)
	}
	m := "auto"
	if err := ApplyFileConfig(cfg, FileConfig{Mode: &m}); err != nil {
		t.Fatal(err)
	}
	if !cfg.AutoMode() {
		t.Error("mode overlay did not reach Config")
	}
	if err := ApplyFileConfig(cfg, FileConfig{}); err != nil {
		t.Fatal(err)
	}
	if !cfg.AutoMode() {
		t.Error("a nil mode field clobbered the previous value")
	}
}
