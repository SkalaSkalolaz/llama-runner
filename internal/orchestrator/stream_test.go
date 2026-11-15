package orchestrator

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/planner"
)

func TestStepStreamWriter_LinesAndFlush(t *testing.T) {
	var buf bytes.Buffer
	s := newStepStreamer(&buf)
	w := s.writer("1")

	if _, err := w.Write([]byte("hel")); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("partial line emitted early: %q", buf.String())
	}

	_, _ = w.Write([]byte("lo\nworld\n"))
	if got := buf.String(); got != "[1] hello\n[1] world\n" {
		t.Errorf("lines = %q, want %q", got, "[1] hello\n[1] world\n")
	}
}

func TestRun_NoStreamingByDefault(t *testing.T) {
	var out bytes.Buffer
	opts := baseOpts(t, "echo hidden-without-flag")
	opts.Stdout = &out
	plan := planner.Plan{Steps: []planner.Step{stepNoDeps("1", "auto")}}
	res := Run(context.Background(), plan, opts)
	if res.Failed {
		t.Fatalf("run failed: %+v", res.Steps)
	}
	got := out.String()
	// Summary всегда есть (стандартное поведение Run)
	if !strings.Contains(got, "Step 1/1") {
		t.Errorf("summary missing: %q", got)
	}
	// Но построчный стриминг с префиксом [1] — только со стримингом
	if strings.Contains(got, "[1] hidden-without-flag") {
		t.Errorf("streaming lines leaked without -stream-output: %q", got)
	}
}
