package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/planner"
)

func TestSummary_SingleParagraph(t *testing.T) {
	if got := summary("one short line\n"); got != "one short line" {
		t.Errorf("summary = %q", got)
	}
}

func TestSummary_LastParagraphPreferred(t *testing.T) {
	p1 := strings.Repeat("a", 50)
	p2 := strings.Repeat("b", 50)
	in := p1 + "\n\n" + p2 + "\n\nDone."
	if got := summary(in); got != p2 {
		t.Errorf("summary = %q, want the last CONTENTFUL paragraph", got)
	}
}

func TestSummary_LastParagraphWhenAllShort(t *testing.T) {
	if got := summary("one\n\ntwo\n\nDone."); got != "Done." {
		t.Errorf("summary = %q, want the last paragraph", got)
	}
}

func TestSummary_Empty(t *testing.T) {
	for _, in := range []string{"", " \n\n\t\n"} {
		if got := summary(in); got != "" {
			t.Errorf("summary(%q) = %q, want empty", in, got)
		}
	}
}

func TestSummary_Truncated(t *testing.T) {
	got := summary(strings.Repeat("x", summaryCap+50))
	if r := []rune(got); len(r) != summaryCap+1 || !strings.HasSuffix(got, "…") {
		t.Errorf("len = %d, suffix missing: %q", len(r), got)
	}
}

func TestSummary_CollapsesNewlines(t *testing.T) {
	got := summary("line one\nline two\nline three")
	if got != "line one line two line three" {
		t.Errorf("summary = %q", got)
	}
}

func TestPickSummary_PrefersStdout(t *testing.T) {
	if got := pickSummary("out", "err"); got != "out" {
		t.Errorf("pickSummary = %q", got)
	}
}

func TestPickSummary_FallsBackToStderr(t *testing.T) {
	if got := pickSummary(" \n", "err text"); got != "err text" {
		t.Errorf("pickSummary = %q", got)
	}
}

func writeTemp(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadArtifact_TextFile(t *testing.T) {
	dir := t.TempDir()
	p := writeTemp(t, dir, "a.txt", []byte("hello\nworld\n"))
	if got := readArtifact(p, dir); got != "hello\nworld\n" {
		t.Errorf("readArtifact = %q", got)
	}
}

func TestReadArtifact_RelativePath(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "f.txt", []byte("content"))
	if got := readArtifact("f.txt", dir); got != "content" {
		t.Errorf("readArtifact = %q", got)
	}
}

func TestReadArtifact_MissingFile(t *testing.T) {
	if got := readArtifact("absent.txt", t.TempDir()); got != "" {
		t.Errorf("readArtifact = %q, want empty", got)
	}
}

func TestReadArtifact_Directory(t *testing.T) {
	if got := readArtifact(t.TempDir(), ""); got != "" {
		t.Errorf("readArtifact(dir) = %q, want empty", got)
	}
}

func TestReadArtifact_BinarySkipped(t *testing.T) {
	dir := t.TempDir()
	p := writeTemp(t, dir, "bin", []byte{'a', 0, 'b'})
	if got := readArtifact(p, dir); got != "" {
		t.Errorf("binary artifact embedded: %q", got)
	}
}

func TestReadArtifact_Truncated(t *testing.T) {
	dir := t.TempDir()
	p := writeTemp(t, dir, "big.txt", []byte(strings.Repeat("x", artifactContentCap+10)))
	got := readArtifact(p, dir)
	if r := []rune(got); len(r) != artifactContentCap+1 || !strings.HasSuffix(got, "…") {
		t.Errorf("len = %d, want cap+ellipsis", len(r))
	}
}

func TestReadArtifact_ProseArtifact(t *testing.T) {
	if got := readArtifact("findings report", t.TempDir()); got != "" {
		t.Errorf("prose artifact read: %q", got)
	}
}

func TestBuildPriorBlock_ProseArtifact(t *testing.T) {
	priors := []StepResult{{
		Step:  planner.Step{ID: "1", Role: "reviewer", Goal: "g", Artifact: "findings report"},
		Alias: "auto", OK: true, Output: "s",
	}}
	b := buildPriorBlock(priors, t.TempDir())
	if !strings.Contains(b, "NOT a file path") || !strings.Contains(b, "findings report") {
		t.Errorf("prose marker missing: %q", b)
	}
	if strings.Contains(b, "begin artifact") {
		t.Errorf("prose artifact must not embed content: %q", b)
	}
}

func TestBuildPriorBlock_PathArtifactNotMarked(t *testing.T) {
	dir := t.TempDir()
	p := writeTemp(t, dir, "a.txt", []byte("x"))
	priors := []StepResult{{
		Step:  planner.Step{ID: "1", Role: "coder", Goal: "g", Artifact: p},
		Alias: "auto", OK: true,
	}}
	b := buildPriorBlock(priors, dir)
	if !strings.Contains(b, "artifact: "+p) {
		t.Errorf("path artifact missing: %q", b)
	}
	if strings.Contains(b, "NOT a file path") {
		t.Errorf("path artifact wrongly marked as prose: %q", b)
	}
}
