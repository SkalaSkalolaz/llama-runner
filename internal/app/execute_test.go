package app

import (
	"bytes"
	"context"
	"os/exec"
	"testing"
)

// scriptCmdRunner executes every command as a fixed shell script, so the
// executor can be exercised without a real claude binary.
type scriptCmdRunner struct{ script string }

func (r scriptCmdRunner) Command(name string, args ...string) *exec.Cmd {
	return exec.Command("sh", "-c", r.script)
}

func (r scriptCmdRunner) LookPath(file string) (string, error) { return "/fake/claude", nil }

func TestRunExecute_EndToEnd(t *testing.T) {
	srv := planUpstream(t, `{"steps":[{"id":"1","role":"coder","goal":"g","alias":"auto"}]}`)
	cfg := planOnlyCfg()
	var out, errb bytes.Buffer
	code := runExecute(context.Background(), cfg, t.TempDir(), srv.URL, nil,
		scriptCmdRunner{script: "echo ok"}, &out, &errb)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, errb.String())
	}
	if errb.Len() != 0 {
		t.Errorf("unexpected stderr: %s", errb.String())
	}
}

func TestRunExecute_PlannerDownIsExitOne(t *testing.T) {
	cfg := planOnlyCfg()
	var out, errb bytes.Buffer
	code := runExecute(context.Background(), cfg, t.TempDir(), "http://127.0.0.1:1", nil,
		scriptCmdRunner{script: "true"}, &out, &errb)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if errb.Len() == 0 {
		t.Error("no diagnostic on stderr")
	}
}