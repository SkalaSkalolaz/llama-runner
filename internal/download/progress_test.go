package download

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

// newFakeClockProgress builds a progressWriter driven by a manual clock so
// throttling can be tested without sleeping.
func newFakeClockProgress(out *bytes.Buffer, total int64) (*progressWriter, *time.Time) {
	base := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	now := base
	p := newProgressWriter(out, "m.gguf", total, base)
	p.now = func() time.Time { return now }
	return p, &now
}

func TestProgressWriter_FirstWriteRendersImmediately(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	var buf bytes.Buffer
	p, _ := newFakeClockProgress(&buf, 1000)
	if n, err := p.Write(make([]byte, 100)); n != 100 || err != nil {
		t.Fatalf("Write = %d, %v", n, err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "\r") {
		t.Errorf("update must start with \\r: %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Errorf("intermediate updates must not contain \\n: %q", got)
	}
	if !strings.Contains(got, "downloading m.gguf") || !strings.Contains(got, "10%") {
		t.Errorf("unexpected line: %q", got)
	}
}

func TestProgressWriter_ThrottledToOncePerSecond(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	var buf bytes.Buffer
	p, now := newFakeClockProgress(&buf, 1000)
	p.Write(make([]byte, 100))
	after := buf.String()

	// Many writes within the same second: nothing new is rendered.
	for i := 0; i < 50; i++ {
		*now = now.Add(10 * time.Millisecond)
		p.Write(make([]byte, 10))
	}
	if buf.String() != after {
		t.Error("writes within one second must not produce output")
	}

	// Past the interval: a new update appears with the accumulated bytes.
	*now = now.Add(1100 * time.Millisecond)
	p.Write(make([]byte, 200))
	if buf.String() == after {
		t.Fatal("no update after the interval elapsed")
	}
	if !strings.Contains(buf.String(), "80%") { // 800 of 1000
		t.Errorf("expected 80%% in %q", buf.String())
	}
	if strings.Count(buf.String(), "\r") != 2 {
		t.Errorf("expected exactly two \\r updates, got %d in %q", strings.Count(buf.String(), "\r"), buf.String())
	}
}

func TestProgressWriter_DoneSuccessLine(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	var buf bytes.Buffer
	p, now := newFakeClockProgress(&buf, 1000)
	p.Write(make([]byte, 500))
	*now = now.Add(2 * time.Second)
	p.Write(make([]byte, 500))
	buf.Reset()

	p.Done(1000, false, nil)
	got := buf.String()
	if !strings.HasPrefix(got, "\r\033[K") {
		t.Errorf("Done must clear a leftover partial line first: %q", got)
	}
	if !strings.Contains(got, "✓ m.gguf (1000 B)") || !strings.HasSuffix(got, "\n") {
		t.Errorf("Done line = %q", got)
	}
}

func TestProgressWriter_DoneWithoutUpdatesKeepsPlainLine(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	var buf bytes.Buffer
	p, _ := newFakeClockProgress(&buf, -1)
	p.Done(42, false, nil)
	got := buf.String()
	if strings.HasPrefix(got, "\r\033[K") {
		t.Errorf("nothing was rendered — Done must not clear: %q", got)
	}
	if !strings.Contains(got, "✓ m.gguf (42 B)") {
		t.Errorf("Done line = %q", got)
	}
}

func TestProgressWriter_DoneFailureMentionsResume(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	var buf bytes.Buffer
	p, _ := newFakeClockProgress(&buf, 1000)
	p.Write(make([]byte, 10))
	buf.Reset()

	p.Done(0, true, errors.New("boom"))
	got := buf.String()
	if !strings.Contains(got, "failed to download m.gguf: boom") {
		t.Errorf("failure line = %q", got)
	}
	if !strings.Contains(got, "resume") {
		t.Errorf("a failed resumed transfer must mention the .partial retry: %q", got)
	}

	buf.Reset()
	p.Done(0, false, errors.New("boom"))
	if strings.Contains(buf.String(), "resume") {
		t.Errorf("a fresh failure must not mention resume: %q", buf.String())
	}
}

func TestProgressWriter_NilOutIsSilent(t *testing.T) {
	p := newProgressWriter(nil, "m.gguf", 10, time.Now())
	if n, err := p.Write([]byte("12345")); n != 5 || err != nil {
		t.Errorf("Write = %d, %v", n, err)
	}
	p.Done(5, false, nil) // must not panic
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{5 << 20, "5.0 MiB"},
		{1610612736, "1.5 GiB"},
		{-5, "0 B"},
	}
	for _, tt := range tests {
		if got := formatBytes(tt.n); got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestFormatETA(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{270 * time.Second, "04:30"},
		{0, "00:00"},
		{-time.Second, "00:00"},
		{59 * time.Second, "00:59"},
		{61 * time.Second, "01:01"},
		{100 * time.Minute, "99:59"},
	}
	for _, tt := range tests {
		if got := formatETA(tt.d); got != tt.want {
			t.Errorf("formatETA(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
