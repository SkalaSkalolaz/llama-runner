package doctor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

// pinEnglish pins BOTH locale variables. IsRussianLocale skips LC_ALL="C"
// and falls through to LANG, so pinning LC_ALL alone leaves an English
// assertion at the mercy of the developer's ambient LANG — the same reason
// internal/app and internal/download pin both in their TestMain.
func pinEnglish(t *testing.T) {
	t.Helper()
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "C")
}

func TestRunDoctor_ReportsAutoModeAndBudget(t *testing.T) {
	pinEnglish(t)
	cfg, home := doctorCfg(t)
	cfg.OpenAIKeyless = true // the fixture configures a provider; keep it valid
	cfg.Mode = "auto"
	cfg.Profiles = map[string]*config.Config{
		"auto": {Alias: "auto", CtxSize: "32768", Parallel: "1"},
		"big":  {Alias: "big", CtxSize: "131072", Parallel: "1"},
		"wave": {Alias: "wave", CtxSize: "262144", Parallel: "2"},
	}
	var stdout, stderr bytes.Buffer
	if code := RunDoctor(cfg, home, fakeRunner{}, "9.9.9", &stdout, &stderr); code != 0 {
		t.Fatalf("code = %d\nstdout=%s\nstderr=%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "auto (planner)") {
		t.Errorf("mode line missing: %s", out)
	}
	// 131072 comes from big: wave has the larger -c, but parallel=2 splits
	// it across slots, so it must not win the ceiling.
	if !strings.Contains(out, "auto context:") || !strings.Contains(out, "131072") {
		t.Errorf("auto context ceiling missing: %s", out)
	}
}

func TestRunDoctor_NoAutoLinesWithoutMode(t *testing.T) {
	pinEnglish(t)
	cfg, home := doctorCfg(t)
	cfg.OpenAIKeyless = true
	var stdout, stderr bytes.Buffer
	_ = RunDoctor(cfg, home, fakeRunner{}, "9.9.9", &stdout, &stderr)
	out := stdout.String()
	if strings.Contains(out, "auto (planner)") || strings.Contains(out, "auto context:") {
		t.Errorf("auto lines printed for an ordinary config: %s", out)
	}
}
