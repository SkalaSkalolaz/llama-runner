package app

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func fixedClockWriter(buf *bytes.Buffer) *timePrefixWriter {
	w := newTimePrefixWriter(buf)
	w.now = func() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.Local) }
	return w
}

func TestTimePrefixWriter_SingleLine(t *testing.T) {
	var buf bytes.Buffer
	w := fixedClockWriter(&buf)
	if n, err := w.Write([]byte("hello\n")); n != 6 || err != nil {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if got := buf.String(); got != "12:00:00 hello\n" {
		t.Errorf("out = %q", got)
	}
}

func TestTimePrefixWriter_MultipleLinesInOneWrite(t *testing.T) {
	var buf bytes.Buffer
	w := fixedClockWriter(&buf)
	_, _ = w.Write([]byte("a\nb\n"))
	if got := buf.String(); got != "12:00:00 a\n12:00:00 b\n" {
		t.Errorf("out = %q", got)
	}
}

func TestTimePrefixWriter_PartialThenComplete(t *testing.T) {
	var buf bytes.Buffer
	w := fixedClockWriter(&buf)
	_, _ = w.Write([]byte("hel"))
	if buf.Len() != 0 {
		t.Fatalf("partial line emitted early: %q", buf.String())
	}
	_, _ = w.Write([]byte("lo\n"))
	if got := buf.String(); got != "12:00:00 hello\n" {
		t.Errorf("out = %q", got)
	}
}

func TestTimePrefixWriter_FlushPartial(t *testing.T) {
	var buf bytes.Buffer
	w := fixedClockWriter(&buf)
	_, _ = w.Write([]byte("tail"))
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "12:00:00 tail" {
		t.Errorf("out = %q", got)
	}
	if err := w.Flush(); err != nil || buf.String() != "12:00:00 tail" {
		t.Errorf("second Flush changed output: %q, %v", buf.String(), err)
	}
}

func TestTimePrefixWriter_EmptyWrite(t *testing.T) {
	var buf bytes.Buffer
	w := fixedClockWriter(&buf)
	if n, err := w.Write(nil); n != 0 || err != nil {
		t.Fatalf("Write(nil) = %d, %v", n, err)
	}
	if err := w.Flush(); err != nil || buf.Len() != 0 {
		t.Errorf("empty write produced output: %q, %v", buf.String(), err)
	}
}

func TestTimePrefixWriter_PrefixPerLine(t *testing.T) {
	var buf bytes.Buffer
	w := fixedClockWriter(&buf)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.Local)
	w.now = func() time.Time { return now }
	_, _ = w.Write([]byte("a\n"))
	now = now.Add(time.Second)
	_, _ = w.Write([]byte("b\n"))
	if got := buf.String(); got != "12:00:00 a\n12:00:01 b\n" {
		t.Errorf("out = %q", got)
	}
}

type errWriter struct{ err error }

func (e errWriter) Write([]byte) (int, error) { return 0, e.err }

func TestTimePrefixWriter_DstError(t *testing.T) {
	boom := errors.New("boom")
	w := newTimePrefixWriter(errWriter{err: boom})
	w.now = time.Now
	if _, err := w.Write([]byte("x\n")); !errors.Is(err, boom) {
		t.Errorf("Write err = %v, want boom", err)
	}
	w2 := newTimePrefixWriter(errWriter{err: boom})
	w2.now = time.Now
	_, _ = w2.Write([]byte("x")) // buffered, no error yet
	if err := w2.Flush(); !errors.Is(err, boom) {
		t.Errorf("Flush err = %v, want boom", err)
	}
}
