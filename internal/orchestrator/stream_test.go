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
	if got := buf.String(); got != "[1] hello\n[1] …ed lines missing: %q", got)
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
	if strings.Contains(out.String(), "hidden-without-flag") {
		t.Errorf("step output leaked to stdout without -stream-output: %q", out.String())
	}
}