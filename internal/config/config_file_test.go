package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		flag string
		want string
		ok   bool
	}{
		{"none", []string{}, "config", "", false},
		{"-config value", []string{"-config", "/x"}, "config", "/x", true},
		{"--config value", []string{"--config", "/x"}, "config", "/x", true},
		{"-config=value", []string{"-config=/x"}, "config", "/x", true},
		{"--config=value", []string{"--config=/x"}, "config", "/x", true},
		{"-config empty", []string{"-config="}, "config", "", true},
		{"-config at end", []string{"-config"}, "config", "", true},
		{"-other then -config", []string{"-other", "v", "-config", "/x"}, "config", "/x", true},
		{"-- terminator", []string{"--", "-config", "/x"}, "config", "", false},
		{"positional stops", []string{"pos", "-config", "/x"}, "config", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FindFlag(tt.args, tt.flag)
			if ok != tt.ok || got != tt.want {
				t.Errorf("FindFlag(%v, %q) = (%q, %v), want (%q, %v)",
					tt.args, tt.flag, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestLoadConfigFile_NotExist(t *testing.T) {
	fc, err := LoadConfigFile(filepath.Join(t.TempDir(), "missing.json"), t.TempDir())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if fc != nil {
		t.Errorf("fc = %v, want nil", fc)
	}
}

func TestLoadConfigFile_EmptyPath(t *testing.T) {
	fc, err := LoadConfigFile("", t.TempDir())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if fc != nil {
		t.Errorf("fc = %v, want nil", fc)
	}
}

func TestLoadConfigFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{"host": "0.0.0.0", "port": "9090", "jinja": false}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	fc, err := LoadConfigFile(path, dir)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if fc == nil {
		t.Fatal("fc = nil")
	}
	if fc.Host == nil || *fc.Host != "0.0.0.0" {
		t.Errorf("Host = %v", fc.Host)
	}
	if fc.Port == nil || *fc.Port != "9090" {
		t.Errorf("Port = %v", fc.Port)
	}
	if fc.Jinja == nil || *fc.Jinja != false {
		t.Errorf("Jinja = %v", fc.Jinja)
	}
}

func TestLoadConfigFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfigFile(path, dir); err == nil {
		t.Fatal("expected error")
	}
}
