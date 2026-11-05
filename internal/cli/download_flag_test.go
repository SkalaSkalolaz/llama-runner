package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseFlagsWithHome_DownloadFlag(t *testing.T) {
	home := t.TempDir()
	var errb bytes.Buffer
	value := home + "/GGUF/https://huggingface.co/org/repo/resolve/main/model.gguf"
	cfg, err := ParseFlagsWithHome([]string{"-download", value}, &errb, home)
	if err != nil {
		t.Fatalf("parse: %v (%q)", err, errb.String())
	}
	if cfg.Download != value {
		t.Errorf("Download = %q, want %q", cfg.Download, value)
	}
	if !cfg.ModelSet {
		t.Error("a CLI -download must set ModelSet so the alias derives from the fetched GGUF")
	}
	if cfg.DownloadOverridesModel {
		t.Error("DownloadOverridesModel must be false without -model")
	}
}

func TestParseFlagsWithHome_DownloadAndModel(t *testing.T) {
	home := t.TempDir()
	var errb bytes.Buffer
	dl := home + "/GGUF/https://example.com/model.gguf"
	cfg, err := ParseFlagsWithHome([]string{"-download", dl, "-model", "~/other.gguf"}, &errb, home)
	if err != nil {
		t.Fatalf("parse: %v (%q)", err, errb.String())
	}
	if cfg.Download != dl {
		t.Errorf("Download = %q", cfg.Download)
	}
	if !cfg.DownloadOverridesModel {
		t.Error("-download together with -model must record the override for the -v warning")
	}
	// The parallel reset rule treats -download like -model (a CLI model
	// choice): an inherited per-profile parallel must not leak through.
	if cfg.Parallel != "1" {
		t.Errorf("Parallel = %q, want the reset default \"1\"", cfg.Parallel)
	}
}

func TestParseFlagsWithHome_DownloadFromProfile(t *testing.T) {
	home := t.TempDir()
	cfgPath := filepath.Join(home, "config.json")
	fc := map[string]any{
		"profiles": map[string]any{
			"ds": map[string]any{
				"download": "~/GGUF/https://huggingface.co/org/repo/resolve/main/model.gguf",
				"alias":    "ds-flash",
			},
		},
	}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	cfg, err := ParseFlagsWithHome([]string{"-config", cfgPath, "-p", "ds"}, &errb, home)
	if err != nil {
		t.Fatalf("parse: %v (%q)", err, errb.String())
	}
	if cfg.Download == "" {
		t.Fatalf("download from the profile was not applied: %+v", cfg)
	}
	if cfg.ModelSet {
		t.Error("a profile-provided download must not set ModelSet (the fetch itself does it later)")
	}
	if cfg.DownloadOverridesModel {
		t.Error("DownloadOverridesModel is a CLI-only origin flag")
	}
}
