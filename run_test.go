package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_Version(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"-version"}, &out, &errb, fakeRunner{})
	if code != 0 {
		t.Errorf("code = %d", code)
	}
	if strings.TrimSpace(out.String()) != version {
		t.Errorf("out = %q, want %q", out.String(), version)
	}
}

func TestRun_Help(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"-h"}, &out, &errb, fakeRunner{})
	if code != 0 {
		t.Errorf("code = %d", code)
	}
	if !strings.Contains(errb.String(), "llama-runner") {
		t.Errorf("usage not printed to stderr: %q", errb.String())
	}
}

func TestRun_BadPort(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"-port", "abc"}, &out, &errb, fakeRunner{})
	if code != 2 {
		t.Errorf("code = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "некорректный -port") {
		t.Errorf("err = %q", errb.String())
	}
}

func TestRun_UpdateFlag_PassesThrough(t *testing.T) {
	dir := t.TempDir()
	runner := fakeRunner{}
	var out, errb bytes.Buffer
	code := run([]string{"-update", "-llama-src", dir}, &out, &errb, runner)
	if code != 0 {
		t.Errorf("code = %d, stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "успешно обновлён") {
		t.Errorf("out = %q", out.String())
	}
}