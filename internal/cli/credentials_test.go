package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// homeWithCredentials creates a home directory holding a credentials file with
// the given content (mode 0600, so no permission warning is expected).
func homeWithCredentials(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "llama-runner")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "credentials"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestResolveOpenAIKey_FlagWins(t *testing.T) {
	var stderr bytes.Buffer
	got, keyless, err := ResolveOpenAIKey("from-flag", true, func(string) string {
		return "from-env"
	}, t.TempDir(), &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-flag" {
		t.Errorf("got %q, want from-flag", got)
	}
	if keyless {
		t.Error("keyless = true, want false when a real key was given")
	}
}

func TestResolveOpenAIKey_EnvPrecedence(t *testing.T) {
	var stderr bytes.Buffer
	env := func(k string) string {
		switch k {
		case "LLAMA_RUNNER_OPENAI_KEY":
			return "runner-key"
		case "OPENAI_API_KEY":
			return "generic-key"
		}
		return ""
	}
	got, keyless, _ := ResolveOpenAIKey("", false, env, t.TempDir(), &stderr)
	if got != "runner-key" {
		t.Errorf("got %q, want runner-key (LLAMA_RUNNER_OPENAI_KEY должен быть выше OPENAI_API_KEY)", got)
	}
	if keyless {
		t.Error("keyless = true, want false")
	}
}

func TestResolveOpenAIKey_GenericEnv(t *testing.T) {
	var stderr bytes.Buffer
	env := func(k string) string {
		if k == "OPENAI_API_KEY" {
			return "generic-key"
		}
		return ""
	}
	got, _, _ := ResolveOpenAIKey("", false, env, t.TempDir(), &stderr)
	if got != "generic-key" {
		t.Errorf("got %q", got)
	}
}

func TestResolveOpenAIKey_FromFile(t *testing.T) {
	home := homeWithCredentials(t, "# comment\nOPENAI_KEY=\"file-key\"\nother=ignored\n")

	var stderr bytes.Buffer
	got, keyless, err := ResolveOpenAIKey("", false, func(string) string { return "" }, home, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if got != "file-key" {
		t.Errorf("got %q, want file-key; stderr=%q", got, stderr.String())
	}
	if keyless {
		t.Error("keyless = true, want false")
	}
	if stderr.Len() != 0 {
		t.Errorf("unexpected warning: %q", stderr.String())
	}
}

func TestResolveOpenAIKey_FileWarnsAboutPerms(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "llama-runner")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credentials")
	if err := os.WriteFile(path, []byte("OPENAI_KEY=k\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	got, _, _ := ResolveOpenAIKey("", false, func(string) string { return "" }, home, &stderr)
	if got != "k" {
		t.Errorf("got %q, want k", got)
	}
	if !strings.Contains(stderr.String(), "0600") {
		t.Errorf("ожидалось предупреждение про 0600, stderr=%q", stderr.String())
	}
}

func TestResolveOpenAIKey_NoSources(t *testing.T) {
	var stderr bytes.Buffer
	got, keyless, err := ResolveOpenAIKey("", false, func(string) string { return "" }, t.TempDir(), &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
	// No sources is NOT the same as "no key wanted": keyless stays false so
	// validation still reports a key as missing in hybrid mode.
	if keyless {
		t.Error("keyless = true, want false when nothing provided anything")
	}
}

func TestResolveOpenAIKey_SentinelBeatsEnvAndFile(t *testing.T) {
	// The whole point of -key none: on a machine that HAS a key everywhere,
	// the explicit decision wins and no source is consulted.
	home := homeWithCredentials(t, "OPENAI_KEY=file-key\n")
	var stderr bytes.Buffer
	got, keyless, err := ResolveOpenAIKey(NoKeySentinel, true, func(string) string {
		return "env-key"
	}, home, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("got %q, want empty key", got)
	}
	if !keyless {
		t.Error("keyless = false, want true")
	}
	if stderr.Len() != 0 {
		t.Errorf("unexpected output: %q", stderr.String())
	}
}

func TestResolveOpenAIKey_SentinelIsCaseAndSpaceTolerant(t *testing.T) {
	for _, v := range []string{"none", "None", "NONE", "  none  "} {
		var stderr bytes.Buffer
		got, keyless, _ := ResolveOpenAIKey(v, true, func(string) string { return "env-key" }, t.TempDir(), &stderr)
		if !keyless || got != "" {
			t.Errorf("%q: got (%q, %v), want (\"\", true)", v, got, keyless)
		}
	}
}

// A config file's "openaiKey": "none" arrives exactly like the flag value, so
// keyless can be set in a profile.
func TestResolveOpenAIKey_SentinelFromConfigFileValue(t *testing.T) {
	var stderr bytes.Buffer
	_, keyless, _ := ResolveOpenAIKey("none", false, func(string) string { return "env-key" }, t.TempDir(), &stderr)
	if !keyless {
		t.Error("keyless = false, want true for a file-provided sentinel")
	}
}

func TestResolveOpenAIKey_EmptyFlagMeansKeyless(t *testing.T) {
	// -key "" (flag present, empty value): no authentication, and the env is
	// NOT consulted — this is the intended behaviour change.
	home := homeWithCredentials(t, "OPENAI_KEY=file-key\n")
	var stderr bytes.Buffer
	got, keyless, _ := ResolveOpenAIKey("", true, func(string) string { return "env-key" }, home, &stderr)
	if got != "" || !keyless {
		t.Errorf("got (%q, %v), want (\"\", true)", got, keyless)
	}
}

func TestResolveOpenAIKey_NoFlagAtAllStillSearchesSources(t *testing.T) {
	// The other half of the distinction: omitting -key must keep meaning
	// "look elsewhere", not "no key".
	var stderr bytes.Buffer
	got, keyless, _ := ResolveOpenAIKey("", false, func(string) string { return "env-key" }, t.TempDir(), &stderr)
	if got != "env-key" || keyless {
		t.Errorf("got (%q, %v), want (env-key, false)", got, keyless)
	}
}

func TestResolveOpenAIKey_EnvSentinelIsUsedAsAKey(t *testing.T) {
	// Environment values are stores, not intent: a value that reads "none" is
	// passed through as the key it claims to be.
	var stderr bytes.Buffer
	got, keyless, _ := ResolveOpenAIKey("", false, func(k string) string {
		if k == "OPENAI_API_KEY" {
			return "none"
		}
		return ""
	}, t.TempDir(), &stderr)
	if got != "none" || keyless {
		t.Errorf("got (%q, %v), want (none, false)", got, keyless)
	}
}
