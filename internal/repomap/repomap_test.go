package repomap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScan_GoProject(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module x\n")
	write(t, filepath.Join(dir, "main.go"), "package main\n")
	info, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Stacks) != 1 || info.Stacks[0] != "Go" {
		t.Errorf("Stacks = %v, want [Go]", info.Stacks)
	}
	if !strings.Contains(info.Prompt(), "go build") {
		t.Errorf("suggested verify missing: %s", info.Prompt())
	}
}

func TestScan_SkipsVendorAndNodeModules(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.go"), "package a\n")
	write(t, filepath.Join(dir, "vendor", "v", "v.go"), "package v\n")
	write(t, filepath.Join(dir, "node_modules", "n", "n.js"), "x\n")
	info, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := info.Prompt()
	if !strings.Contains(p, ".go=1") || strings.Contains(p, ".js") {
		t.Errorf("skipped dirs leaked into languages: %s", p)
	}
}

func TestScan_DepthLimit(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a", "b", "c", "d", "f.txt"), "x")
	info, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(info.Tree, " ")
	if !strings.Contains(joined, "a/b/") || strings.Contains(joined, "a/b/c/") {
		t.Errorf("tree depth limit broken: %v", info.Tree)
	}
}

func TestScan_TreeCap(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < maxTreeEntries+50; i++ {
		if err := os.MkdirAll(filepath.Join(dir, fmt.Sprintf("d%03d", i)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	info, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Tree) > maxTreeEntries || len(info.Tree) == 0 {
		t.Errorf("len(Tree) = %d, want 1..%d", len(info.Tree), maxTreeEntries)
	}
}

func TestScan_EmptyDirPromptEmpty(t *testing.T) {
	info, err := Scan(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if info.Prompt() != "" {
		t.Errorf("empty dir prompt = %q, want empty", info.Prompt())
	}
}

func TestPrompt_Deterministic(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module x\n")
	write(t, filepath.Join(dir, "internal", "a", "a.go"), "package a\n")
	write(t, filepath.Join(dir, "cmd", "m", "m.go"), "package main\n")
	a, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if a.Prompt() != b.Prompt() {
		t.Error("Prompt is not deterministic")
	}
}

func TestPrompt_Capped(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "main.go"), "package main\n")
	for i := 0; i < 300; i++ {
		name := fmt.Sprintf("directory-%03d-%s", i, strings.Repeat("x", 40))
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	info, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p := info.Prompt(); len(p) > promptCap {
		t.Errorf("len(Prompt) = %d, want <= %d", len(p), promptCap)
	}
}

func TestScan_MissingDir(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}
