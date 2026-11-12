package app

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

func TestRunInteractive_ExitImmediately(t *testing.T) {
	cfg := planOnlyCfg()
	var out, errb bytes.Buffer
	code := runInteractive(context.Background(), cfg, t.TempDir(),
		"http://127.0.0.1:1", nil, scriptCmdRunner{script: "true"},
		strings.NewReader("exit\n"), &out, &errb)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), i18n.T(i18n.MsgInteractiveBye)) {
		t.Errorf("no bye message: %q", out.String())
	}
}

func TestRunInteractive_EmptyLineReprompts(t *testing.T) {
	cfg := planOnlyCfg()
	var out, errb bytes.Buffer
	code := runInteractive(context.Background(), cfg, t.TempDir(),
		"http://127.0.0.1:1", nil, scriptCmdRunner{script: "true"},
		strings.NewReader("\nexit\n"), &out, &errb)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if got := strings.Count(out.String(), i18n.T(i18n.MsgInteractivePrompt)); got != 2 {
		t.Errorf("prompt count = %d, want 2 (empty line must re-prompt)", got)
	}
}

func TestRunInteractive_EOFExits(t *testing.T) {
	cfg := planOnlyCfg()
	var out, errb bytes.Buffer
	code := runInteractive(context.Background(), cfg, t.TempDir(),
		"http://127.0.0.1:1", nil, scriptCmdRunner{script: "true"},
		strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (EOF must not be an error)", code)
	}
}

func TestRunInteractive_ExecutesTask(t *testing.T) {
	srv := planUpstream(t, `{"steps":[{"id":"1","role":"coder","goal":"g","alias":"auto"}]}`)
	cfg := planOnlyCfg()
	var out, errb bytes.Buffer
	code := runInteractive(context.Background(), cfg, t.TempDir(), srv.URL,
		nil, scriptCmdRunner{script: "echo ok"},
		strings.NewReader("do it\nexit\n"), &out, &errb)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr=%q)", code, errb.String())
	}
	// Plan header and step messages should reach stdout; the
	// building-progress line should reach stderr only.
	if !strings.Contains(out.String(), "Plan") && !strings.Contains(out.String(), "План") {
		t.Errorf("no plan header in stdout: %q", out.String())
	}
}

func TestRunInteractive_FailureDoesNotEndLoop(t *testing.T) {
	// planUpstream serves a valid plan, but the step script always fails;
	// the loop must continue so a second task is accepted.
	srv := planUpstream(t, `{"steps":[{"id":"1","role":"coder","goal":"g","alias":"auto"}]}`)
	cfg := planOnlyCfg()
	var out, errb bytes.Buffer
	code := runInteractive(context.Background(), cfg, t.TempDir(), srv.URL,
		nil, scriptCmdRunner{script: "exit 3"},
		strings.NewReader("first\nsecond\nexit\n"), &out, &errb)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (loop continues after a failed task)", code)
	}
	if !strings.Contains(errb.String(), "exited with code") &&
		!strings.Contains(errb.String(), "завершилась с кодом") {
		t.Errorf("no failure message in stderr: %q", errb.String())
	}
}
