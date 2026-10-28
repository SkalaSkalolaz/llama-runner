package cli

import (
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// --- DeriveAliasFromModel ---

func TestDeriveAliasFromModel(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"gpt-oss with quant tag", "~/GGUF/gpt-oss-20b-Q5_K_M.gguf", "gpt-oss-20b-Q5_K_M"},
		{"qwen with quant tag", "~/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf", "Qwen3.8-35B-A3B-Q4_K_M"},
		// Only the last path component matters, so an unexpanded "~" value
		// and an already-absolute path give the same name.
		{"tilde path", "~/GGUF/m.gguf", "m"},
		{"absolute path", "/home/u/models/m.gguf", "m"},
		{"expanded tilde", config.ExpandHome("~/GGUF/m.gguf", "/home/u"), "m"},
		{"no extension kept as is", "/m/qwen3-coder", "qwen3-coder"},
		{"uppercase extension stripped", "/m/MODEL.GGUF", "MODEL"},
		{"sharded file name kept whole",
			"~/GGUF/Qwen3.8-Flash-Next-GSQ-RCO-IQ1_M-00001-of-00002.gguf",
			"Qwen3.8-Flash-Next-GSQ-RCO-IQ1_M-00001-of-00002"},
		{"empty path", "", ""},
		{"bare extension has no stem", "/m/.gguf", ""},
		{"directory-like input", "/m/.", ""},
		{"trailing slash tolerated", "/m/m.gguf/", "m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeriveAliasFromModel(tt.path); got != tt.want {
				t.Errorf("DeriveAliasFromModel(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// --- ResolveAlias precedence ---

func TestResolveAlias_Precedence(t *testing.T) {
	// Mirrors the command table: which source wins for the visible name.
	tests := []struct {
		name    string
		cfg     *config.Config
		want    string
		wantSrc string
	}{
		{
			name: "-alias beats everything",
			cfg: &config.Config{Alias: "Z", AliasSet: true, AliasFromCLI: true,
				ModelSet: true, Model: "/m/gpt-oss-20b-Q5_K_M.gguf"},
			want: "Z", wantSrc: i18n.MsgAliasSrcCLI,
		},
		{
			name: "-model overrides a stale file alias (the reported bug)",
			cfg: &config.Config{Alias: "qwen3.8-35b", AliasSet: true,
				ModelSet: true,
				Model:    "~/GGUF/gpt-oss-20b-Q5_K_M.gguf"},
			want: "gpt-oss-20b-Q5_K_M", wantSrc: i18n.MsgAliasSrcDerived,
		},
		{
			name: "-p coder keeps the profile alias",
			cfg: &config.Config{Alias: "qwen3.8-coder", AliasSet: true,
				AliasFromProfile: true},
			want: "qwen3.8-coder", wantSrc: i18n.MsgAliasSrcProfile,
		},
		{
			name: "root-file alias without a profile",
			cfg:  &config.Config{Alias: "qwen3.8-35b", AliasSet: true},
			want: "qwen3.8-35b", wantSrc: i18n.MsgAliasSrcFile,
		},
		{
			name: "nothing configured",
			cfg:  &config.Config{},
			want: "local", wantSrc: i18n.MsgAliasSrcDefault,
		},
		{
			name: "-model with no usable stem falls back, never empties",
			cfg:  &config.Config{ModelSet: true, Model: ""},
			want: "local", wantSrc: i18n.MsgAliasSrcDefault,
		},
		{
			name: "-model with no stem still keeps the file alias",
			cfg: &config.Config{ModelSet: true, Model: "",
				Alias: "qwen3.8-35b", AliasSet: true},
			want: "qwen3.8-35b", wantSrc: i18n.MsgAliasSrcFile,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alias, src := ResolveAlias(tt.cfg)
			if alias != tt.want {
				t.Errorf("alias = %q, want %q", alias, tt.want)
			}
			if src != tt.wantSrc {
				t.Errorf("origin = %q, want %q", src, tt.wantSrc)
			}
			if alias == "" {
				t.Error("alias must never be empty — llama-server would get no name")
			}
		})
	}
}

// ResolveAlias is pure: it must not write anything back into cfg, otherwise
// -doctor (which resolves independently of run) could observe a mutated config.
func TestResolveAlias_DoesNotMutateConfig(t *testing.T) {
	cfg := &config.Config{Alias: "qwen3.8-35b", AliasSet: true, ModelSet: true,
		Model: "/m/gpt-oss-20b-Q5_K_M.gguf"}
	ResolveAlias(cfg)
	if cfg.Alias != "qwen3.8-35b" {
		t.Errorf("cfg.Alias = %q, want untouched qwen3.8-35b", cfg.Alias)
	}
}
