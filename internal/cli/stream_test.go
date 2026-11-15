package cli

import (
	"bytes"
	"testing"
)

func TestParseFlags_StreamOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var buf bytes.Buffer
	cfg, err := ParseFlags([]string{}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StreamOutput {
		t.Error("StreamOutput should be false by default")
	}
	cfg, err = ParseFlags([]string{"-stream-output"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.StreamOutput {
		t.Error("StreamOutput should be true with -stream-output")
	}
}
